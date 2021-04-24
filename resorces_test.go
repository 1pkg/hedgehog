package hedgehog

import (
	"net/http"
	"regexp"
	"testing"
	"time"
)

//nolint
const (
	ms_0   = time.Millisecond * 0
	ms_1   = time.Millisecond * 1
	ms_2   = time.Millisecond * 2
	ms_5   = time.Millisecond * 5
	ms_8   = time.Millisecond * 8
	ms_10  = time.Millisecond * 10
	ms_20  = time.Millisecond * 20
	ms_50  = time.Millisecond * 50
	ms_100 = time.Millisecond * 100
)

func TestResorces(t *testing.T) {
	ttable := map[string]struct {
		res    Resource
		delays []time.Duration
		after  time.Duration
	}{
		"static resource after should not be changed": {
			res:    NewResourceStatic("", regexp.MustCompile(``), ms_1, 0),
			delays: []time.Duration{ms_5, ms_5, ms_5, ms_5, ms_5},
			after:  ms_2,
		},
		"average resource after should be using static before saturation": {
			res:    NewResourceAverage("", regexp.MustCompile(``), ms_1, -1, 0),
			delays: []time.Duration{ms_1, ms_2, ms_5, ms_10, ms_20},
			after:  ms_2,
		},
		"average resource after should be adjusted accurately": {
			res:    NewResourceAverage("", regexp.MustCompile(``), ms_1, 8, 0),
			delays: []time.Duration{ms_1, ms_2, ms_5, ms_10, ms_20},
			after:  ms_10,
		},
		"average resource after should be adjusted accurately with overflow": {
			res:    NewResourceAverage("", regexp.MustCompile(``), ms_1, 3, 0),
			delays: []time.Duration{ms_1, ms_2, ms_5, ms_10, ms_20, ms_10, ms_1, ms_1, ms_1, ms_1, ms_1, ms_1},
			after:  ms_5,
		},
		"percentiles resource after should be using static before saturation": {
			res:    NewResourcePercentiles("", regexp.MustCompile(``), ms_1, 1.2, -1, 0),
			delays: []time.Duration{ms_5, ms_5, ms_5, ms_5, ms_5},
			after:  ms_2,
		},
		"percentiles resource after should be adjusted accurately": {
			res:    NewResourcePercentiles("", regexp.MustCompile(``), ms_1, 0.9, 8, 0),
			delays: []time.Duration{ms_5, ms_5, ms_5, ms_5, ms_5},
			after:  ms_8,
		},
		"percentiles resource after should be adjusted accurately with overflow": {
			res:    NewResourcePercentiles("", regexp.MustCompile(``), ms_1, 0.9, 10, 0),
			delays: []time.Duration{ms_5, ms_5, ms_5, ms_5, ms_5, ms_10, ms_10, ms_10, ms_10, ms_10, ms_10},
			after:  ms_20,
		},
	}
	for tname, tcase := range ttable {
		tcase := tcase
		t.Run(tname, func(t *testing.T) {
			t.Parallel()
			for _, d := range tcase.delays {
				h := tcase.res.Hook(&http.Request{})
				time.Sleep(d)
				h(&http.Response{})
			}
			ts := time.Now()
			<-tcase.res.After()
			ds := time.Since(ts)
			if tcase.after < ds {
				t.Fatalf("expected resource after time should be < %s but got %s", tcase.after, ds)
			}
		})
	}
}
