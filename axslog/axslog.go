package axslog

import (
	"strconv"
	"unsafe"
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

// BFloat64 :
func BFloat64(b []byte) (float64, error) {
	return strconv.ParseFloat(*(*string)(unsafe.Pointer(&b)), 64)
}

// BInt :
func BInt(b []byte) (int, error) {
	return strconv.Atoi(*(*string)(unsafe.Pointer(&b)))
}
