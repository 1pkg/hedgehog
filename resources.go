package hedgehog

import (
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"sync"
	"time"
)

type ErrResourceUnexpectedResponseCode struct {
	StatusCode int
}

func (err ErrResourceUnexpectedResponseCode) Error() string {
	return fmt.Sprintf("resource check failed: received unexpected response status code %d", err.StatusCode)
}

type Resource interface {
	After() <-chan time.Time
	Match(*http.Request) bool
	Check(*http.Response) error
	Hook(*http.Request) func(*http.Response)
}

type static struct {
	method string
	path   *regexp.Regexp
	delay  time.Duration
	codes  map[int]bool
}

func NewResourceStatic(method string, path *regexp.Regexp, delay time.Duration, allowedCodes ...int) Resource {
	rs := static{
		method: method,
		path:   path,
		delay:  delay,
		codes:  make(map[int]bool, len(allowedCodes)),
	}
	for _, code := range allowedCodes {
		rs.codes[code] = true
	}
	return rs
}

func (r static) After() <-chan time.Time {
	return time.After(r.delay)
}

func (r static) Match(req *http.Request) bool {
	if r.method != req.Method {
		return false
	}
	if r.path != nil && !r.path.MatchString(req.URL.String()) {
		return false
	}
	return true
}

func (r static) Check(resp *http.Response) error {
	if !r.codes[resp.StatusCode] {
		return ErrResourceUnexpectedResponseCode{StatusCode: resp.StatusCode}
	}
	return nil
}

func (r static) Hook(*http.Request) func(*http.Response) {
	return func(*http.Response) {}
}

type dynamic struct {
	static
	percentile float64
	capacity   int
	latencies  []time.Duration
	lock       sync.RWMutex
}

func NewResourceDynamic(method string, path *regexp.Regexp, delay time.Duration, percentile float64, capacity uint64, allowedCodes ...int) Resource {
	percentile = math.Abs(percentile)
	if percentile > 1.0 {
		percentile = 1.0
	}
	if capacity > math.MaxInt32 {
		capacity = math.MaxInt32
	}
	return &dynamic{
		static:     NewResourceStatic(method, path, delay, allowedCodes...).(static),
		percentile: percentile,
		capacity:   int(capacity),
		latencies:  make([]time.Duration, 0, capacity),
	}
}

func (r *dynamic) After() <-chan time.Time {
	delay := r.delay
	r.lock.RLock()
	if l := len(r.latencies); l >= r.capacity {
		lat := make([]time.Duration, l)
		copy(lat, r.latencies)
		sort.Slice(lat, func(i, j int) bool {
			return lat[i] < lat[j]
		})
		delay = lat[int(math.Round(float64(l)*r.percentile))-1]
	}
	r.lock.RUnlock()
	return time.After(delay)
}

func (r *dynamic) Hook(*http.Request) func(*http.Response) {
	t := time.Now()
	return func(*http.Response) {
		d := time.Since(t)
		r.lock.Lock()
		r.latencies = append(r.latencies, d)
		if len(r.latencies) >= r.capacity*2 {
			r.latencies = r.latencies[r.capacity:]
		}
		r.lock.Unlock()
	}
}
