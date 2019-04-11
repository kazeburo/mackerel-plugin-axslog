package jsonreader

import (
	"bufio"
	"io"

	"github.com/buger/jsonparser"
	"github.com/kazeburo/mackerel-plugin-axslog/axslog"
	"go.uber.org/zap"
)

var ptimeFlag = 1
var statusFlag = 2

// Reader struct
type Reader struct {
	bufscan   *bufio.Scanner
	logger    *zap.Logger
	ptimeKey  []string
	statusKey []string
}

// New :
func New(ir io.Reader, logger *zap.Logger, ptimeKey string, statusKey string) *Reader {
	bs := bufio.NewScanner(ir)
	return &Reader{bs, logger, []string{ptimeKey}, []string{statusKey}}
}

func (r *Reader) parseJSON(data []byte) (int, []byte, []byte) {
	c := 0
	var pt []byte
	var st []byte
	jsonparser.EachKey(data, func(idx int, value []byte, vt jsonparser.ValueType, err error) {
		switch idx {
		case 0:
			//ptime
			c = c | ptimeFlag
			pt = value
		case 1:
			//status
			c = c | statusFlag
			st = value
		}
	}, r.ptimeKey, r.statusKey)
	return c, pt, st
}

// Parse :
func (r *Reader) Parse() (float64, int, error) {
	for r.bufscan.Scan() {
		c, pt, st := r.parseJSON(r.bufscan.Bytes())
		if c&ptimeFlag == 0 {
			r.logger.Warn("No ptime in json. continue", zap.String("key", r.ptimeKey[0]))
			continue
		}
		if c&statusFlag == 0 {
			r.logger.Warn("No status in json. continue", zap.String("key", r.statusKey[0]))
			continue
		}
		ptime, err := axslog.BFloat64(pt)
		if err != nil {
			r.logger.Warn("Failed to convert ptime. continue", zap.Error(err))
			continue
		}
		status, err := axslog.BInt(st)
		if err != nil {
			r.logger.Warn("Failed to convert status. continue", zap.Error(err))
			continue
		}
		return ptime, status, nil
	}
	if r.bufscan.Err() != nil {
		return float64(0), int(0), r.bufscan.Err()
	}
	return float64(0), int(0), io.EOF
}
