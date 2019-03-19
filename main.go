package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

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

// StatusLebels :
var StatusLebels = []string{"1xx", "2xx", "3xx", "4xx", "499", "5xx", "total"}

type cmdOpts struct {
	LogFile   string `long:"logfile" description:"path to nginx ltsv logfile" required:"true"`
	Format    string `long:"format" default:"ltsv" description:"format of logfile. support json and ltsv"`
	KeyPrefix string `long:"key-prefix" description:"Metric key prefix" required:"true"`
	PtimeKey  string `long:"ptime-key" default:"ptime" description:"key name for request_time"`
	StatusKey string `long:"status-key" default:"status" description:"key name for response status"`
	Version   bool   `short:"v" long:"version" description:"Show version"`
}

type filePos struct {
	Pos   int64   `json:"pos"`
	Time  float64 `json:"time"`
	Inode uint64  `json:"inode"`
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func fileInode(s os.FileInfo) (uint64, error) {
	s2 := s.Sys().(*syscall.Stat_t)
	if s2 == nil {
		return 0, fmt.Errorf("Could not get Inode")
	}
	return s2.Ino, nil
}

func searchFileByInode(d string, ino uint64) (string, error) {
	files, err := ioutil.ReadDir(d)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		i, _ := fileInode(file)
		if i == ino {
			return file.Name(), nil
		}
	}
	return "", fmt.Errorf("Could not get file by inode")
}
func writePos(filename string, ino uint64, pos int64) error {
	fp := filePos{pos, float64(time.Now().Unix()), ino}
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	jb, err := json.Marshal(fp)
	if err != nil {
		return err
	}
	_, err = file.Write(jb)
	return err
}

func readPos(filename string) (int64, float64, uint64, error) {
	fp := filePos{}
	d, err := ioutil.ReadFile(filename)
	if err != nil {
		return 0, 0, 0, err
	}
	err = json.Unmarshal(d, &fp)
	if err != nil {
		return 0, 0, 0, err
	}
	duration := float64(time.Now().Unix()) - fp.Time
	return fp.Pos, duration, fp.Inode, nil
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

	inode, err := fileInode(stat)
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
		err = writePos(posFile, inode, fpr.Pos)
		if err != nil {
			return errors.Wrap(err, "failed to update pos file")
		}
	}
	return nil
}

func getStats(opts cmdOpts, logger *zap.Logger) error {
	lastPos := int64(0)
	lastInode := uint64(0)
	tmpDir := os.TempDir()
	curUser, _ := user.Current()
	uid := "0"
	if curUser != nil {
		uid = curUser.Uid
	}
	posFile := filepath.Join(tmpDir, fmt.Sprintf("%s-axslog-v3-%s", uid, opts.KeyPrefix))
	duration := float64(0)
	stats := axslog.NewStats()

	if fileExists(posFile) {
		last, du, ino, err := readPos(posFile)
		if err != nil {
			return errors.Wrap(err, "failed to load pos file")
		}
		lastPos = last
		duration = du
		lastInode = ino
	}

	stat, err := os.Stat(opts.LogFile)
	if err != nil {
		return errors.Wrap(err, "failed to stat log file")
	}
	inode, err := fileInode(stat)
	if err != nil {
		return errors.Wrap(err, "failed to get inode from log file")
	}
	if lastPos != 0 && inode != lastInode {
		// rotate!!
		lastFile, err := searchFileByInode(filepath.Dir(opts.LogFile), lastInode)
		if err != nil {
			logger.Warn("Detect rotate but could not previous file",
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
	} else {
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
