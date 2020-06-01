package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	flags "github.com/jessevdk/go-flags"
	"github.com/kazeburo/mackerel-plugin-axslog/axslog"
	"github.com/kazeburo/mackerel-plugin-axslog/jsonreader"
	"github.com/kazeburo/mackerel-plugin-axslog/ltsvreader"
	"github.com/kazeburo/mackerel-plugin-axslog/posreader"
	"go.uber.org/zap"

	"github.com/pkg/errors"
)

// Version by Makefile
var Version string

// MaxReadSizeJSON : Maximum size for read
var MaxReadSizeJSON int64 = 500 * 1000 * 1000

// MaxReadSizeLTSV : Maximum size for read
var MaxReadSizeLTSV int64 = 1000 * 1000 * 1000

var f0 = float64(0)

type cmdOpts struct {
	LogFile   string `long:"logfile" description:"path to nginx ltsv logfiles. multiple log files can be specified, separated by commas." required:"true"`
	Format    string `long:"format" default:"ltsv" description:"format of logfile. support json and ltsv"`
	KeyPrefix string `long:"key-prefix" description:"Metric key prefix" required:"true"`
	PtimeKey  string `long:"ptime-key" default:"ptime" description:"key name for request_time"`
	StatusKey string `long:"status-key" default:"status" description:"key name for response status"`
	Filter    string `long:"filter" default:"" description:"text for filtering log"`
	Version   bool   `short:"v" long:"version" description:"Show version"`
}

// parseLog :
func parseLog(bs *bufio.Scanner, r axslog.Reader, filter []byte, ptimeKey, statusKey string, logger *zap.Logger) (float64, int, error) {
	for bs.Scan() {
		b := bs.Bytes()
		if len(filter) > 0 {
			if bytes.Index(b, filter) < 0 {
				continue
			}
		}
		c, pt, st := r.Parse(b)
		if c&axslog.PtimeFlag == 0 {
			logger.Warn("No ptime. continue", zap.String("key", ptimeKey))
			continue
		}
		if c&axslog.StatusFlag == 0 {
			logger.Warn("No status. continue", zap.String("key", statusKey))
			continue
		}
		ptime, err := axslog.BFloat64(pt)
		if err != nil {
			logger.Warn("Failed to convert ptime. continue", zap.Error(err))
			continue
		}
		status, err := axslog.BInt(st)
		if err != nil {
			logger.Warn("Failed to convert status. continue", zap.Error(err))
			continue
		}
		return ptime, status, nil
	}
	if bs.Err() != nil {
		return float64(0), int(0), bs.Err()
	}
	return float64(0), int(0), io.EOF
}

// parseFile :
func parseFile(logFile string, lastPos int64, format, filter, ptimeKey, statusKey, posFile string, stats *axslog.Stats, logger *zap.Logger) (float64, error) {
	maxReadSize := int64(0)
	switch format {
	case "ltsv":
		maxReadSize = MaxReadSizeLTSV
	case "json":
		maxReadSize = MaxReadSizeJSON
	default:
		return f0, fmt.Errorf("format %s is not supported", format)
	}

	stat, err := os.Stat(logFile)
	if err != nil {
		return f0, errors.Wrap(err, "failed to stat log file")
	}

	fstat, err := axslog.FileStat(stat)
	if err != nil {
		return f0, errors.Wrap(err, "failed to inode of log file")
	}

	logger.Info("Analysis start",
		zap.String("logFile", logFile),
		zap.Int64("lastPos", lastPos),
		zap.Int64("Size", stat.Size()),
	)

	if lastPos == 0 && stat.Size() > maxReadSize {
		// first time and big logile
		lastPos = stat.Size()
	}

	if stat.Size()-lastPos > maxReadSize {
		// big delay
		lastPos = stat.Size()
	}

	f, err := os.Open(logFile)
	if err != nil {
		return f0, errors.Wrap(err, "failed to open log file")
	}
	defer f.Close()
	fpr, err := posreader.New(f, lastPos)
	if err != nil {
		return f0, errors.Wrap(err, "failed to seek log file")
	}

	var ar axslog.Reader
	switch format {
	case "ltsv":
		ar = ltsvreader.New(ptimeKey, statusKey, logger)
	case "json":
		ar = jsonreader.New(ptimeKey, statusKey, logger)
	}

	total := 0
	bs := bufio.NewScanner(fpr)
	fb := []byte(filter)
	for {
		ptime, status, errb := parseLog(bs, ar, fb, ptimeKey, statusKey, logger)
		if errb == io.EOF {
			break
		}
		if errb != nil {
			return f0, errors.Wrap(errb, "Something wrong in parse log")
		}
		stats.Append(ptime, status)
		total++
	}

	logger.Info("Analysis completed",
		zap.String("logFile", logFile),
		zap.Int64("startPos", lastPos),
		zap.Int64("endPos", fpr.Pos),
		zap.Int("Rows", total),
	)
	// postionのupdate
	endTime := float64(0)
	if posFile != "" {
		endTime, err = axslog.WritePos(posFile, fpr.Pos, fstat)
		if err != nil {
			return endTime, errors.Wrap(err, "failed to update pos file")
		}
	}
	return endTime, nil
}

func getStats(opts cmdOpts, logger *zap.Logger) error {
	tmpDir := os.TempDir()
	curUser, _ := user.Current()
	uid := "0"
	if curUser != nil {
		uid = curUser.Uid
	}

	logfiles := strings.Split(opts.LogFile, ",")

	if len(logfiles) == 1 {
		posFile := filepath.Join(tmpDir, fmt.Sprintf("%s-axslog-v4-%s", uid, opts.KeyPrefix))
		stats, err := getStatsFile(opts, posFile, opts.LogFile, logger)
		if err != nil {
			return err
		}
		stats.Display(opts.KeyPrefix)
		return nil
	}

	sCh := make(chan axslog.StatsCh, len(logfiles))
	defer close(sCh)
	for _, l := range logfiles {
		logfile := l
		go func() {
			md5 := md5.Sum([]byte(logfile))
			posFile := filepath.Join(tmpDir, fmt.Sprintf("%s-axslog-v4-%s-%x", uid, opts.KeyPrefix, md5))
			stats, err := getStatsFile(opts, posFile, logfile, logger)
			sCh <- axslog.StatsCh{stats, logfile, err}
		}()
	}
	errCnt := 0
	var statsAll []*axslog.Stats
	for range logfiles {
		s := <-sCh
		if s.Err != nil {
			errCnt++
			if len(logfiles) == errCnt {
				return s.Err
			}
			// warnings and ignore
			logger.Warn("getStats", zap.String("file", s.Logfile), zap.Error(s.Err))
		} else {
			statsAll = append(statsAll, s.Stats)
		}
	}

	axslog.DisplayAll(statsAll, opts.KeyPrefix)
	return nil
}

func getStatsFile(opts cmdOpts, posFile, logFile string, logger *zap.Logger) (*axslog.Stats, error) {
	stats := axslog.NewStats()
	lastPos := int64(0)
	lastFstat := &axslog.FStat{}
	startTime := float64(0)
	endTime := float64(0)

	if axslog.FileExists(posFile) {
		l, s, f, err := axslog.ReadPos(posFile)
		if err != nil {
			return stats, errors.Wrap(err, "failed to load pos file")
		}
		lastPos = l
		startTime = s
		lastFstat = f
	}

	stat, err := os.Stat(logFile)
	if err != nil {
		return stats, errors.Wrap(err, "failed to stat log file")
	}
	fstat, err := axslog.FileStat(stat)
	if err != nil {
		return stats, errors.Wrap(err, "failed to get inode from log file")
	}
	if fstat.IsNotRotated(lastFstat) {
		endTime, err = parseFile(
			logFile,
			lastPos,
			opts.Format,
			opts.Filter,
			opts.PtimeKey,
			opts.StatusKey,
			posFile,
			stats,
			logger,
		)
		if err != nil {
			return stats, err
		}
	} else {
		// rotate!!
		logger.Info("Detect Rotate")
		lastFile, err := axslog.SearchFileByInode(filepath.Dir(logFile), lastFstat)
		if err != nil {
			logger.Warn("Could not search previous file",
				zap.Error(err),
			)
			// new file
			endTime, err = parseFile(
				logFile,
				0, // lastPos
				opts.Format,
				opts.Filter,
				opts.PtimeKey,
				opts.StatusKey,
				posFile,
				stats,
				logger,
			)
			if err != nil {
				return stats, err
			}
		} else {
			// new file
			endTime, err = parseFile(
				logFile,
				0, // lastPos
				opts.Format,
				opts.Filter,
				opts.PtimeKey,
				opts.StatusKey,
				posFile,
				stats,
				logger,
			)
			if err != nil {
				return stats, err
			}
			// previous file
			_, err = parseFile(
				lastFile,
				lastPos,
				opts.Format,
				opts.Filter,
				opts.PtimeKey,
				opts.StatusKey,
				"", // no update posfile
				stats,
				logger,
			)
			if err != nil {
				logger.Warn("Could not parse previous file",
					zap.Error(err),
				)
			}
		}
	}
	stats.SetDuration(endTime - startTime)
	return stats, nil
}

func printVersion() {
	fmt.Printf(`%s %s
Compiler: %s %s
`,
		os.Args[0],
		Version,
		runtime.Compiler,
		runtime.Version())
}

func main() {
	os.Exit(_main())
}

func _main() int {
	opts := cmdOpts{}
	psr := flags.NewParser(&opts, flags.Default)
	_, err := psr.Parse()
	if err != nil {
		return 1
	}
	if opts.Version {
		printVersion()
		return 0
	}

	logger, _ := zap.NewProduction()
	err = getStats(opts, logger)
	if err != nil {
		logger.Error("getStats", zap.Error(err))
		return 1
	}
	return 0
}
