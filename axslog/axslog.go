package axslog

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

// PtimeFlag : ptime is exists
var PtimeFlag = 1

// StatusFlag : stattus is exists
var StatusFlag = 2

// AllFlagOK : all OK
var AllFlagOK = 3

type CmdOpts struct {
	LogFile          string `long:"logfile" description:"path to nginx ltsv logfiles. multiple log files can be specified, separated by commas." required:"true"`
	Format           string `long:"format" default:"ltsv" description:"format of logfile. support json and ltsv"`
	KeyPrefix        string `long:"key-prefix" description:"Metric key prefix" required:"true"`
	PtimeKey         string `long:"ptime-key" default:"ptime" description:"key name for request_time"`
	StatusKey        string `long:"status-key" default:"status" description:"key name for response status"`
	Filter           string `long:"filter" default:"" description:"text for filtering log"`
	SkipUntilBracket bool   `long:"skip-until-json" description:"skip reading until first { for json log with plain text header"`
	Version          bool   `short:"v" long:"version" description:"Show version"`
	filterByte       *[]byte
}

// Reader :
type Reader interface {
	Parse([]byte) (int, []byte, []byte)
}

// Stats :
type Stats struct {
	f64s     sort.Float64Slice
	tf       float64
	c1xx     float64
	c2xx     float64
	c3xx     float64
	c4xx     float64
	c499     float64
	c5xx     float64
	total    float64
	duration float64
}

// StatsCh :
type StatsCh struct {
	Stats   *Stats
	Logfile string
	Err     error
}

// FilePos :
type FilePos struct {
	Pos   int64   `json:"pos"`
	Time  float64 `json:"time"`
	Inode uint64  `json:"inode"`
	Dev   uint64  `json:"dev"`
}

// FStat :
type FStat struct {
	Inode uint64
	Dev   uint64
}

// FileExists :
func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// FileStat :
func FileStat(s os.FileInfo) (*FStat, error) {
	s2 := s.Sys().(*syscall.Stat_t)
	if s2 == nil {
		return &FStat{}, fmt.Errorf("Could not get Inode")
	}
	return &FStat{s2.Ino, uint64(s2.Dev)}, nil
}

// IsNotRotated :
func (fstat *FStat) IsNotRotated(lastFstat *FStat) bool {
	return lastFstat.Inode == 0 || lastFstat.Dev == 0 || (fstat.Inode == lastFstat.Inode && fstat.Dev == lastFstat.Dev)
}

// SearchFileByInode :
func SearchFileByInode(d string, fstat *FStat) (string, error) {
	files, err := ioutil.ReadDir(d)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		s, _ := FileStat(file)
		if s.Inode == fstat.Inode && s.Dev == fstat.Dev {
			return filepath.Join(d, file.Name()), nil
		}
	}
	return "", fmt.Errorf("There is no file by inode:%d in %s", fstat.Inode, d)
}

// WritePos :
func WritePos(filename string, pos int64, fstat *FStat) (float64, error) {
	now := float64(time.Now().Unix())
	fp := FilePos{pos, now, fstat.Inode, fstat.Dev}
	file, err := os.Create(filename)
	if err != nil {
		return now, err
	}
	defer file.Close()
	jb, err := json.Marshal(fp)
	if err != nil {
		return now, err
	}
	_, err = file.Write(jb)
	return now, err
}

// ReadPos :
func ReadPos(filename string) (int64, float64, *FStat, error) {
	fp := FilePos{}
	d, err := ioutil.ReadFile(filename)
	if err != nil {
		return 0, 0, &FStat{}, err
	}
	err = json.Unmarshal(d, &fp)
	if err != nil {
		return 0, 0, &FStat{}, err
	}
	return fp.Pos, fp.Time, &FStat{fp.Inode, fp.Dev}, nil
}

func round(f float64) int64 {
	return int64(math.Round(f)) - 1
}

func statusCode(status int) int {
	switch status {
	case 499:
		return 499
	default:
		return status / 100
	}
}

// NewStats :
func NewStats() *Stats {
	return &Stats{}
}

// GetTotal :
func (s *Stats) GetTotal() float64 {
	return s.total
}

// Append :
func (s *Stats) Append(ptime float64, status int) {
	switch statusCode(status) {
	case 2:
		s.c2xx++
	case 3:
		s.c3xx++
	case 4:
		s.c4xx++
	case 5:
		s.c5xx++
	case 499:
		s.c499++
	case 1:
		s.c1xx++
	}
	s.total++

	s.f64s = append(s.f64s, ptime)
	s.tf += ptime

}

// SetDuration :
func (s *Stats) SetDuration(d float64) {
	s.duration = d
}

// Display :
func (s *Stats) Display(keyPrefix string) {
	now := uint64(time.Now().Unix())
	sort.Sort(s.f64s)
	fl := float64(len(s.f64s))
	// fmt.Printf("count: %d\n", len(f64s))
	if len(s.f64s) > 0 {
		fmt.Printf("axslog.latency_%s.average\t%f\t%d\n", keyPrefix, s.tf/fl, now)
		fmt.Printf("axslog.latency_%s.99_percentile\t%f\t%d\n", keyPrefix, s.f64s[round(fl*0.99)], now)
		fmt.Printf("axslog.latency_%s.95_percentile\t%f\t%d\n", keyPrefix, s.f64s[round(fl*0.95)], now)
		fmt.Printf("axslog.latency_%s.90_percentile\t%f\t%d\n", keyPrefix, s.f64s[round(fl*0.90)], now)
	}

	if s.duration > 0 {
		fmt.Printf("axslog.access_num_%s.1xx_count\t%f\t%d\n", keyPrefix, s.c1xx/s.duration, now)
		fmt.Printf("axslog.access_num_%s.2xx_count\t%f\t%d\n", keyPrefix, s.c2xx/s.duration, now)
		fmt.Printf("axslog.access_num_%s.3xx_count\t%f\t%d\n", keyPrefix, s.c3xx/s.duration, now)
		fmt.Printf("axslog.access_num_%s.4xx_count\t%f\t%d\n", keyPrefix, s.c4xx/s.duration, now)
		fmt.Printf("axslog.access_num_%s.499_count\t%f\t%d\n", keyPrefix, s.c499/s.duration, now)
		fmt.Printf("axslog.access_num_%s.5xx_count\t%f\t%d\n", keyPrefix, s.c5xx/s.duration, now)
		fmt.Printf("axslog.access_total_%s.count\t%f\t%d\n", keyPrefix, s.total/s.duration, now)
	}
	if s.total > 0 {
		fmt.Printf("axslog.access_ratio_%s.1xx_percentage\t%f\t%d\n", keyPrefix, s.c1xx*100/s.total, now)
		fmt.Printf("axslog.access_ratio_%s.2xx_percentage\t%f\t%d\n", keyPrefix, s.c2xx*100/s.total, now)
		fmt.Printf("axslog.access_ratio_%s.3xx_percentage\t%f\t%d\n", keyPrefix, s.c3xx*100/s.total, now)
		fmt.Printf("axslog.access_ratio_%s.4xx_percentage\t%f\t%d\n", keyPrefix, s.c4xx*100/s.total, now)
		fmt.Printf("axslog.access_ratio_%s.499_percentage\t%f\t%d\n", keyPrefix, s.c499*100/s.total, now)
		fmt.Printf("axslog.access_ratio_%s.5xx_percentage\t%f\t%d\n", keyPrefix, s.c5xx*100/s.total, now)
	}
}

// SFloat64 :
func SFloat64(val string) (float64, error) {
	return strconv.ParseFloat(val, 64)
}

// SInt :
func SInt(val string) (int, error) {
	return strconv.Atoi(val)
}

// BFloat64 :
func BFloat64(b []byte) (float64, error) {
	return strconv.ParseFloat(*(*string)(unsafe.Pointer(&b)), 64)
}

// BInt :
func BInt(b []byte) (int, error) {
	return strconv.Atoi(*(*string)(unsafe.Pointer(&b)))
}

// DisplayAll :
func DisplayAll(statsAll []*Stats, keyPrefix string) {
	now := uint64(time.Now().Unix())

	var f64s sort.Float64Slice
	tf := float64(0)
	c1xx := float64(0)
	c2xx := float64(0)
	c3xx := float64(0)
	c4xx := float64(0)
	c499 := float64(0)
	c5xx := float64(0)
	total := float64(0)
	allDutrainNG := true
	for _, s := range statsAll {
		for _, pt := range s.f64s {
			f64s = append(f64s, pt)
			tf += pt
		}
		if s.duration > 0 {
			allDutrainNG = false
			c1xx += s.c1xx / s.duration
			c2xx += s.c2xx / s.duration
			c3xx += s.c3xx / s.duration
			c4xx += s.c4xx / s.duration
			c499 += s.c499 / s.duration
			c5xx += s.c5xx / s.duration
			total += s.total / s.duration
		}
	}
	sort.Sort(f64s)
	fl := float64(len(f64s))
	// fmt.Printf("count: %d\n", len(f64s))
	if len(f64s) > 0 {
		fmt.Printf("axslog.latency_%s.average\t%f\t%d\n", keyPrefix, tf/fl, now)
		fmt.Printf("axslog.latency_%s.99_percentile\t%f\t%d\n", keyPrefix, f64s[round(fl*0.99)], now)
		fmt.Printf("axslog.latency_%s.95_percentile\t%f\t%d\n", keyPrefix, f64s[round(fl*0.95)], now)
		fmt.Printf("axslog.latency_%s.90_percentile\t%f\t%d\n", keyPrefix, f64s[round(fl*0.90)], now)
	}

	if allDutrainNG == false {
		fmt.Printf("axslog.access_num_%s.1xx_count\t%f\t%d\n", keyPrefix, c1xx, now)
		fmt.Printf("axslog.access_num_%s.2xx_count\t%f\t%d\n", keyPrefix, c2xx, now)
		fmt.Printf("axslog.access_num_%s.3xx_count\t%f\t%d\n", keyPrefix, c3xx, now)
		fmt.Printf("axslog.access_num_%s.4xx_count\t%f\t%d\n", keyPrefix, c4xx, now)
		fmt.Printf("axslog.access_num_%s.499_count\t%f\t%d\n", keyPrefix, c499, now)
		fmt.Printf("axslog.access_num_%s.5xx_count\t%f\t%d\n", keyPrefix, c5xx, now)
		fmt.Printf("axslog.access_total_%s.count\t%f\t%d\n", keyPrefix, total, now)
	}

	if total > 0 {
		fmt.Printf("axslog.access_ratio_%s.1xx_percentage\t%f\t%d\n", keyPrefix, c1xx*100/total, now)
		fmt.Printf("axslog.access_ratio_%s.2xx_percentage\t%f\t%d\n", keyPrefix, c2xx*100/total, now)
		fmt.Printf("axslog.access_ratio_%s.3xx_percentage\t%f\t%d\n", keyPrefix, c3xx*100/total, now)
		fmt.Printf("axslog.access_ratio_%s.4xx_percentage\t%f\t%d\n", keyPrefix, c4xx*100/total, now)
		fmt.Printf("axslog.access_ratio_%s.499_percentage\t%f\t%d\n", keyPrefix, c499*100/total, now)
		fmt.Printf("axslog.access_ratio_%s.5xx_percentage\t%f\t%d\n", keyPrefix, c5xx*100/total, now)
	}

}
