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

var version string

// MaxReadSizeJSON : Maximum size for read
var MaxReadSizeJSON int64 = 500 * 1000 * 1000

// MaxReadSizeLTSV : Maximum size for read
var MaxReadSizeLTSV int64 = 1000 * 1000 * 1000

const (
	maxScanTokenSize = 1 * 1024 * 1024 // 1MiB
	startBufSize     = 4096
)

var f0 = float64(0)

// parseLog :
var bracketByte = []byte("{")

func parseLog(bs *bufio.Scanner, r axslog.Reader, opts axslog.CmdOpts) (float64, int, error) {
	filter := []byte(opts.Filter)
	for bs.Scan() {
		b := bs.Bytes()
		if len(filter) > 0 {
			if opts.InvertFilter {
				if bytes.Contains(b, filter) {
					continue
				}
			} else {
				if !bytes.Contains(b, filter) {
					continue
				}
			}
		}
		if opts.SkipUntilBracket {
			i := bytes.Index(b, bracketByte)
			if i > 0 {
				b = b[i:]
			}
		}
		c, pt, st := r.Parse(b)
		if c&axslog.PtimeFlag == 0 {
			log.Printf("No ptime. continue key:%s", opts.PtimeKey)
			continue
		}
		if c&axslog.StatusFlag == 0 {
			log.Printf("No status. continue key:%v", opts.StatusKeys)
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
func parseFile(logFile string, lastPos int64, opts axslog.CmdOpts, posFile string, stats *axslog.Stats) (float64, error) {
	maxReadSize := int64(0)
	switch opts.Format {
	case "ltsv":
		maxReadSize = MaxReadSizeLTSV
	case "json":
		maxReadSize = MaxReadSizeJSON
	default:
		return f0, fmt.Errorf("format %s is not supported", opts.Format)
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
	switch opts.Format {
	case "ltsv":
		ar = ltsvreader.New(opts.PtimeKey, opts.StatusKeys)
	case "json":
		ar = jsonreader.New(opts.PtimeKey, opts.StatusKeys)
	}

	total := 0
	bs := bufio.NewScanner(fpr)
	bs.Buffer(make([]byte, startBufSize), maxScanTokenSize)
	for {
		ptime, status, errb := parseLog(bs, ar, opts)
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

func getFileStats(opts axslog.CmdOpts, posFile, logFile string) (*axslog.Stats, error) {
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
			opts,
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
				opts,
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
				opts,
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
				opts,
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

func getStats(opts axslog.CmdOpts) error {
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
			// BEGIN-NOSCAN
			md5 := md5.Sum([]byte(logfile))
			// END-NOSCAN
			posFile := filepath.Join(tmpDir, fmt.Sprintf("%s-axslog-v4-%s-%x", uid, opts.KeyPrefix, md5))
			stats, err := getFileStats(opts, posFile, logfile)
			sCh <- axslog.StatsCh{
				Stats:   stats,
				Logfile: logfile,
				Err:     err,
			}
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
		version,
		runtime.Compiler,
		runtime.Version())
}

func main() {
	os.Exit(_main())
}

func _main() int {
	opts := axslog.CmdOpts{}
	psr := flags.NewParser(&opts, flags.HelpFlag|flags.PassDoubleDash)
	_, err := psr.Parse()
	if opts.Version {
		printVersion()
		return 0
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	err = getStats(opts)
	if err != nil {
		log.Printf("getStats: %v", err)
		return 1
	}
	return 0
}
