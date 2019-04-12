package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"runtime"

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

type cmdOpts struct {
	LogFile   string `long:"logfile" description:"path to nginx ltsv logfile" required:"true"`
	Format    string `long:"format" default:"ltsv" description:"format of logfile. support json and ltsv"`
	KeyPrefix string `long:"key-prefix" description:"Metric key prefix" required:"true"`
	PtimeKey  string `long:"ptime-key" default:"ptime" description:"key name for request_time"`
	StatusKey string `long:"status-key" default:"status" description:"key name for response status"`
	Version   bool   `short:"v" long:"version" description:"Show version"`
}

// Parse :
func parseLog(bs *bufio.Scanner, r axslog.Reader, ptimeKey, statusKey string, logger *zap.Logger) (float64, int, error) {
	for bs.Scan() {
		c, pt, st := r.Parse(bs.Bytes())
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

func parseFile(logFile string, lastPos int64, format, ptimeKey, statusKey, posFile string, stats *axslog.Stats, logger *zap.Logger) error {
	maxReadSize := int64(0)
	switch format {
	case "ltsv":
		maxReadSize = MaxReadSizeLTSV
	case "json":
		maxReadSize = MaxReadSizeJSON
	default:
		return fmt.Errorf("format %s is not supported", format)
	}

	stat, err := os.Stat(logFile)
	if err != nil {
		return errors.Wrap(err, "failed to stat log file")
	}

	fstat, err := axslog.FileStat(stat)
	if err != nil {
		return errors.Wrap(err, "failed to inode of log file")
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
		return errors.Wrap(err, "failed to open log file")
	}
	defer f.Close()
	fpr, err := posreader.New(f, lastPos)
	if err != nil {
		return errors.Wrap(err, "failed to seek log file")
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
	for {
		ptime, status, errb := parseLog(bs, ar, ptimeKey, statusKey, logger)
		if errb == io.EOF {
			break
		}
		if errb != nil {
			return errors.Wrap(errb, "Something wrong in parse log")
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
	if posFile != "" {
		err = axslog.WritePos(posFile, fpr.Pos, fstat)
		if err != nil {
			return errors.Wrap(err, "failed to update pos file")
		}
	}
	return nil
}

func getStats(opts cmdOpts, logger *zap.Logger) error {
	lastPos := int64(0)
	lastFstat := &axslog.FStat{}
	tmpDir := os.TempDir()
	curUser, _ := user.Current()
	uid := "0"
	if curUser != nil {
		uid = curUser.Uid
	}
	posFile := filepath.Join(tmpDir, fmt.Sprintf("%s-axslog-v4-%s", uid, opts.KeyPrefix))
	duration := float64(0)
	stats := axslog.NewStats()

	if axslog.FileExists(posFile) {
		l, d, f, err := axslog.ReadPos(posFile)
		if err != nil {
			return errors.Wrap(err, "failed to load pos file")
		}
		lastPos = l
		duration = d
		lastFstat = f
	}

	stat, err := os.Stat(opts.LogFile)
	if err != nil {
		return errors.Wrap(err, "failed to stat log file")
	}
	fstat, err := axslog.FileStat(stat)
	if err != nil {
		return errors.Wrap(err, "failed to get inode from log file")
	}
	if fstat.IsNotRotated(lastFstat) {
		err := parseFile(
			opts.LogFile,
			lastPos,
			opts.Format,
			opts.PtimeKey,
			opts.StatusKey,
			posFile,
			stats,
			logger,
		)
		if err != nil {
			return err
		}
	} else {
		// rotate!!
		logger.Info("Detect Rotate")
		lastFile, err := axslog.SearchFileByInode(filepath.Dir(opts.LogFile), lastFstat)
		if err != nil {
			logger.Warn("Could not search previous file",
				zap.Error(err),
			)
		} else {
			// new file
			err := parseFile(
				opts.LogFile,
				0, // lastPos
				opts.Format,
				opts.PtimeKey,
				opts.StatusKey,
				posFile,
				stats,
				logger,
			)
			if err != nil {
				return err
			}
			// previous file
			err = parseFile(
				lastFile,
				lastPos,
				opts.Format,
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

	stats.Display(opts.KeyPrefix, duration)

	return nil
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
