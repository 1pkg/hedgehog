package hedgehog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync/atomic"
	"testing"
	"time"
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

func TestRoundTripper(t *testing.T) {
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
	ctxCanceled, cancel := context.WithCancel(context.TODO())
	cancel()
	ttable := map[string]struct {
		ctx   context.Context
		calls uint64
		res   []Resource
		tcall struct {
			req  treq
			resp tresp
		}
	}{
		"should execute default transport if no matching resources found method": {
			ctx:   context.TODO(),
			calls: 1,
			res:   []Resource{NewResourceStatic(http.MethodGet, regexp.MustCompile(`profile`), ms_1, http.StatusOK)},
			tcall: struct {
				req  treq
				resp tresp
			}{
				req: treq{
					method: http.MethodHead,
					path:   "/profile",
					codes:  []int{http.StatusOK, http.StatusOK},
				},
				resp: tresp{
					code: http.StatusOK,
				},
			},
		},
		"should execute default transport if no matching resources found path": {
			ctx:   context.TODO(),
			calls: 1,
			res:   []Resource{NewResourceStatic(http.MethodGet, regexp.MustCompile(`users`), ms_1, http.StatusOK)},
			tcall: struct {
				req  treq
				resp tresp
			}{
				req: treq{
					method: http.MethodGet,
					path:   "/profile",
					codes:  []int{http.StatusOK, http.StatusOK},
				},
				resp: tresp{
					code: http.StatusOK,
				},
			},
		},
		"should return error back on canceled request and matching resources": {
			ctx:   ctxCanceled,
			calls: 1,
			res:   []Resource{NewResourceStatic(http.MethodPut, regexp.MustCompile(`profile`), ms_1, http.StatusOK)},
			tcall: struct {
				req  treq
				resp tresp
			}{
				req: treq{
					method: http.MethodPut,
					path:   "/profile",
					codes:  []int{http.StatusOK, http.StatusOK},
				},
				resp: tresp{
					err: ctxCanceled.Err(),
				},
			},
		},
		"should return error back on unexpected response status code and matching resources": {
			ctx:   context.TODO(),
			calls: 1,
			res:   []Resource{NewResourceStatic(http.MethodDelete, regexp.MustCompile(`profile`), ms_1, http.StatusOK)},
			tcall: struct {
				req  treq
				resp tresp
			}{
				req: treq{
					method: http.MethodDelete,
					path:   "/profile",
					codes:  []int{http.StatusConflict, http.StatusForbidden},
					delays: []time.Duration{ms_50, ms_2},
				},
				resp: tresp{
					err: ErrResourceUnexpectedResponseCode{StatusCode: http.StatusForbidden},
				},
			},
		},
		"should return response back on successful response and matching resources": {
			ctx:   context.TODO(),
			calls: 1,
			res:   []Resource{NewResourceStatic(http.MethodGet, regexp.MustCompile(`profile`), ms_1, http.StatusOK)},
			tcall: struct {
				req  treq
				resp tresp
			}{
				req: treq{
					method: http.MethodGet,
					path:   "/profile",
					codes:  []int{http.StatusOK, http.StatusOK},
				},
				resp: tresp{
					code: http.StatusOK,
				},
			},
		},
		"should return response back on successful response and first matching resources": {
			ctx:   context.TODO(),
			calls: 1,
			res: []Resource{
				NewResourceStatic(http.MethodGet, regexp.MustCompile(`user`), ms_1, http.StatusOK),
				NewResourceStatic(http.MethodPut, regexp.MustCompile(`profile`), ms_1, http.StatusOK),
				NewResourceStatic(http.MethodGet, regexp.MustCompile(`profile`), ms_5, http.StatusOK),
				NewResourceStatic(http.MethodGet, regexp.MustCompile(`profile`), ms_100, http.StatusOK),
			},
			tcall: struct {
				req  treq
				resp tresp
			}{
				req: treq{
					method: http.MethodGet,
					path:   "/profile",
					codes:  []int{http.StatusNotFound, http.StatusOK},
				},
				resp: tresp{
					code:  http.StatusOK,
					delay: ms_50,
				},
			},
		},
		"should return response back on successful response and matching resources even if first request failed": {
			ctx:   context.TODO(),
			calls: 1,
			res:   []Resource{NewResourceStatic(http.MethodGet, regexp.MustCompile(`profile`), ms_1, http.StatusOK)},
			tcall: struct {
				req  treq
				resp tresp
			}{
				req: treq{
					method: http.MethodGet,
					path:   "/profile",
					codes:  []int{http.StatusForbidden, http.StatusOK},
				},
				resp: tresp{
					code: http.StatusOK,
				},
			},
		},
		"should return response back on successful response and matching resources multi calls": {
			ctx:   context.TODO(),
			calls: 3,
			res:   []Resource{NewResourceStatic(http.MethodGet, regexp.MustCompile(`profile/[0-9]`), ms_1, http.StatusOK)},
			tcall: struct {
				req  treq
				resp tresp
			}{
				req: treq{
					method: http.MethodGet,
					path:   "/profile/7",
					codes:  []int{http.StatusOK, http.StatusOK, http.StatusOK, http.StatusOK},
					delays: []time.Duration{ms_100, ms_2, ms_100, ms_5, ms_100},
				},
				resp: tresp{
					code:  http.StatusOK,
					delay: ms_50,
				},
			},
		},
	}
	for tname, tcase := range ttable {
		t.Run(tname, func(t *testing.T) {
			cli := NewHTTPClient(nil, tcase.calls, tcase.res...)
			uri, stop := tserv(tcase.tcall.req.method, tcase.tcall.req.path, tcase.tcall.req.codes, tcase.tcall.req.delays)
			req, _ := http.NewRequest(tcase.tcall.req.method, uri+tcase.tcall.req.path, nil)
			req = req.WithContext(tcase.ctx)
			ts := time.Now()
			resp, err := cli.Do(req)
			ds := time.Since(ts)
			stop()
			if unwrapHttpError(tcase.tcall.resp.err) != unwrapHttpError(err) {
				t.Fatalf("expected err %v but got %v", unwrapHttpError(tcase.tcall.resp.err), unwrapHttpError(err))
			}
			if tcase.tcall.resp.code != 0 && tcase.tcall.resp.code != resp.StatusCode {
				t.Fatalf("expected response status code %d but got %d", tcase.tcall.resp.code, resp.StatusCode)
			}
			if tcase.tcall.resp.delay != 0 && tcase.tcall.resp.delay < ds {
				t.Fatalf("expected response latency be < %s but got %s", tcase.tcall.resp.delay, ds)
			}
		})
	}
}
