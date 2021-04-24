package hedgehog

import (
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ErrResourceUnexpectedResponseCode defines resource response check error that is returned on non matching response code.
type ErrResourceUnexpectedResponseCode struct {
	StatusCode int
}

func (err ErrResourceUnexpectedResponseCode) Error() string {
	return fmt.Sprintf("resource check failed: received unexpected response status code %d", err.StatusCode)
}

// Resource defines abstract http resource that is capable of:
// - matching http request applicability
// - checking http request validity
// - and returning delay which should be accounted before executing this resource request
type Resource interface {
	After() <-chan time.Time
	Match(*http.Request) bool
	Check(*http.Response) error
	Hook(*http.Request) func(*http.Response)
}

type static struct {
	method string
	url    *regexp.Regexp
	delay  time.Duration
	codes  map[int]bool
}

// NewResourceStatic returns new resource instance that always waits for static specified delay.
// Returned resource matches each request against both provided http method and full url regexp.
// Returned resource checks if response result http code is included in provided allowed codes,
// if it is not it returnes `ErrResourceUnexpectedResponseCode`.
func NewResourceStatic(method string, url *regexp.Regexp, delay time.Duration, allowedCodes ...int) Resource {
	rs := static{
		method: method,
		url:    url,
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
	if r.url != nil && !r.url.MatchString(req.URL.String()) {
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

type average struct {
	static
	sum      int64
	count    int64
	capacity int64
}

// NewResourceAverage returns new resource instance that dynamically adjust wait delay based on
// recieved successfull responses average delays.
// Returned resource is starting to use dynamically adjusted wait delay only after capacity/4 calls.
// Returned resource matches each request against both provided http method and full url regexp.
// Returned resource checks if response result http code is included in provided allowed codes,
// if it is not it returnes `ErrResourceUnexpectedResponseCode`.
func NewResourceAverage(method string, url *regexp.Regexp, delay time.Duration, capacity int, allowedCodes ...int) Resource {
	if capacity < 0 {
		capacity = math.MaxInt16
	}
	return &average{
		static:   NewResourceStatic(method, url, delay, allowedCodes...).(static),
		capacity: int64(capacity),
	}
}

func (r *average) After() <-chan time.Time {
	delay := r.delay
	count := atomic.LoadInt64(&r.count)
	if count >= r.capacity {
		delay = time.Duration(atomic.LoadInt64(&r.sum) / count)
	}
	return time.After(delay)
}

func (r *average) Hook(*http.Request) func(*http.Response) {
	t := time.Now()
	return func(*http.Response) {
		d := time.Since(t)
		oldval := atomic.LoadInt64(&r.sum)
		newval := atomic.AddInt64(&r.sum, int64(d))
		count := atomic.AddInt64(&r.count, 1)
		// in case of overflow:
		// - calculate average value on capacity+1
		// - replace current sum and count with it
		if newval < 0 || count > r.capacity*2 {
			val := oldval / count * (r.capacity + 1)
			atomic.StoreInt64(&r.sum, val)
			atomic.StoreInt64(&r.count, r.capacity+1)
		}
	}
}

type percentiles struct {
	static
	percentile float64
	capacity   int64
	latencies  []time.Duration
	lock       sync.RWMutex
}

// NewResourcePercentiles returns new resource instance that dynamically adjust wait delay based on
// recieved successfull responses delays percentiles.
// Returned resource is starting to use dynamically adjusted wait delay only after capacity/2 calls,
// if more than provided capacity calls were recieved, first half of delay percentiles buffer will be flushed.
// Returned resource matches each request against both provided http method and full url regexp.
// Returned resource checks if response result http code is included in provided allowed codes,
// if it is not it returnes `ErrResourceUnexpectedResponseCode`.
func NewResourcePercentiles(method string, url *regexp.Regexp, delay time.Duration, percentile float64, capacity int, allowedCodes ...int) Resource {
	percentile = math.Abs(percentile)
	if percentile > 1.0 {
		percentile = 1.0
	}
	if capacity < 0 {
		capacity = math.MaxInt16
	}
	return &percentiles{
		static:     NewResourceStatic(method, url, delay, allowedCodes...).(static),
		percentile: percentile,
		capacity:   int64(capacity),
		latencies:  make([]time.Duration, 0, capacity+capacity/2),
	}
}

func (r *percentiles) After() <-chan time.Time {
	delay := r.delay
	r.lock.RLock()
	if l := int64(len(r.latencies)); l >= r.capacity/2 {
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

func (r *percentiles) Hook(*http.Request) func(*http.Response) {
	t := time.Now()
	return func(*http.Response) {
		d := time.Since(t)
		r.lock.Lock()
		r.latencies = append(r.latencies, d)
		// in case of overflow: just drop half of the buffer
		if int64(len(r.latencies)) >= r.capacity {
			r.latencies = r.latencies[r.capacity/2:]
		}
		r.lock.Unlock()
	}
}
