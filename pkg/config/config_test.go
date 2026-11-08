package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseValidConfig(t *testing.T) {
	doc, is := Parse([]byte(`
version: 1
job:
  id: task-42
collect:
  profile: standard
  exclude_artifacts: [browser.*]
  hashes: [md5, sha256]
  custom_artifacts:
    - name: custom.app
      sources:
        - os: [windows]
          paths: ['%ProgramData%\App\logs\*.log']
        - os: [macos, linux]
          paths: ['/var/log/app/*.log']
limits:
  max_file_size: 512MiB
  max_total_size: 10737418240
  max_duration: 90m
output:
  path: 'C:\Windows\Temp\relic-{job_id}.zip'
`), "test")
	if is.HasErrors() {
		t.Fatal(is)
	}
	c := doc.Config
	if c.Limits.MaxFileSize != 512*MiB || c.Limits.MaxTotalSize != 10*GiB || c.Limits.MaxDuration.D() != 90*time.Minute {
		t.Errorf("limits: %+v", c.Limits)
	}
	// Defaults are kept for unspecified values.
	if c.Limits.MinFreeSpace != GiB || c.Collect.Method != MethodAuto || c.Output.Compression != CompressionAuto {
		t.Errorf("defaults lost: %+v %+v", c.Limits, c.Collect)
	}
	if c.Collect.CustomArtifacts[0].Sources[0].Type != SourceTypeFile {
		t.Errorf("source type default not applied")
	}
	if doc.SHA256 == "" || doc.Source != "test" {
		t.Errorf("document metadata missing")
	}
}

func TestParseReportsAllProblemsWithPositions(t *testing.T) {
	_, is := Parse([]byte(`version: 1
collect:
  profil: standard
  method: fast
  hashes: [sha256, crc32]
  custom_artifacts:
    - name: Bad Name
      sources:
        - type: registry
          paths: ['var/log/*.log']
limits:
  max_file_size: 12 apples
  max_duration: soon
  max_files: many
output:
  compression_level: 12
  path: 'out-{date}.zip'
  colour: blue
`), "test")
	want := []struct {
		line int
		path string
		msg  string
	}{
		{3, "collect.profil", `did you mean "profile"`},
		{4, "collect.method", `invalid value "fast"`},
		{5, "collect.hashes[1]", `unsupported hash algorithm "crc32"`},
		{7, "collect.custom_artifacts[0].name", "invalid name"},
		{9, "collect.custom_artifacts[0].sources[0].type", `unsupported source type "registry"`},
		{10, "collect.custom_artifacts[0].sources[0].paths[0]", "must be absolute"},
		{12, "limits.max_file_size", "unknown unit"},
		{13, "limits.max_duration", "invalid duration"},
		{14, "limits.max_files", "expected an integer"},
		{16, "output.compression_level", "between 1"},
		{17, "output.path", "unknown placeholder {date}"},
		{18, "output.colour", "unknown field"},
	}
	for _, w := range want {
		found := false
		for _, i := range is {
			if i.Line == w.line && i.Path == w.path && strings.Contains(i.Message, w.msg) {
				found = true
			}
		}
		if !found {
			t.Errorf("missing issue line %d %s %q in:\n%s", w.line, w.path, w.msg, is.Error())
		}
	}
}

func TestParseStructuralErrors(t *testing.T) {
	cases := map[string]string{
		"":                              "configuration is empty",
		"- a\n- b\n":                    "must be a YAML mapping",
		"collect: {profile: minimal}\n": "version: is required",
		"version: 2\ncollect: {profile: minimal}\n": "unsupported configuration version 2",
		"version: 1\n":               "nothing selected",
		"version: 1\ncollect: [a]\n": "expected a mapping",
		"version: 1\ncollect:\n  profile: x\n   bad: indent\n": "invalid YAML",
	}
	for in, want := range cases {
		_, is := Parse([]byte(in), "test")
		if !strings.Contains(is.Error(), want) {
			t.Errorf("%q: want %q, got:\n%s", in, want, is.Error())
		}
	}
}

func TestNullSectionsKeepDefaults(t *testing.T) {
	doc, is := Parse([]byte("version: 1\ncollect:\n  profile: minimal\nlimits:\noutput:\n"), "test")
	if is.HasErrors() {
		t.Fatal(is)
	}
	if doc.Config.Limits.MaxFileSize != Default().Limits.MaxFileSize {
		t.Errorf("empty section reset defaults")
	}
}

func TestOSSpecificPathValidation(t *testing.T) {
	_, is := Parse([]byte(`version: 1
collect:
  custom_artifacts:
    - name: custom.x
      sources:
        - paths: ['C:\x\*.log']
        - os: [linux]
          method: raw
          paths: ['/x']
`), "test")
	msg := is.Error()
	for _, w := range []string{"only valid on Windows; restrict the source with os: [windows]", "method raw (NTFS) is only supported"} {
		if !strings.Contains(msg, w) {
			t.Errorf("missing %q in:\n%s", w, msg)
		}
	}
}

func TestSizeAndDuration(t *testing.T) {
	for in, want := range map[string]Size{"1": 1, "1KB": 1000, "1KiB": 1024, "1k": 1024, "1.5GiB": 3 * GiB / 2, "2 MB": 2e6} {
		got, err := ParseSize(in)
		if err != nil || got != want {
			t.Errorf("%s: %d %v", in, got, err)
		}
	}
	if (4*GiB).String() != "4GiB" || Size(1500).String() != "1500" {
		t.Errorf("size String")
	}
	for d, want := range map[time.Duration]string{4 * time.Hour: "4h", 90 * time.Minute: "1h30m", 30 * time.Second: "30s", time.Minute: "1m"} {
		if got := Duration(d).String(); got != want {
			t.Errorf("%v: %s", d, got)
		}
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	c := Default()
	c.Collect.Profile = "minimal"
	data, err := c.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	doc, is := Parse(data, "roundtrip")
	if is.HasErrors() {
		t.Fatalf("%v\n%s", is, data)
	}
	if doc.Config.Limits != c.Limits || doc.Config.Output != c.Output {
		t.Errorf("round trip mismatch:\n%s", data)
	}
}

func TestFromBuiltinRoundTrip(t *testing.T) {
	c := Default()
	c.Collect.Profile = "standard"
	doc, err := FromBuiltin(c)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Source != "builtin" || len(doc.SHA256) != 64 || !bytes.HasPrefix(doc.Raw, []byte(BuiltinHeader)) {
		t.Fatalf("document: source %q sha %q", doc.Source, doc.SHA256)
	}
	sum := sha256.Sum256(doc.Raw)
	if hex.EncodeToString(sum[:]) != doc.SHA256 {
		t.Error("SHA256 is not the hash of Raw")
	}
	parsed, issues := Parse(doc.Raw, "test")
	if len(issues) != 0 {
		t.Fatalf("raw form does not parse cleanly: %v", issues)
	}
	if !reflect.DeepEqual(parsed.Config, c) {
		t.Errorf("round trip differs:\n%+v\n%+v", parsed.Config, c)
	}
}
