package jsonreader

import (
	"bufio"
	"encoding/json"
	"io"

	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
)

// Reader struct
type Reader struct {
	bufscan   *bufio.Scanner
	logger    *zap.Logger
	json      jsoniter.API
	ptimeKey  string
	statusKey string
}

// New :
func New(ir io.Reader, logger *zap.Logger, ptimeKey string, statusKey string) *Reader {
	bs := bufio.NewScanner(ir)
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	return &Reader{bs, logger, json, ptimeKey, statusKey}
}

// Parse :
func (r *Reader) Parse() (float64, int, error) {
	for r.bufscan.Scan() {
		var d map[string]json.Number
		err := r.json.Unmarshal(r.bufscan.Bytes(), &d)
		if err != nil {
			r.logger.Warn("Failed to parse json. continue", zap.Error(err))
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
		ptime, err := d[r.ptimeKey].Float64()
		if err != nil {
			r.logger.Warn("Failed to convert ptime. continue", zap.Error(err))
			continue
		}
		status, err := d[r.statusKey].Int64()
		if err != nil {
			r.logger.Warn("Failed to convert status. continue", zap.Error(err))
			continue
		}
		return ptime, int(status), nil
	}
	if r.bufscan.Err() != nil {
		return float64(0), int(0), r.bufscan.Err()
	}
	return float64(0), int(0), io.EOF
}
