package hedgehog

import (
	"context"
	"net/http"

	"golang.org/x/sync/errgroup"
)

type transport struct {
	internal  http.RoundTripper
	resources []Resource
	calls     uint64
}

func NewRoundTripper(internal http.RoundTripper, calls uint64, resources ...Resource) http.RoundTripper {
	return transport{internal: internal, calls: calls, resources: resources}
}

func (t transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	for _, rs := range t.resources {
		if rs.Match(req) {
			return t.multiRoundTrip(req, rs)
		}
	}
	return t.internal.RoundTrip(req)
}

func (t transport) multiRoundTrip(req *http.Request, rs Resource) (resp *http.Response, err error) {
	g, ctx := errgroup.WithContext(req.Context())
	req = req.WithContext(ctx)
	res := make(chan interface{}, t.calls+1)
	g.Go(func() error {
		defer close(res)
		for i := uint64(0); i < t.calls+1; i++ {
			select {
			case r := <-res:
				switch tr := r.(type) {
				case *http.Response:
					resp = tr
					// if we got result hard stop execution.
					return context.Canceled
				case error:
					err = tr
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
	roundTrip := func() error {
		h := rs.Hook(req)
		resp, err := t.internal.RoundTrip(req)
		if err != nil {
			res <- err
			return nil
		}
		if err := rs.Check(resp); err != nil {
			res <- err
			return nil
		}
		h(resp)
		res <- resp
		return nil
	}
	g.Go(roundTrip)
	<-rs.After()
	for i := uint64(0); i < t.calls; i++ {
		g.Go(roundTrip)
	}
	_ = g.Wait()
	return
}
