<p align="center">
    <img src="https://raw.githubusercontent.com/1pkg/hedgehog/master/gopher.png" alt="gohalt"/>
</p>

# Hedgehog 🦔: http hedged transport in go

[![lint](https://github.com/1pkg/hedgehog/workflows/lint/badge.svg)](https://github.com/1pkg/hedgehog/actions?query=workflow%3Alint+branch%3Amaster+)
[![build](https://github.com/1pkg/hedgehog/workflows/build/badge.svg)](https://github.com/1pkg/hedgehog/actions?query=workflow%3Abuild+branch%3Amaster+)
[![report](https://goreportcard.com/badge/github.com/1pkg/hedgehog?nocache)](https://goreportcard.com/report/github.com/1pkg/hedgehog)
[![version](https://img.shields.io/github/go-mod/go-version/1pkg/hedgehog?nocache)](https://github.com/1pkg/hedgehog/blob/master/go.mod)
[![license](https://img.shields.io/github/license/1pkg/hedgehog?nocache)](LICENSE)

`go get -u github.com/1pkg/hedgehog`

## Details

Hedgehog provides hedged http transport decorator to reduce [tail latency at scale](https://cacm.acm.org/magazines/2013/2/160173-the-tail-at-scale/fulltext). Hedged transport makes hedged http calls for matching http resource up to calls+1 times, where initial http call starts right away and then all hedged calls start together after delay calculated by resource. Hedged transport processes and returns first successful http response all other requests in flight are canceled, in case all hedged response failed it simply returns first occurred error. If no matching resources were found - hedged transport simply calls underlying transport.

```go
hedgehog.NewHTTPClient(
    http.DefaultClient,
    // will initiate 2+1 hedged http request.
    2,
    // for GET /profile/[0-9] initiate hedged request only after flat 1ms.
    NewResourceStatic(http.MethodGet, regexp.MustCompile(`profile/[0-9]`), ms_1, http.StatusOK),
    // for POST /profile initiate hedged request starting with flat 5ms, but after 40/4 calls use aggregated average latency.
    NewResourceAverage(http.MethodPost, regexp.MustCompile(`profile`), ms_5, 40, http.StatusOK),
    // for Delete /profile initiate hedged request starting with flat 5ms, but after 50/2 calls use aggregated p30 latency.
    NewResourcePercentiles(http.MethodDelete, regexp.MustCompile(`profile`), ms_5, 0.3, 50, http.StatusOK),
).Get("http://example.com/profile/5")
```

There are multiple different http hedged resource types to control hedging behavior.

| Resource | Definition | Description |
|---|---|---|
| static | `func NewResourceStatic(method string, url *regexp.Regexp, delay time.Duration, allowedCodes ...int) Resource` | Returned resource always waits for static specified delay.<br> The resource matches each request against both provided http method and full url regexp.<br> The resource checks if response result http code is included in provided allowed codes, if it is not it returnes `ErrResourceUnexpectedResponseCode`. |
| average | `func NewResourceAverage(method string, url *regexp.Regexp, delay time.Duration, capacity int, allowedCodes ...int) Resource` | Returned resource dynamically adjusts wait delay based on received successful responses average delays.<br> The resource is starting to use dynamically adjusted wait delay only after capacity/4 calls.<br> The resource matches each request against both provided http method and full url regexp.<br> The resource checks if response result http code is included in provided allowed codes, if it is not it returnes `ErrResourceUnexpectedResponseCode`. |
| percentiles | `func NewResourcePercentiles(method string, url *regexp.Regexp, delay time.Duration, percentile float64, capacity int, allowedCodes ...int) Resource` | Returned resource dynamically adjusts wait delay based on received successful responses delays percentiles.<br> The resource is starting to use dynamically adjusted wait delay only after capacity/2 calls, if more than provided capacity calls were received, first half of delay percentiles buffer will be flushed.<br> Returned resource matches each request against both provided http method and full url regexp.<br> Returned resource checks if response result http code is included in provided allowed codes, if it is not it returnes `ErrResourceUnexpectedResponseCode`. |

## Licence

Hedgehog is licensed under the MIT License.  
See [LICENSE](LICENSE) for the full license text.
