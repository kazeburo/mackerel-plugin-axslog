package axslog

import (
	"strconv"
)

// Reader :
type Reader interface {
	Parse() (float64, int, error)
}

// SFloat64 :
func SFloat64(val string) (float64, error) {
	return strconv.ParseFloat(val, 64)
}

// SInt :
func SInt(val string) (int, error) {
	return strconv.Atoi(val)
}
