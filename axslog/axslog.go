package axslog

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"
	"unsafe"
)

// Reader :
type Reader interface {
	Parse() (float64, int, error)
}

// Stats :
type Stats struct {
	f64s  sort.Float64Slice
	tf    float64
	c1xx  float64
	c2xx  float64
	c3xx  float64
	c4xx  float64
	c499  float64
	c5xx  float64
	total float64
}

func round(f float64) int64 {
	return int64(math.Round(f)) - 1
}

func statusCode(status int) int {
	switch status {
	case 499:
		return 499
	default:
		return status / 100
	}
}

// NewStats :
func NewStats() *Stats {
	return &Stats{}
}

// GetTotal :
func (s *Stats) GetTotal() float64 {
	return s.total
}

// Append :
func (s *Stats) Append(ptime float64, status int) {
	switch statusCode(status) {
	case 2:
		s.c2xx++
	case 3:
		s.c3xx++
	case 4:
		s.c4xx++
	case 5:
		s.c5xx++
	case 499:
		s.c499++
	case 1:
		s.c1xx++
	}
	s.total++

	s.f64s = append(s.f64s, ptime)
	s.tf += ptime

}

// Display :
func (s *Stats) Display(keyPrefix string, duration float64) {
	now := uint64(time.Now().Unix())
	sort.Sort(s.f64s)
	fl := float64(len(s.f64s))
	// fmt.Printf("count: %d\n", len(f64s))
	if len(s.f64s) > 0 {
		fmt.Printf("axslog.latency_%s.average\t%f\t%d\n", keyPrefix, s.tf/fl, now)
		fmt.Printf("axslog.latency_%s.99_percentile\t%f\t%d\n", keyPrefix, s.f64s[round(fl*0.99)], now)
		fmt.Printf("axslog.latency_%s.95_percentile\t%f\t%d\n", keyPrefix, s.f64s[round(fl*0.95)], now)
		fmt.Printf("axslog.latency_%s.90_percentile\t%f\t%d\n", keyPrefix, s.f64s[round(fl*0.90)], now)
	}

	if duration > 0 {
		fmt.Printf("axslog.access_num_%s.1xx_count\t%f\t%d\n", keyPrefix, s.c1xx/duration, now)
		fmt.Printf("axslog.access_num_%s.2xx_count\t%f\t%d\n", keyPrefix, s.c2xx/duration, now)
		fmt.Printf("axslog.access_num_%s.3xx_count\t%f\t%d\n", keyPrefix, s.c3xx/duration, now)
		fmt.Printf("axslog.access_num_%s.4xx_count\t%f\t%d\n", keyPrefix, s.c4xx/duration, now)
		fmt.Printf("axslog.access_num_%s.499_count\t%f\t%d\n", keyPrefix, s.c499/duration, now)
		fmt.Printf("axslog.access_num_%s.5xx_count\t%f\t%d\n", keyPrefix, s.c5xx/duration, now)
		fmt.Printf("axslog.access_total_%s.count\t%f\t%d\n", keyPrefix, s.total/duration, now)
	}
	if s.total > 0 {
		fmt.Printf("axslog.access_ratio_%s.1xx_percentage\t%f\t%d\n", keyPrefix, s.c1xx/s.total, now)
		fmt.Printf("axslog.access_ratio_%s.2xx_percentage\t%f\t%d\n", keyPrefix, s.c2xx/s.total, now)
		fmt.Printf("axslog.access_ratio_%s.3xx_percentage\t%f\t%d\n", keyPrefix, s.c3xx/s.total, now)
		fmt.Printf("axslog.access_ratio_%s.4xx_percentage\t%f\t%d\n", keyPrefix, s.c4xx/s.total, now)
		fmt.Printf("axslog.access_ratio_%s.499_percentage\t%f\t%d\n", keyPrefix, s.c499/s.total, now)
		fmt.Printf("axslog.access_ratio_%s.5xx_percentage\t%f\t%d\n", keyPrefix, s.c5xx/s.total, now)
	}
}

// SFloat64 :
func SFloat64(val string) (float64, error) {
	return strconv.ParseFloat(val, 64)
}

// SInt :
func SInt(val string) (int, error) {
	return strconv.Atoi(val)
}

// BFloat64 :
func BFloat64(b []byte) (float64, error) {
	return strconv.ParseFloat(*(*string)(unsafe.Pointer(&b)), 64)
}

// BInt :
func BInt(b []byte) (int, error) {
	return strconv.Atoi(*(*string)(unsafe.Pointer(&b)))
}
