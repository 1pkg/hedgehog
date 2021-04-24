package hedgehog

import (
	"net/http"
	"regexp"
	"time"
)

var DefaultResource = NewResourceDynamic(
	MethodGet|MethodHead|MethodPost|MethodPut|MethodDelete|MethodConnect|MethodOptions|MethodTrace|MethodPatch,
	regexp.MustCompile(`.*`),
	100*time.Millisecond,
	0.5,
	100,
	http.StatusOK,
	http.StatusCreated,
	http.StatusAccepted,
	http.StatusNonAuthoritativeInfo,
	http.StatusNoContent,
	http.StatusResetContent,
	http.StatusPartialContent,
	http.StatusMultiStatus,
	http.StatusAlreadyReported,
	http.StatusIMUsed,
)

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
	def := ClientWithRoundTripper(NewRoundTripper(client.Transport, 1, DefaultResource))
	opts = append([]ClientOption{def}, opts...)
	for _, opt := range opts {
		opt(client)
	}
	return client
}
