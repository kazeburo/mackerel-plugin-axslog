package ltsvreader

import (
	"bufio"
	"bytes"
	"io"

	"github.com/kazeburo/mackerel-plugin-axslog/axslog"
	"go.uber.org/zap"
)

var ptimeFlag = 1
var statusFlag = 2

// Reader struct
type Reader struct {
	bufscan       *bufio.Scanner
	logger        *zap.Logger
	ptimeKey      string
	statusKey     string
	bytePtimeKey  []byte
	byteStatusKey []byte
}

// New :
func New(ir io.Reader, logger *zap.Logger, ptimeKey string, statusKey string) *Reader {
	bs := bufio.NewScanner(ir)
	return &Reader{bs, logger, ptimeKey, statusKey, []byte(ptimeKey), []byte(statusKey)}
}

var bTab = []byte("\t")
var bCol = []byte(":")

// ParseLTSV :
func ParseLTSV(d1, ptimeKey, statusKey []byte) (int, []byte, []byte) {
	c := 0
	var pt []byte
	var st []byte
	p1 := 0
	for {
		p2 := bytes.Index(d1[p1:], bTab)
		if p2 < 0 {
			break
		}
		p3 := bytes.Index(d1[p1:p1+p2], bCol)
		if p3 < 0 {
			break
		}
		if bytes.Equal(d1[p1:p1+p3], ptimeKey) {
			pt = d1[p1+p3+1 : p1+p2]
			c = c | ptimeFlag
		}

		if bytes.Equal(d1[p1:p1+p3], statusKey) {
			st = d1[p1+p3+1 : p1+p2]
			c = c | statusFlag
		}
		p1 += p2 + 1
	}
	return c, pt, st
}

// Parse :
func (r *Reader) Parse() (float64, int, error) {
	for r.bufscan.Scan() {
		c, pt, st := ParseLTSV(r.bufscan.Bytes(), r.bytePtimeKey, r.byteStatusKey)
		if c&ptimeFlag == 0 {
			r.logger.Warn("No ptime in ltsv. continue", zap.String("key", r.ptimeKey))
			continue
		}
		if c&statusFlag == 0 {
			r.logger.Warn("No status in ltsv. continue", zap.String("key", r.statusKey))
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
