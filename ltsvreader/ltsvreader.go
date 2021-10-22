package ltsvreader

import (
	"bytes"

	"github.com/kazeburo/mackerel-plugin-axslog/axslog"
)

// Reader struct
type Reader struct {
	bytePtimeKey   []byte
	byteStatusKeys [][]byte
}

// New :
func New(ptimeKey string, statusKeys []string) *Reader {
	byteStatusKey := make([][]byte, 0)
	for _, stKey := range statusKeys {
		byteStatusKey = append(byteStatusKey, []byte(stKey))
	}
	return &Reader{[]byte(ptimeKey), byteStatusKey}
}

var bTab = []byte("\t")
var bCol = []byte(":")
var bHif = []byte("-")

// Parse :
func (r *Reader) Parse(d1 []byte) (int, []byte, []byte) {
	c := 0
	var pt []byte
	var st []byte
	p1 := 0
	dlen := len(d1)
	stIndex := len(r.byteStatusKeys)
PARSE_LTSV:
	for {
		if dlen == p1 {
			break
		}
		p2 := bytes.Index(d1[p1:], bTab)
		if p2 < 0 {
			p2 = dlen - p1 - 1
		}
		p3 := bytes.Index(d1[p1:p1+p2], bCol)
		if p3 < 0 {
			break
		}

		// `-` はskip
		if bytes.Equal(d1[p1+p3+1:p1+p2], bHif) {
			break
		}

		if bytes.Equal(d1[p1:p1+p3], r.bytePtimeKey) {
			pt = d1[p1+p3+1 : p1+p2]
			c = c | axslog.PtimeFlag
			if c == axslog.AllFlagOK {
				break PARSE_LTSV
			}
		}

		for idx, stKey := range r.byteStatusKeys {
			if bytes.Equal(d1[p1:p1+p3], stKey) {
				if idx < stIndex {
					stIndex = idx
					st = d1[p1+p3+1 : p1+p2]
				}
				if stIndex == 0 {
					c = c | axslog.StatusFlag
				}
				if c == axslog.AllFlagOK {
					break PARSE_LTSV
				}
			}
		}
		p1 += p2 + 1
	}
	return c, pt, st
}
