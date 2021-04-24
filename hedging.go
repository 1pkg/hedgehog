package hedgehog

import (
	"context"
	"net/http"

	"golang.org/x/sync/errgroup"
)

type hedging struct {
	internal  http.RoundTripper
	resources []Resource
	times     uint64
}

func NewRoundTripper(internal http.RoundTripper, times uint64, resources ...Resource) http.RoundTripper {
	return hedging{internal: internal, times: times, resources: resources}
}

func (rt hedging) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	for _, rs := range rt.resources {
		if rs.Match(req) {
			return rt.multiRoundTrip(req, rs)
		}
	}
	return rt.internal.RoundTrip(req)
}

func (rt hedging) multiRoundTrip(req *http.Request, rs Resource) (resp *http.Response, err error) {
	g, ctx := errgroup.WithContext(req.Context())
	req = req.WithContext(ctx)
	res := make(chan interface{}, rt.times+1)
	g.Go(func() error {
		defer close(res)
		for i := uint64(0); i < rt.times+1; i++ {
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
		resp, err := rt.internal.RoundTrip(req)
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
	for i := uint64(0); i < rt.times; i++ {
		g.Go(roundTrip)
	}
	_ = g.Wait()
	return
}
