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

type average struct {
	static
	sum      int64
	count    int64
	capacity int64
}

func NewResourceAverage(method string, path *regexp.Regexp, delay time.Duration, capacity int, allowedCodes ...int) Resource {
	if capacity < 0 {
		capacity = math.MaxInt16
	}
	return &average{
		static:   NewResourceStatic(method, path, delay, allowedCodes...).(static),
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

func NewResourcePercentiles(method string, path *regexp.Regexp, delay time.Duration, percentile float64, capacity int, allowedCodes ...int) Resource {
	percentile = math.Abs(percentile)
	if percentile > 1.0 {
		percentile = 1.0
	}
	if capacity < 0 {
		capacity = math.MaxInt16
	}
	return &percentiles{
		static:     NewResourceStatic(method, path, delay, allowedCodes...).(static),
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
