package hedgehog

import (
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Resource interface {
	Match(*http.Request) bool
	Check(*http.Response) error
	After() <-chan time.Time
}

type Method uint16

const (
	MethodGet Method = 1 << iota
	MethodHead
	MethodPost
	MethodPut
	MethodDelete
	MethodConnect
	MethodOptions
	MethodTrace
	MethodPatch
	MethodUndefined
)

func toMethod(method string) Method {
	method = strings.ToUpper(method)
	switch method {
	case "GET":
		return MethodGet
	case "HEAD":
		return MethodHead
	case "POST":
		return MethodPost
	case "PUT":
		return MethodPut
	case "DELETE":
		return MethodDelete
	case "CONNECT":
		return MethodConnect
	case "OPTIONS":
		return MethodOptions
	case "TRACE":
		return MethodTrace
	case "PATCH":
		return MethodPatch
	default:
		return MethodUndefined
	}
}

type Simple struct {
	Method    Method
	Path      *regexp.Regexp
	Delay     time.Duration
	Condition func(code int) error
}

func (r Simple) Match(req *http.Request) bool {
	if r.Method&toMethod(req.Method) == 0 {
		return false
	}
	if r.Path != nil && !r.Path.MatchString(req.URL.String()) {
		return false
	}
	return true
}

func (r Simple) Check(resp *http.Response) error {
	return r.Condition(resp.StatusCode)
}

func (r Simple) After() <-chan time.Time {
	return time.After(r.Delay)
}
