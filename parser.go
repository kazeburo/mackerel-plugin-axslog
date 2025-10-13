package main

import (
	"bytes"
	"log"

	"github.com/kazeburo/mackerel-plugin-axslog/axslog"
	"github.com/kazeburo/mackerel-plugin-axslog/jsonreader"
	"github.com/kazeburo/mackerel-plugin-axslog/ltsvreader"
)

type parser struct {
	opts   *axslog.CmdOpts
	stats  *axslog.Stats
	ar     axslog.Reader
	filter []byte
}

func NewParser(opts *axslog.CmdOpts, stats *axslog.Stats) *parser {

	var ar axslog.Reader
	switch opts.Format {
	case "ltsv":
		ar = ltsvreader.New(opts.PtimeKey, opts.StatusKeys)
	case "json":
		ar = jsonreader.New(opts.PtimeKey, opts.StatusKeys)
	}

	p := &parser{
		opts:  opts,
		stats: stats,
		ar:    ar,
	}

	if opts.Filter != "" {
		p.filter = []byte(opts.Filter)
	}

	return p
}

func (p *parser) Parse(b []byte) error {
	if p.filter != nil {
		if p.opts.InvertFilter {
			if bytes.Contains(b, p.filter) {
				return nil
			}
		} else {
			if !bytes.Contains(b, p.filter) {
				return nil
			}
		}
	}
	if p.opts.SkipUntilBracket {
		i := bytes.IndexByte(b, '{')
		if i >= 0 {
			b = b[i:]
		}
	}
	c, pt, st := p.ar.Parse(b)
	if c&axslog.PtimeFlag == 0 {
		log.Printf("No ptime. continue key:%s", p.opts.PtimeKey)
		return nil
	}
	if c&axslog.StatusFlag == 0 {
		log.Printf("No status. continue key:%v", p.opts.StatusKeys)
		return nil
	}
	ptime, err := axslog.BFloat64(pt)
	if err != nil {
		log.Printf("Failed to convert ptime. continue: %v", err)
		return nil
	}
	status, err := axslog.BInt(st)
	if err != nil {
		log.Printf("Failed to convert status. continue: %v", err)
		return nil
	}
	p.stats.Append(ptime, status)
	return nil
}

func (p *parser) Finish(duration float64) {
	p.stats.SetDuration(duration)
}
