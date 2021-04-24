package hedgehog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"testing"
	"time"
)

type tparams struct {
	method string
	path   string
	code   int
	delay  time.Duration
	rcode  int
	err    error
}

func tserv(method string, path string, delay time.Duration, code int) (string, context.CancelFunc) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == method && req.URL.Path == path {
			time.Sleep(delay)
			w.WriteHeader(code)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	return srv.URL, srv.Close
}

func mustNewRequest(method string, url string) *http.Request {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		panic(err)
	}
	return req
}

func TestRoundTripper(t *testing.T) {
	ctxCanceled, cancel := context.WithCancel(context.TODO())
	cancel()
	ttable := map[string]struct {
		ctx    context.Context
		t      http.RoundTripper
		params []tparams
	}{
		"should execute default transport if no matching resources find by method": {
			ctx: context.TODO(),
			t: NewRoundTripper(
				http.DefaultTransport,
				1,
				NewResourceStatic(MethodGet, regexp.MustCompile(`profile`), time.Millisecond, http.StatusOK),
			),
			params: []tparams{
				{
					method: "Post",
					path:   "/profile",
					code:   http.StatusOK,
					rcode:  http.StatusOK,
				},
			},
		},
		"should execute default transport if no matching resources find by path": {
			ctx: context.TODO(),
			t: NewRoundTripper(
				http.DefaultTransport,
				1,
				NewResourceStatic(MethodGet, regexp.MustCompile(`users`), time.Millisecond, http.StatusOK),
			),
			params: []tparams{
				{
					method: "Get",
					path:   "/profile",
					code:   http.StatusOK,
					rcode:  http.StatusOK,
				},
			},
		},
		"should return error back on canceled request and matching resources": {
			ctx: ctxCanceled,
			t: NewRoundTripper(
				http.DefaultTransport,
				1,
				NewResourceStatic(MethodPut, regexp.MustCompile(`profile`), time.Millisecond, http.StatusOK),
			),
			params: []tparams{
				{
					method: "Put",
					path:   "/profile",
					code:   http.StatusOK,
					err:    ctxCanceled.Err(),
				},
			},
		},
		"should return error back on unexpected response status code and matching resources": {
			ctx: context.TODO(),
			t: NewRoundTripper(
				http.DefaultTransport,
				1,
				NewResourceStatic(MethodHead, regexp.MustCompile(`profile`), time.Millisecond, http.StatusOK),
			),
			params: []tparams{
				{
					method: "Head",
					path:   "/profile",
					code:   http.StatusForbidden,
					err:    ErrResourceUnexpectedResponseCode{StatusCode: http.StatusForbidden},
				},
			},
		},
		"should return response back on successful response and matching resources": {
			ctx: context.TODO(),
			t: NewRoundTripper(
				http.DefaultTransport,
				1,
				NewResourceStatic(MethodOptions, regexp.MustCompile(`profile`), time.Millisecond, http.StatusOK),
			),
			params: []tparams{
				{
					method: "Options",
					path:   "/profile",
					code:   http.StatusOK,
					rcode:  http.StatusOK,
				},
			},
		},
		"should return response back on successful response and matching resources multi calls": {
			ctx: context.TODO(),
			t: NewRoundTripper(
				http.DefaultTransport,
				5,
				NewResourceStatic(MethodTrace, regexp.MustCompile(`profile/[0-9]`), time.Millisecond, http.StatusOK),
			),
			params: []tparams{
				{
					method: "Trace",
					path:   "/profile/7",
					code:   http.StatusOK,
					delay:  time.Millisecond * 5,
					rcode:  http.StatusOK,
				},
			},
		},
	}
	for tname, tcase := range ttable {
		t.Run(tname, func(t *testing.T) {
			cli := NewHTTPClient(nil, ClientWithRoundTripper(tcase.t))
			for _, param := range tcase.params {
				t.Run(fmt.Sprintf("%s %s", param.method, param.path), func(t *testing.T) {
					uri, stop := tserv(param.method, param.path, param.delay, param.code)
					req := mustNewRequest(param.method, uri+param.path).WithContext(tcase.ctx)
					resp, err := cli.Do(req)
					stop()
					var perr error
					if param.err != nil {
						perr = &url.Error{
							Op:  req.Method,
							URL: req.URL.String(),
							Err: param.err,
						}
					}
					if !errors.Is(perr, err) && (err != nil && perr.Error() != err.Error()) {
						t.Fatalf("expected err %v but got %v", perr, err)
					}
					if err == nil && param.rcode != resp.StatusCode {
						t.Fatalf("expected status code %d but got %d", param.rcode, resp.StatusCode)
					}
				})
			}
		})
	}
}
