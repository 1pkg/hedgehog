package hedgehog

import "net/http"

type ClientOption func(*http.Client)

func ClientWithRoundTripper(t http.RoundTripper) ClientOption {
	return func(client *http.Client) {
		if t != nil {
			client.Transport = t
		}
	}
}

func NewHTTPClient(client *http.Client, opts ...ClientOption) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	for _, opt := range opts {
		opt(client)
	}
	return client
}
