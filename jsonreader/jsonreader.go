package jsonreader

import (
	"github.com/buger/jsonparser"
	"github.com/kazeburo/mackerel-plugin-axslog/axslog"
	"go.uber.org/zap"
)

// Reader struct
type Reader struct {
	ptimeKey  []string
	statusKey []string
	logger    *zap.Logger
}

// New :
func New(ptimeKey, statusKey string, logger *zap.Logger) *Reader {
	return &Reader{[]string{ptimeKey}, []string{statusKey}, logger}
}

// Parse :
func (r *Reader) Parse(data []byte) (int, []byte, []byte) {
	c := 0
	var pt []byte
	var st []byte
	jsonparser.EachKey(data, func(idx int, value []byte, vt jsonparser.ValueType, err error) {
		switch idx {
		case 0:
			//ptime
			c = c | axslog.PtimeFlag
			pt = value
		case 1:
			//status
			c = c | axslog.StatusFlag
			st = value
		}
	}, r.ptimeKey, r.statusKey)
	return c, pt, st
}
