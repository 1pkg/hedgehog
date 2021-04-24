package hedgehog

import (
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
)

type hedging struct {
	internal  http.RoundTripper
	resources []Resource
	times     uint64
}

func NewHedging(internal http.RoundTripper, times uint64, resources ...Resource) http.RoundTripper {
	return hedging{internal: internal, times: times + 1, resources: resources}
}

func (rt hedging) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	var resource Resource
	for _, rs := range rt.resources {
		if rs.Match(req) {
			resource = rs
		}
	}
	if resource == nil {
		return rt.internal.RoundTrip(req)
	}
	g, ctx := errgroup.WithContext(req.Context())
	req = req.WithContext(ctx)
	results := make(chan interface{}, rt.times)
	g.Go(func() error {
		defer close(results)
		for i := uint64(0); i < rt.times; i++ {
			select {
			case <-results:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
	for i := uint64(0); i < rt.times; i++ {
		g.Go(func() error {
			t := time.Now()
			resp, err := rt.internal.RoundTrip(req)
			if err != nil {
				results <- err
				return nil
			}
			d := time.Since(t)
			if err := resource.Check(resp); err != nil {
				results <- err
				return nil
			}
			resource.Acknowledge(d)
			results <- resp
			return nil
		})
		if i == 0 && rt.times > 1 {
			<-resource.After()
		}
	}
	_ = g.Wait()
	return
}
