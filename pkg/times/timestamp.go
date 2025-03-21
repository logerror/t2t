package times

import (
	"strconv"
	"time"
)

// Timestamp is a wrapper around time.Time
type Timestamp struct {
	t time.Time
}

func NewTimestamp(t time.Time) *Timestamp {
	return &Timestamp{t: t}
}

func FromTimep(t *time.Time) *Timestamp {
	if t == nil {
		return &Timestamp{}
	}

	return &Timestamp{t: *t}
}

func (t *Timestamp) String() string {
	ts := t.t.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
	return strconv.FormatInt(ts, 10)
}

// MarshalJSON implements the json.Marshaler interface.
func (t *Timestamp) MarshalJSON() ([]byte, error) {
	ts := t.t.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
	return []byte(strconv.FormatInt(ts, 10)), nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	// Ignore null, like in the main JSON package.
	if string(data) == "null" {
		return nil
	}
	ts, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return err
	}
	t.t = time.Unix(0, ts*1000000)
	return nil
}
