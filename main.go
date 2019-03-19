package main

import (
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

func parseLog(logFile string, lastPos int64, format, ptimeKey, statusKey, posFile string, stats *axslog.Stats, logger *zap.Logger) error {
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

	fs, err := axslog.FileStat(stat)
	if err != nil {
		return errors.Wrap(err, "failed to inode of log file")
	}

	logger.Info("Start analyzing",
		zap.String("logfile", logFile),
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
		ar = ltsvreader.New(fpr, logger, ptimeKey, statusKey)
	case "json":
		ar = jsonreader.New(fpr, logger, ptimeKey, statusKey)
	}

	total := 0
	for {
		ptime, status, errb := ar.Parse()
		if errb == io.EOF {
			break
		}
		if errb != nil {
			return errors.Wrap(errb, "Something wrong in parse log")
		}
		stats.Append(ptime, status)
		total++
	}

	logger.Info("Analyzing Succeeded",
		zap.String("logfile", logFile),
		zap.Int64("startPos", lastPos),
		zap.Int64("endPos", fpr.Pos),
		zap.Int("Rows", total),
	)
	// postionのupdate
	if posFile != "" {
		err = axslog.WritePos(posFile, fpr.Pos, fs)
		if err != nil {
			return errors.Wrap(err, "failed to update pos file")
		}
	}
	return nil
}

func getStats(opts cmdOpts, logger *zap.Logger) error {
	lastPos := int64(0)
	lastFs := &axslog.FStat{}
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
		last, du, fs, err := axslog.ReadPos(posFile)
		if err != nil {
			return errors.Wrap(err, "failed to load pos file")
		}
		lastPos = last
		duration = du
		lastFs = fs
	}

	stat, err := os.Stat(opts.LogFile)
	if err != nil {
		return errors.Wrap(err, "failed to stat log file")
	}
	fs, err := axslog.FileStat(stat)
	if err != nil {
		return errors.Wrap(err, "failed to get inode from log file")
	}
	if lastFs.Inode == 0 || lastFs.Dev == 0 || (fs.Inode == lastFs.Inode && fs.Dev == lastFs.Dev) {
		err := parseLog(
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
		lastFile, err := axslog.SearchFileByInode(filepath.Dir(opts.LogFile), lastFs)
		if err != nil {
			logger.Warn("Could not search previous file",
				zap.Error(err),
			)
		} else {
			// new file
			err := parseLog(
				opts.LogFile,
				0,
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
			err = parseLog(
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
				return err
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
