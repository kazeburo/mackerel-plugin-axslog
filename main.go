package main

import (
	"crypto/md5"
	"fmt"
	"log"
	"os"
	"os/user"
	"runtime"
	"strings"

	flags "github.com/jessevdk/go-flags"
	"github.com/kazeburo/followparser"
	"github.com/kazeburo/mackerel-plugin-axslog/axslog"
	"github.com/mackerelio/golib/pluginutil"
)

var version string

var (
	// MaxReadSizeJSON : Maximum size for read
	maxReadSizeJSON int64 = 500 * 1000 * 1000

	// MaxReadSizeLTSV : Maximum size for read
	maxReadSizeLTSV int64 = 1000 * 1000 * 1000

	maxScanTokenSize = 1 * 1024 * 1024 // 1MiB
	startBufSize     = 4096
)

func getFileStats(opts *axslog.CmdOpts, posFile, logFile string) (*axslog.Stats, error) {
	stats := axslog.NewStats()
	maxReadSize := int64(0)
	switch opts.Format {
	case "ltsv":
		maxReadSize = maxReadSizeLTSV
	case "json":
		maxReadSize = maxReadSizeJSON
	default:
		return stats, fmt.Errorf("format %s is not supported", opts.Format)
	}

	parser := NewParser(opts, stats)
	fp := &followparser.Parser{
		WorkDir:      pluginutil.PluginWorkDir(),
		Callback:     parser,
		Silent:       false,
		MaxReadSize:  maxReadSize,
		StartBufSize: startBufSize,
		MaxBufSize:   maxScanTokenSize,
	}
	fp.Parse(posFile, logFile)

	return stats, nil
}

func getStats(opts *axslog.CmdOpts) error {
	curUser, _ := user.Current()
	uid := "0"
	if curUser != nil {
		uid = curUser.Uid
	}

	logfiles := strings.Split(opts.LogFile, ",")

	if len(logfiles) == 1 {
		posFile := fmt.Sprintf("%s-axslog-v5-%s", uid, opts.KeyPrefix)
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
			posFile := fmt.Sprintf("%s-axslog-v5-%s-%x", uid, opts.KeyPrefix, md5)
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
	opts := &axslog.CmdOpts{}
	psr := flags.NewParser(opts, flags.HelpFlag|flags.PassDoubleDash)
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
