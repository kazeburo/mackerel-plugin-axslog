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

// Reader :
type Reader interface {
	Parse([]byte) (int, []byte, []byte)
}

// Stats :
type Stats struct {
	f64s  sort.Float64Slice
	tf    float64
	c1xx  float64
	c2xx  float64
	c3xx  float64
	c4xx  float64
	c499  float64
	c5xx  float64
	total float64
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
func WritePos(filename string, pos int64, fstat *FStat) error {
	fp := FilePos{pos, float64(time.Now().Unix()), fstat.Inode, fstat.Dev}
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
	duration := float64(time.Now().Unix()) - fp.Time
	return fp.Pos, duration, &FStat{fp.Inode, fp.Dev}, nil
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

// Display :
func (s *Stats) Display(keyPrefix string, duration float64) {
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

	if duration > 0 {
		fmt.Printf("axslog.access_num_%s.1xx_count\t%f\t%d\n", keyPrefix, s.c1xx/duration, now)
		fmt.Printf("axslog.access_num_%s.2xx_count\t%f\t%d\n", keyPrefix, s.c2xx/duration, now)
		fmt.Printf("axslog.access_num_%s.3xx_count\t%f\t%d\n", keyPrefix, s.c3xx/duration, now)
		fmt.Printf("axslog.access_num_%s.4xx_count\t%f\t%d\n", keyPrefix, s.c4xx/duration, now)
		fmt.Printf("axslog.access_num_%s.499_count\t%f\t%d\n", keyPrefix, s.c499/duration, now)
		fmt.Printf("axslog.access_num_%s.5xx_count\t%f\t%d\n", keyPrefix, s.c5xx/duration, now)
		fmt.Printf("axslog.access_total_%s.count\t%f\t%d\n", keyPrefix, s.total/duration, now)
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
