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

// MaxReadSize : Maximum size for read
var MaxReadSize int64 = 500 * 1000 * 1000

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

	if lastPos == 0 && stat.Size() > MaxReadSize {
		// first time and big logile
		lastPos = stat.Size()
	}

	if stat.Size()-lastPos > MaxReadSize {
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
	default:
		return fmt.Errorf("format %s is not supported", opts.Format)
	}
	var f64s sort.Float64Slice
	var tf float64
	sc := make(map[string]float64)
	for _, k := range []string{"1xx", "2xx", "3xx", "4xx", "5xx", "total"} {
		sc[k] = 0
	}
	for {
		ptime, status, errb := ar.Parse()
		if errb == io.EOF {
			break
		}
		if errb != nil {
			return errors.Wrap(err, "Something wrong in parse log")
		}

		sc[string(fmt.Sprintf("%d", status)[0])+"xx"]++
		sc["total"]++

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
		for _, k := range []string{"1xx", "2xx", "3xx", "4xx", "5xx"} {
			fmt.Printf("axslog.access_num_%s.%s_count\t%f\t%d\n", opts.KeyPrefix, k, sc[k]/duration, now)
		}
		fmt.Printf("axslog.access_total_%s.count\t%f\t%d\n", opts.KeyPrefix, sc["total"]/duration, now)
	}
	if sc["total"] > 0 {
		for _, k := range []string{"1xx", "2xx", "3xx", "4xx", "5xx"} {
			fmt.Printf("axslog.access_ratio_%s.%s_percentage\t%f\t%d\n", opts.KeyPrefix, k, sc[k]/sc["total"], now)
		}
	}
	// postionのupdate
	err = writePos(posFile, fpr.Pos)
	if err != nil {
		return errors.Wrap(err, "failed to update pos file")
	}
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
