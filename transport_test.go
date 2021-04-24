package hedgehog

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync/atomic"
	"testing"
	"time"
)

const (
	ms_0   = time.Millisecond * 0
	ms_1   = time.Millisecond * 1
	ms_5   = time.Millisecond * 5
	ms_10  = time.Millisecond * 10
	ms_20  = time.Millisecond * 20
	ms_50  = time.Millisecond * 50
	ms_100 = time.Millisecond * 50
)

func tserv(method string, path string, codes []int, delays []time.Duration) (string, context.CancelFunc) {
	var i int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == method && req.URL.Path == path {
			n := atomic.AddInt64(&i, 1) - 1
			code := http.StatusOK
			delay := ms_0
			if n < int64(len(codes)) {
				code = codes[n]
			}
			if n < int64(len(delays)) {
				delay = delays[n]
			}
			time.Sleep(delay)
			w.WriteHeader(code)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	return srv.URL, srv.Close
}

func unwrapHttpError(err error) string {
	if err == nil {
		return "nil"
	}
	if err := errors.Unwrap(err); err != nil {
		return err.Error()
	}
	return err.Error()
}

type treq struct {
	method string
	path   string
	codes  []int
	delays []time.Duration
}

type tresp struct {
	code  int
	delay time.Duration
	err   error
}

type tcall struct {
	req  treq
	resp tresp
}

func TestRoundTripper(t *testing.T) {
	ctxCanceled, cancel := context.WithCancel(context.TODO())
	cancel()
	ttable := map[string]struct {
		ctx    context.Context
		t      http.RoundTripper
		tcalls []tcall
	}{
		"should execute default transport if no matching resources find by method": {
			ctx: context.TODO(),
			t: NewRoundTripper(
				http.DefaultTransport,
				1,
				NewResourceStatic(MethodGet, regexp.MustCompile(`profile`), ms_1, http.StatusOK),
			),
			tcalls: []tcall{
				{
					req: treq{
						method: "Post",
						path:   "/profile",
						codes:  []int{http.StatusOK},
					},
					resp: tresp{
						code: http.StatusOK,
					},
				},
			},
		},
		"should execute default transport if no matching resources find by path": {
			ctx: context.TODO(),
			t: NewRoundTripper(
				http.DefaultTransport,
				1,
				NewResourceStatic(MethodGet, regexp.MustCompile(`users`), ms_1, http.StatusOK),
			),
			tcalls: []tcall{
				{
					req: treq{
						method: "Get",
						path:   "/profile",
						codes:  []int{http.StatusOK, http.StatusOK},
					},
					resp: tresp{
						code: http.StatusOK,
					},
				},
			},
		},
		"should return error back on canceled request and matching resources": {
			ctx: ctxCanceled,
			t: NewRoundTripper(
				http.DefaultTransport,
				1,
				NewResourceStatic(MethodPut, regexp.MustCompile(`profile`), ms_1, http.StatusOK),
			),
			tcalls: []tcall{
				{
					req: treq{
						method: "Put",
						path:   "/profile",
						codes:  []int{http.StatusOK, http.StatusOK},
					},
					resp: tresp{
						err: ctxCanceled.Err(),
					},
				},
			},
		},
		"should return error back on unexpected response status code and matching resources": {
			ctx: context.TODO(),
			t: NewRoundTripper(
				http.DefaultTransport,
				1,
				NewResourceStatic(MethodHead, regexp.MustCompile(`profile`), ms_1, http.StatusOK),
			),
			tcalls: []tcall{
				{
					req: treq{
						method: "Head",
						path:   "/profile",
						codes:  []int{http.StatusForbidden, http.StatusForbidden},
					},
					resp: tresp{
						err: ErrResourceUnexpectedResponseCode{StatusCode: http.StatusForbidden},
					},
				},
			},
		},
		"should return response back on successful response and matching resources": {
			ctx: context.TODO(),
			t: NewRoundTripper(
				http.DefaultTransport,
				1,
				NewResourceStatic(MethodOptions, regexp.MustCompile(`profile`), ms_1, http.StatusOK),
			),
			tcalls: []tcall{
				{
					req: treq{
						method: "Options",
						path:   "/profile",
						codes:  []int{http.StatusOK, http.StatusOK},
					},
					resp: tresp{
						code: http.StatusOK,
					},
				},
			},
		},
		"should return response back on successful response and matching resources even if first request failed": {
			ctx: context.TODO(),
			t: NewRoundTripper(
				http.DefaultTransport,
				1,
				NewResourceStatic(MethodHead, regexp.MustCompile(`profile`), ms_1, http.StatusOK),
			),
			tcalls: []tcall{
				{
					req: treq{
						method: "Head",
						path:   "/profile",
						codes:  []int{http.StatusForbidden, http.StatusOK},
					},
					resp: tresp{
						code: http.StatusOK,
					},
				},
			},
		},
		"should return response back on successful response and matching resources multi calls": {
			ctx: context.TODO(),
			t: NewRoundTripper(
				http.DefaultTransport,
				3,
				NewResourceStatic(MethodTrace, regexp.MustCompile(`profile/[0-9]`), ms_1, http.StatusOK),
			),
			tcalls: []tcall{
				{
					req: treq{
						method: "Trace",
						path:   "/profile/7",
						codes:  []int{http.StatusOK, http.StatusOK, http.StatusOK, http.StatusOK},
						delays: []time.Duration{ms_20, ms_5, ms_20, ms_5, ms_20},
					},
					resp: tresp{
						code:  http.StatusOK,
						delay: ms_5,
					},
				},
			},
		},
		"should memorize latencies on dynamic resources and return response back": {
			ctx: context.TODO(),
			t: NewRoundTripper(
				http.DefaultTransport,
				2,
				NewResourceDynamic(MethodConnect|MethodDelete, regexp.MustCompile(`profile/[0-9]+`), ms_1, 0.5, 3, http.StatusOK),
			),
			tcalls: []tcall{
				{
					req: treq{
						method: "Connect",
						path:   "/profile/711",
						codes:  []int{http.StatusOK, http.StatusOK, http.StatusOK},
						delays: []time.Duration{ms_100, ms_50, ms_100},
					},
					resp: tresp{
						code:  http.StatusOK,
						delay: ms_50,
					},
				},
				{
					req: treq{
						method: "Connect",
						path:   "/profile/712",
						codes:  []int{http.StatusOK, http.StatusOK, http.StatusOK},
						delays: []time.Duration{ms_50, ms_20, ms_50},
					},
					resp: tresp{
						code:  http.StatusOK,
						delay: ms_20,
					},
				},
				{
					req: treq{
						method: "Connect",
						path:   "/profile/713",
						codes:  []int{http.StatusOK, http.StatusOK, http.StatusOK},
						delays: []time.Duration{ms_20, ms_1, ms_20},
					},
					resp: tresp{
						code:  http.StatusOK,
						delay: ms_1,
					},
				},
				{
					req: treq{
						method: "Delete",
						path:   "/profile/714",
						codes:  []int{http.StatusOK, http.StatusOK, http.StatusOK},
						delays: []time.Duration{ms_1, ms_1, ms_1},
					},
					resp: tresp{
						code:  http.StatusOK,
						delay: ms_20,
					},
				},
			},
		},
	}
	for tname, tcase := range ttable {
		t.Run(tname, func(t *testing.T) {
			cli := NewHTTPClient(nil, ClientWithRoundTripper(tcase.t))
			for _, tcall := range tcase.tcalls {
				t.Run(fmt.Sprintf("%s %s", tcall.req.method, tcall.req.path), func(t *testing.T) {
					uri, stop := tserv(tcall.req.method, tcall.req.path, tcall.req.codes, tcall.req.delays)
					req, _ := http.NewRequest(tcall.req.method, uri+tcall.req.path, nil)
					req = req.WithContext(tcase.ctx)
					ts := time.Now()
					resp, err := cli.Do(req)
					ds := time.Since(ts)
					stop()
					if unwrapHttpError(tcall.resp.err) != unwrapHttpError(err) {
						t.Fatalf("expected err %v but got %v", unwrapHttpError(tcall.resp.err), unwrapHttpError(err))
					}
					if tcall.resp.err == nil && tcall.resp.code != resp.StatusCode {
						t.Fatalf("expected response status code %d but got %d", tcall.resp.code, resp.StatusCode)
					}
					if tcall.resp.err == nil && time.Duration(math.Abs(float64(tcall.resp.delay-ds))) > ms_10 {
						t.Fatalf("expected response latency be < %s but got %s", tcall.resp.delay, ds)
					}
				})
			}
		})
	}
}
