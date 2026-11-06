package config

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Size is a byte count. In YAML it may be written as a plain integer (bytes)
// or as a string with a unit suffix: B, KB/MB/GB/TB (powers of 1000),
// KiB/MiB/GiB/TiB or K/M/G/T (powers of 1024). Examples: 512MiB, 4GiB, 1500000.
type Size int64

const (
	KiB Size = 1 << 10
	MiB Size = 1 << 20
	GiB Size = 1 << 30
	TiB Size = 1 << 40
)

var sizeUnits = map[string]float64{
	"":    1,
	"b":   1,
	"k":   1 << 10,
	"kib": 1 << 10,
	"kb":  1e3,
	"m":   1 << 20,
	"mib": 1 << 20,
	"mb":  1e6,
	"g":   1 << 30,
	"gib": 1 << 30,
	"gb":  1e9,
	"t":   1 << 40,
	"tib": 1 << 40,
	"tb":  1e12,
}

// ParseSize parses a human readable size such as "4GiB" or "500MB".
func ParseSize(s string) (Size, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	num, unit := s[:i], strings.ToLower(strings.TrimSpace(s[i:]))
	if num == "" {
		return 0, fmt.Errorf("invalid size %q: missing number", s)
	}
	mult, ok := sizeUnits[unit]
	if !ok {
		return 0, fmt.Errorf("invalid size %q: unknown unit %q (use B, KiB, MiB, GiB, TiB, KB, MB, GB, TB)", s, s[i:])
	}
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	total := v * mult
	if total > math.MaxInt64 {
		return 0, fmt.Errorf("size %q is too large", s)
	}
	return Size(total), nil
}

// String renders the size using the largest binary unit that divides it exactly.
func (s Size) String() string {
	if s == 0 {
		return "0"
	}
	for _, u := range []struct {
		name string
		v    Size
	}{{"TiB", TiB}, {"GiB", GiB}, {"MiB", MiB}, {"KiB", KiB}} {
		if s%u.v == 0 {
			return fmt.Sprintf("%d%s", s/u.v, u.name)
		}
	}
	return strconv.FormatInt(int64(s), 10)
}

func (s *Size) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return typeErr(n, "expected a size (e.g. 4GiB or a number of bytes)")
	}
	if n.Tag == "!!int" {
		v, err := strconv.ParseInt(n.Value, 0, 64)
		if err != nil {
			return typeErr(n, fmt.Sprintf("invalid size %q", n.Value))
		}
		*s = Size(v)
		return nil
	}
	v, err := ParseSize(n.Value)
	if err != nil {
		return typeErr(n, err.Error())
	}
	*s = v
	return nil
}

func (s Size) MarshalYAML() (interface{}, error) { return s.String(), nil }

// MarshalJSON renders sizes as plain byte counts for machine consumers.
func (s Size) MarshalJSON() ([]byte, error) { return json.Marshal(int64(s)) }

func (s *Size) UnmarshalJSON(b []byte) error {
	var n int64
	if err := json.Unmarshal(b, &n); err == nil {
		*s = Size(n)
		return nil
	}
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return fmt.Errorf("expected size as number or string")
	}
	v, err := ParseSize(str)
	if err != nil {
		return err
	}
	*s = v
	return nil
}

// Duration is a time span. In YAML it may be a Go duration string ("90m",
// "2h30m") or an integer number of seconds.
type Duration time.Duration

func (d Duration) D() time.Duration { return time.Duration(d) }

func (d Duration) String() string {
	if d == 0 {
		return "0"
	}
	s := time.Duration(d).String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return typeErr(n, "expected a duration (e.g. 2h or 90m)")
	}
	if n.Tag == "!!int" {
		v, err := strconv.ParseInt(n.Value, 0, 64)
		if err != nil {
			return typeErr(n, fmt.Sprintf("invalid duration %q", n.Value))
		}
		*d = Duration(time.Duration(v) * time.Second)
		return nil
	}
	v, err := time.ParseDuration(strings.TrimSpace(n.Value))
	if err != nil {
		return typeErr(n, fmt.Sprintf("invalid duration %q (use e.g. 30m, 2h, 1h30m or seconds as integer)", n.Value))
	}
	*d = Duration(v)
	return nil
}

func (d Duration) MarshalYAML() (interface{}, error) { return d.String(), nil }

// MarshalJSON renders durations as seconds.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(int64(time.Duration(d) / time.Second))
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var n int64
	if err := json.Unmarshal(b, &n); err == nil {
		*d = Duration(time.Duration(n) * time.Second)
		return nil
	}
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return fmt.Errorf("expected duration as seconds or string")
	}
	v, err := time.ParseDuration(str)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// typeErr returns a yaml.TypeError so that decoding continues and all
// problems are reported at once.
func typeErr(n *yaml.Node, msg string) error {
	return &yaml.TypeError{Errors: []string{fmt.Sprintf("line %d: %s", n.Line, msg)}}
}
