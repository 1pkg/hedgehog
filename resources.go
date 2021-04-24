package hedgehog

import (
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type Resource interface {
	Match(*http.Request) bool
	Check(*http.Response) error
	After() <-chan time.Time
	Acknowledge(time.Duration)
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

type static struct {
	method Method
	path   *regexp.Regexp
	delay  time.Duration
	codes  map[int]bool
}

func NewResourceStatic(method Method, path *regexp.Regexp, delay time.Duration, codes ...int) Resource {
	rs := static{
		method: method,
		path:   path,
		delay:  delay,
		codes:  make(map[int]bool, len(codes)),
	}
	for _, code := range codes {
		rs.codes[code] = true
	}
	return rs
}

func (r static) Match(req *http.Request) bool {
	if r.method&toMethod(req.Method) == 0 {
		return false
	}
	if r.path != nil && !r.path.MatchString(req.URL.String()) {
		return false
	}
	return true
}

func (r static) Check(resp *http.Response) error {
	if !r.codes[resp.StatusCode] {
		return nil
	}
	return nil
}

func (r static) After() <-chan time.Time {
	return time.After(r.delay)
}

func (r static) Acknowledge(time.Duration) {}

type dynamic struct {
	static
	percentile float64
	capacity   int
	latencies  []time.Duration
	lock       sync.RWMutex
}

func NewResourceDynamic(method Method, path *regexp.Regexp, delay time.Duration, percentile float64, capacity uint64, codes ...int) Resource {
	percentile = math.Abs(percentile)
	if percentile > 1.0 {
		percentile = 1.0
	}
	if capacity > math.MaxInt32 {
		capacity = math.MaxInt32
	}
	return &dynamic{
		static:     NewResourceStatic(method, path, delay, codes...).(static),
		percentile: percentile,
		capacity:   int(capacity),
		latencies:  make([]time.Duration, 0, capacity),
	}
}

func (r *dynamic) After() <-chan time.Time {
	delay := r.delay
	r.lock.RLock()
	if l := len(r.latencies); l >= r.capacity/2 {
		lat := make([]time.Duration, l)
		copy(lat, r.latencies)
		sort.Slice(lat, func(i, j int) bool {
			return lat[i] < lat[j]
		})
		delay = lat[int(math.Round(float64(l)*r.percentile))]
	}
	r.lock.RUnlock()
	return time.After(delay)
}

func (r *dynamic) Acknowledge(d time.Duration) {
	r.lock.Lock()
	r.latencies = append(r.latencies, d)
	if len(r.latencies) > r.capacity*2 {
		r.latencies = r.latencies[r.capacity:]
	}
	r.lock.Unlock()
}
