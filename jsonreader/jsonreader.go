package jsonreader

import (
	"bytes"

	"github.com/buger/jsonparser"
	"github.com/kazeburo/mackerel-plugin-axslog/axslog"
)

// Reader struct
type Reader struct {
	keys [][]string
}

// New :
func New(ptimeKey string, statusKeys []string) *Reader {
	keys := make([][]string, 0)
	keys = append(keys, []string{ptimeKey})
	for _, stKey := range statusKeys {
		keys = append(keys, []string{stKey})
	}
	return &Reader{keys}
}

var bHif = []byte("-")

// Parse :
func (r *Reader) Parse(data []byte) (int, []byte, []byte) {
	c := 0
	var pt []byte
	var st []byte
	stIndex := len(r.keys)
	jsonparser.EachKey(data, func(idx int, value []byte, vt jsonparser.ValueType, err error) {
		// `-` はskip
		if bytes.Equal(value, bHif) || len(value) == 0 {
			return
		}
		switch {
		case idx == 0:
			//ptime
			c = c | axslog.PtimeFlag
			pt = value
		case idx > 0:
			//status
			c = c | axslog.StatusFlag
			if idx < stIndex {
				stIndex = idx
				st = value
			}
		}
	}, r.keys...)
	return c, pt, st
}
