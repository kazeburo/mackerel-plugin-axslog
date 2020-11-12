package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"log"
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
	"github.com/mackerelio/golib/pluginutil"

	"github.com/pkg/errors"
)

// Version by Makefile
var Version string

// MaxReadSizeJSON : Maximum size for read
var MaxReadSizeJSON int64 = 500 * 1000 * 1000

// MaxReadSizeLTSV : Maximum size for read
var MaxReadSizeLTSV int64 = 1000 * 1000 * 1000

const (
	maxScanTokenSize = 1 * 1024 * 1024 // 1MiB
	startBufSize     = 4096
)

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
func parseLog(bs *bufio.Scanner, r axslog.Reader, filter []byte, ptimeKey, statusKey string) (float64, int, error) {
	for bs.Scan() {
		b := bs.Bytes()
		if len(filter) > 0 {
			if bytes.Index(b, filter) < 0 {
				continue
			}
		}
		c, pt, st := r.Parse(b)
		if c&axslog.PtimeFlag == 0 {
			log.Printf("No ptime. continue key:%s", ptimeKey)
			continue
		}
		if c&axslog.StatusFlag == 0 {
			log.Printf("No status. continue key:%s", statusKey)
			continue
		}
		ptime, err := axslog.BFloat64(pt)
		if err != nil {
			log.Printf("Failed to convert ptime. continue: %v", err)
			continue
		}
		status, err := axslog.BInt(st)
		if err != nil {
			log.Printf("Failed to convert status. continue: %v", err)
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
func parseFile(logFile string, lastPos int64, format, filter, ptimeKey, statusKey, posFile string, stats *axslog.Stats) (float64, error) {
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

	log.Printf("Analysis start logFile:%s lastPos:%d Size:%d", logFile, lastPos, stat.Size())

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
		ar = ltsvreader.New(ptimeKey, statusKey)
	case "json":
		ar = jsonreader.New(ptimeKey, statusKey)
	}

	total := 0
	bs := bufio.NewScanner(fpr)
	bs.Buffer(make([]byte, startBufSize), maxScanTokenSize)
	fb := []byte(filter)
	for {
		ptime, status, errb := parseLog(bs, ar, fb, ptimeKey, statusKey)
		if errb == io.EOF {
			break
		}
		if errb != nil {
			return f0, errors.Wrap(errb, "Something wrong in parse log")
		}
		stats.Append(ptime, status)
		total++
	}

	log.Printf("Analysis completed logFile:%s startPos:%d endPos:%d Rows:%d", logFile, lastPos, fpr.Pos, total)

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

func getFileStats(opts cmdOpts, posFile, logFile string) (*axslog.Stats, error) {
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
		)
		if err != nil {
			return stats, err
		}
	} else {
		// rotate!!
		log.Printf("Detect Rotate")
		lastFile, err := axslog.SearchFileByInode(filepath.Dir(logFile), lastFstat)
		if err != nil {
			log.Printf("Could not search previous file: %v", err)
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
			)
			if err != nil {
				log.Printf("Could not parse previous file: %v", err)
			}
		}
	}
	stats.SetDuration(endTime - startTime)
	return stats, nil
}

func getStats(opts cmdOpts) error {
	tmpDir := pluginutil.PluginWorkDir()
	curUser, _ := user.Current()
	uid := "0"
	if curUser != nil {
		uid = curUser.Uid
	}

	logfiles := strings.Split(opts.LogFile, ",")

	if len(logfiles) == 1 {
		posFile := filepath.Join(tmpDir, fmt.Sprintf("%s-axslog-v4-%s", uid, opts.KeyPrefix))
		stats, err := getFileStats(opts, posFile, opts.LogFile)
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
			stats, err := getFileStats(opts, posFile, logfile)
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
			log.Printf("getStats file:%s :%v", s.Logfile, s.Err)
		} else {
			statsAll = append(statsAll, s.Stats)
		}
	}

	axslog.DisplayAll(statsAll, opts.KeyPrefix)
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
	err = getStats(opts)
	if err != nil {
		log.Printf("getStats: %v", err)
		return 1
	}
	return 0
}
