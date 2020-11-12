package ltsvreader

import (
	"bytes"

	"github.com/kazeburo/mackerel-plugin-axslog/axslog"
)

// Reader struct
type Reader struct {
	bytePtimeKey  []byte
	byteStatusKey []byte
}

// New :
func New(ptimeKey, statusKey string) *Reader {
	return &Reader{[]byte(ptimeKey), []byte(statusKey)}
}

var bTab = []byte("\t")
var bCol = []byte(":")

// Parse :
func (r *Reader) Parse(d1 []byte) (int, []byte, []byte) {
	c := 0
	var pt []byte
	var st []byte
	p1 := 0
	dlen := len(d1)
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
		if bytes.Equal(d1[p1:p1+p3], r.bytePtimeKey) {
			pt = d1[p1+p3+1 : p1+p2]
			c = c | axslog.PtimeFlag
			if c == axslog.AllFlagOK {
				break
			}
		}

		if bytes.Equal(d1[p1:p1+p3], r.byteStatusKey) {
			st = d1[p1+p3+1 : p1+p2]
			c = c | axslog.StatusFlag
			if c == axslog.AllFlagOK {
				break
			}
		}
		p1 += p2 + 1
	}
	return c, pt, st
}
