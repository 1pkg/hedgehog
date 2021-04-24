package hedgehog

import (
	"context"
	"net/http"

	"golang.org/x/sync/errgroup"
)

// NewHTTPClient wraps provided http client with hedged transport.
// If nil client is provided default client will be used, if nil transport is provided default transport will be used.
func NewHTTPClient(client *http.Client, calls uint64, resources ...Resource) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	if client.Transport == nil {
		client.Transport = http.DefaultTransport
	}
	client.Transport = NewRoundTripper(client.Transport, calls, resources...)
	return client
}

type transport struct {
	internal  http.RoundTripper
	resources []Resource
	calls     uint64
}

// NewRoundTripper returns new http hedged transport with provided resources.
// Returned transport will make hedged http calls in case of resource matching http request up to calls+1 times,
// original http call starts right away and then all hedged calls start together after delay specified by resource.
// Returned transport will process and return first successful http response, in case all hedged response failed
// it will simply return first occurred error.
// If no matching resource were found - the transport will simply call underlying transport.
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
	res := make(chan interface{}, t.calls+1)
	defer close(res)
	g.Go(func() error {
		for i := uint64(0); i < t.calls+1; i++ {
			select {
			case r := <-res:
				switch tr := r.(type) {
				case *http.Response:
					resp = tr
					err = nil
					// if we got result hard stop execution.
					return context.Canceled
				case error:
					// keep only first occurred error.
					if err == nil {
						err = tr
					}
				}
			case <-ctx.Done():
				err = ctx.Err()
				// if group was canceled hard stop execution.
				return context.Canceled
			}
		}
		return nil
	})
	roundTrip := func() error {
		req := req.Clone(ctx)
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
