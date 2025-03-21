package times

import (
	"time"
)

const (
	DateLayout          = "2006-01-02"
	DateTimeLayout      = "2006-01-02 15:04:05"
	DateTimestampLayout = "2006-01-02 15:04:05.000"
)

const (
	FormatYear        = "2006"
	FormatMonth       = "01"
	FormatDay         = "02"
	FormatHour        = "15"
	FormatMinute      = "04"
	FormatSecond      = "05"
	FormatMillisecond = "0"

	// YYYY_MM_DD : YYYY-MM-DD
	YYYY_MM_DD = "2006-01-02"
	// YYYY_MM_DD__HH_mm_ss : YYYY-MM-DD HH:mm:ss
	YYYY_MM_DD__HH_mm_ss = "2006-01-02 15:04:05"
	// YYYY_MM_DD__HH_mm_ss_SSS : YYYY-MM-DD HH:mm:ss.SSS
	YYYY_MM_DD__HH_mm_ss_SSS = "2006-01-02 15:04:05.000"
	// YYYY_MM_DD__HH_mm_ss_SSSSS : YYYY-MM-DD HH:mm:ss.SSSSS
	YYYY_MM_DD__HH_mm_ss_SSSSS = "2006-01-02 15:04:05.00000"
)

type (
	// Time is an alias for time.Time
	Time = time.Time
)

func Location() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err == nil && location != nil {
		return location
	}
	return time.FixedZone("CST", 8*3600)
}

// Now is current time in +8000 CST
func Now() time.Time {
	return time.Now().In(Location())
}

func Nowp() *time.Time {
	t := Now()
	return &t
}

func Unix(timeStamp int64) *time.Time {
	t := time.Unix(timeStamp, 0)
	return &t
}

func UnixMilli(msec int64) *time.Time {
	t := time.Unix(msec/1e3, (msec%1e3)*1e6)
	return &t
}

// NowStr is a formatted string of current time
func NowStr() string {
	return Now().Format(DateTimeLayout)
}

// CurrentMillisecond returns the current time in milliseconds.
func CurrentMillisecond() int64 {
	return Millisecond(time.Now())
}

// Millisecond returns the milliseconds of the time.
func Millisecond(t time.Time) int64 {
	return t.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
}

// CurrentTimestamp returns currently timestamp
func CurrentTimestamp() int64 {
	return Now().Unix()
}

func Format(t time.Time, layout string) string {
	return t.Format(layout)
}

func FormatToDateTime(t time.Time) string {
	return Format(t, DateTimeLayout)
}

func FormatToDate(t time.Time) string {
	return Format(t, DateLayout)
}

func ParseToDateTime(str string) *time.Time {
	t, _ := time.ParseInLocation(DateLayout, str, Location())
	return &t
}

func NowPGFormat() string {
	return time.Now().Format("2006-01-02 15:04:05 -0700")
}
