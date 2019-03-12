package ltsvreader

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/kazeburo/mackerel-plugin-axslog/axslog"
	"go.uber.org/zap"
)

// Reader struct
type Reader struct {
	bufscan   *bufio.Scanner
	logger    *zap.Logger
	ptimeKey  string
	statusKey string
}

// New :
func New(ir io.Reader, logger *zap.Logger, ptimeKey string, statusKey string) *Reader {
	bs := bufio.NewScanner(ir)
	return &Reader{bs, logger, ptimeKey, statusKey}
}

// ParseLTSV :
func ParseLTSV(d1 string) (map[string]string, error) {
	c := strings.Count(d1, "\t")
	if c == 0 {
		return nil, fmt.Errorf("No TABs in a log")
	}
	d := make(map[string]string, c+1)
	p1 := 0
	for {
		p2 := strings.Index(d1[p1:], "\t")
		if p2 < 0 {
			break
		}
		p3 := strings.Index(d1[p1:p1+p2], ":")
		if p3 < 0 {
			break
		}
		d[d1[p1:p1+p3]] = d1[p1+p3+1 : p1+p2]
		p1 += p2 + 1
	}
	return d, nil
}

// Parse :
func (r *Reader) Parse() (float64, int, error) {
	for r.bufscan.Scan() {
		d, err := ParseLTSV(r.bufscan.Text())
		if err != nil {
			r.logger.Warn("Failed to parse ltsv. continue", zap.Error(err))
			continue
		}
		_, exists := d[r.ptimeKey]
		if exists == false {
			r.logger.Warn("No ptime in ltsv. continue", zap.String("key", r.ptimeKey))
			continue
		}
		_, exists = d[r.statusKey]
		if exists == false {
			r.logger.Warn("No status in ltsv. continue", zap.String("key", r.statusKey))
			continue
		}
		ptime, err := axslog.SFloat64(d[r.ptimeKey])
		if err != nil {
			r.logger.Warn("Failed to convert ptime. continue", zap.Error(err))
			continue
		}
		status, err := axslog.SInt(d[r.statusKey])
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
