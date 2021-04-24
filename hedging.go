package hedgehog

import (
	"net/http"
	"regexp"
	"time"

	"golang.org/x/sync/errgroup"
)

var FanoutTimes uint64 = 1

type Endpoint struct {
	Regexp  *regexp.Regexp
	After   time.Duration
	Checker func(code int) error
}

type hedging struct {
	rt        http.RoundTripper
	ednpoints []Endpoint
}

func NewHedging(rt http.RoundTripper, ednpoints ...Endpoint) http.RoundTripper {
	return hedging{rt: rt, ednpoints: ednpoints}
}

func (ep Endpoint) roundTrip(rt http.RoundTripper, req *http.Request) (*http.Response, error) {
	resp, err := rt.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if err := ep.Checker(resp.StatusCode); err != nil {
		return nil, err
	}
	return resp, nil
}

func (t hedging) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	endpoint, ok := t.match(req)
	if !ok {
		return t.rt.RoundTrip(req)
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
			resp, err := endpoint.roundTrip(t.rt, req)
			if err != nil {
				results <- err
				return nil
			}
			results <- resp
			return nil
		})
		if i == 0 && times > 1 {
			<-time.After(endpoint.After)
		}
	}

	if resp != nil {
		return resp, nil
	}
	return nil, err
}

func (t hedging) match(req *http.Request) (*Endpoint, bool) {
	for _, ednpoint := range t.ednpoints {
		if ednpoint.Regexp != nil && ednpoint.Regexp.MatchString(req.URL.String()) {
			return &ednpoint, true
		}
	}
	return nil, false
}
