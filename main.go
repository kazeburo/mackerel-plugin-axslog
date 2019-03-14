package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
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
	Pos  int64   `json:"pos"`
	Time float64 `json:"time"`
}

func round(f float64) int64 {
	return int64(math.Round(f)) - 1
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func writePos(filename string, pos int64) error {
	fp := filePos{pos, float64(time.Now().Unix())}
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

func readPos(filename string) (int64, float64, error) {
	fp := filePos{}
	d, err := ioutil.ReadFile(filename)
	if err != nil {
		return 0, 0, err
	}
	err = json.Unmarshal(d, &fp)
	if err != nil {
		return 0, 0, err
	}
	duration := float64(time.Now().Unix()) - fp.Time
	return fp.Pos, duration, nil
}

func statusCode(status int) int {
	switch status {
	case 499:
		return 499
	default:
		return status / 100
	}
}

func getStats(opts cmdOpts, logger *zap.Logger) error {
	lastPos := int64(0)
	tmpDir := os.TempDir()
	curUser, _ := user.Current()
	posFile := filepath.Join(tmpDir, fmt.Sprintf("%s-nginx-ltsv-%s", curUser.Uid, opts.KeyPrefix))
	duration := float64(0)

	if fileExists(posFile) {
		last, du, err := readPos(posFile)
		if err != nil {
			return errors.Wrap(err, "failed to load pos file")
		}
		lastPos = last
		duration = du
	}

	stat, err := os.Stat(opts.LogFile)
	if err != nil {
		return errors.Wrap(err, "failed to stat log file")
	}

	if stat.Size() < lastPos {
		// rotate
		lastPos = 0
	}
	maxReadSize := int64(0)
	switch opts.Format {
	case "ltsv":
		maxReadSize = MaxReadSizeLTSV
	case "json":
		maxReadSize = MaxReadSizeJSON
	default:
		return fmt.Errorf("format %s is not supported", opts.Format)
	}

	if lastPos == 0 && stat.Size() > maxReadSize {
		// first time and big logile
		lastPos = stat.Size()
	}

	if stat.Size()-lastPos > maxReadSize {
		// big delay
		lastPos = stat.Size()
	}

	logger.Info("Start analyzing",
		zap.String("logfile", opts.LogFile),
		zap.Int64("lastPos", lastPos),
		zap.Int64("Size", stat.Size()),
	)

	f, err := os.Open(opts.LogFile)
	if err != nil {
		return errors.Wrap(err, "failed to open log file")
	}
	defer f.Close()
	fpr, err := posreader.New(f, lastPos)
	if err != nil {
		return errors.Wrap(err, "failed to seek log file")
	}

	var ar axslog.Reader
	switch opts.Format {
	case "ltsv":
		ar = ltsvreader.New(fpr, logger, opts.PtimeKey, opts.StatusKey)
	case "json":
		ar = jsonreader.New(fpr, logger, opts.PtimeKey, opts.StatusKey)
	}
	var f64s sort.Float64Slice
	var tf float64
	c1xx := float64(0)
	c2xx := float64(0)
	c3xx := float64(0)
	c4xx := float64(0)
	c499 := float64(0)
	c5xx := float64(0)
	total := float64(0)

	for {
		ptime, status, errb := ar.Parse()
		if errb == io.EOF {
			break
		}
		if errb != nil {
			return errors.Wrap(err, "Something wrong in parse log")
		}

		switch statusCode(status) {
		case 2:
			c2xx++
		case 3:
			c3xx++
		case 4:
			c4xx++
		case 5:
			c5xx++
		case 499:
			c499++
		case 1:
			c1xx++
		}
		total++

		f64s = append(f64s, ptime)
		tf += ptime
	}

	now := uint64(time.Now().Unix())
	sort.Sort(f64s)
	fl := float64(len(f64s))
	// fmt.Printf("count: %d\n", len(f64s))
	if len(f64s) > 0 {
		fmt.Printf("axslog.latency_%s.average\t%f\t%d\n", opts.KeyPrefix, tf/fl, now)
		fmt.Printf("axslog.latency_%s.99_percentile\t%f\t%d\n", opts.KeyPrefix, f64s[round(fl*0.99)], now)
		fmt.Printf("axslog.latency_%s.95_percentile\t%f\t%d\n", opts.KeyPrefix, f64s[round(fl*0.95)], now)
		fmt.Printf("axslog.latency_%s.90_percentile\t%f\t%d\n", opts.KeyPrefix, f64s[round(fl*0.90)], now)
	}

	if duration > 0 {
		fmt.Printf("axslog.access_num_%s.1xx_count\t%f\t%d\n", opts.KeyPrefix, c1xx/duration, now)
		fmt.Printf("axslog.access_num_%s.2xx_count\t%f\t%d\n", opts.KeyPrefix, c2xx/duration, now)
		fmt.Printf("axslog.access_num_%s.3xx_count\t%f\t%d\n", opts.KeyPrefix, c3xx/duration, now)
		fmt.Printf("axslog.access_num_%s.4xx_count\t%f\t%d\n", opts.KeyPrefix, c4xx/duration, now)
		fmt.Printf("axslog.access_num_%s.499_count\t%f\t%d\n", opts.KeyPrefix, c499/duration, now)
		fmt.Printf("axslog.access_num_%s.5xx_count\t%f\t%d\n", opts.KeyPrefix, c5xx/duration, now)
		fmt.Printf("axslog.access_total_%s.count\t%f\t%d\n", opts.KeyPrefix, total/duration, now)
	}
	if total > 0 {
		fmt.Printf("axslog.access_ratio_%s.1xx_percentage\t%f\t%d\n", opts.KeyPrefix, c1xx/total, now)
		fmt.Printf("axslog.access_ratio_%s.2xx_percentage\t%f\t%d\n", opts.KeyPrefix, c2xx/total, now)
		fmt.Printf("axslog.access_ratio_%s.3xx_percentage\t%f\t%d\n", opts.KeyPrefix, c3xx/total, now)
		fmt.Printf("axslog.access_ratio_%s.4xx_percentage\t%f\t%d\n", opts.KeyPrefix, c4xx/total, now)
		fmt.Printf("axslog.access_ratio_%s.499_percentage\t%f\t%d\n", opts.KeyPrefix, c499/total, now)
		fmt.Printf("axslog.access_ratio_%s.5xx_percentage\t%f\t%d\n", opts.KeyPrefix, c5xx/total, now)
	}
	// postionのupdate
	err = writePos(posFile, fpr.Pos)
	if err != nil {
		return errors.Wrap(err, "failed to update pos file")
	}
	logger.Info("Analyzing Succeeded",
		zap.String("logfile", opts.LogFile),
		zap.Int64("startPos", lastPos),
		zap.Int64("endPos", fpr.Pos),
		zap.Float64("Rows", total),
	)
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
