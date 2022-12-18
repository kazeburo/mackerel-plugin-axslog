package ltsvreader

import (
	"bytes"

	"github.com/kazeburo/ltsvparser"
	"github.com/kazeburo/mackerel-plugin-axslog/axslog"
)

// Reader struct
type Reader struct {
	keys [][]byte
}

// New :
func New(ptimeKey string, statusKeys []string) *Reader {
	keys := make([][]byte, 0)
	keys = append(keys, []byte(ptimeKey))
	for _, stKey := range statusKeys {
		keys = append(keys, []byte(stKey))
	}
	return &Reader{keys}
}

var bHif = []byte("-")

// Parse
func (r *Reader) Parse(data []byte) (int, []byte, []byte) {
	c := 0
	var pt []byte
	var st []byte
	stIndex := len(r.keys)
	ltsvparser.Each(data, func(idx int, value []byte) error {
		// `-` はskip
		if bytes.Equal(value, bHif) || len(value) == 0 {
			return nil
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
		if c == axslog.AllFlagOK {
			return ltsvparser.Cancel
		}
		return nil
	}, r.keys...)
	return c, pt, st

}
