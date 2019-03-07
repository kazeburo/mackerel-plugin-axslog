package axslog

import (
	"fmt"
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

// Cloat64 :
func CFloat64(x interface{}) (float64, error) {
	switch xi := x.(type) {
	case float64:
		return xi, nil
	case string:
		return SFloat64(xi)
	}
	return float64(0), fmt.Errorf("Failed to cast to float64")
}

// CInt :
func CInt(x interface{}) (int, error) {
	switch xi := x.(type) {
	case int:
		return xi, nil
	case float64:
		return int(xi), nil
	case string:
		return SInt(xi)
	}
	return int(0), fmt.Errorf("Failed to cast to int")
}
