package ltsvreader

import (
	"io"

	"github.com/kazeburo/mackerel-plugin-axslog/axslog"
	"github.com/najeira/ltsv"
	"go.uber.org/zap"
)

// Reader struct
type Reader struct {
	ltsv      *ltsv.Reader
	logger    *zap.Logger
	ptimeKey  string
	statusKey string
}

// New :
func New(ir io.Reader, logger *zap.Logger, ptimeKey string, statusKey string) *Reader {
	lr := ltsv.NewReader(ir)
	return &Reader{lr, logger, ptimeKey, statusKey}
}

// Parse :
func (r *Reader) Parse() (float64, int, error) {
	for {
		d, err := r.ltsv.Read()
		if err == io.EOF {
			return float64(0), int(0), err
		}
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
}
