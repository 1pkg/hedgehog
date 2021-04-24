package hedgehog

import (
	"net/http"

	"golang.org/x/sync/errgroup"
)

var FanoutTimes uint64 = 1

type hedging struct {
	processor http.RoundTripper
	resources []Resource
}

func NewHedging(processor http.RoundTripper, resources ...Resource) http.RoundTripper {
	return hedging{processor: processor, resources: resources}
}

func (t hedging) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	var resource Resource
	for _, rs := range t.resources {
		if rs.Match(req) {
			resource = rs
		}
	}
	if resource == nil {
		return t.processor.RoundTrip(req)
	}
	g, ctx := errgroup.WithContext(req.Context())
	req = req.WithContext(ctx)
	times := FanoutTimes + 1
	results := make(chan interface{}, times)
	g.Go(func() error {
		defer close(results)
		for i := uint64(0); i < times; i++ {
			select {
			case <-results:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
	for i := uint64(0); i < times; i++ {
		g.Go(func() error {
			resp, err := t.processor.RoundTrip(req)
			if err != nil {
				results <- err
				return nil
			}
			if err := resource.Check(resp); err != nil {
				results <- err
				return nil
			}
			results <- resp
			return nil
		})
		if i == 0 && times > 1 {
			<-resource.After()
		}
	}
	_ = g.Wait()
	return
}
