// Package verify checks a Relic archive: ZIP integrity, the metadata files,
// and every stored file against the size and hashes recorded in files.jsonl.
package verify

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/Nextron-Labs/nextron-relic/internal/hashing"
	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

// Archive entry names written by the collector.
const (
	manifestName        = "manifest.json"
	filesName           = "files.jsonl"
	logName             = "relic.log"
	configEffectiveName = "config.effective.yaml"
	filesPrefix         = "files/"
)

// maxProblems bounds the problems listed in a report.
const maxProblems = 100

// Report is the outcome of a verification. Its JSON form is printed by
// "relic verify --json".
type Report struct {
	Archive string `json:"archive"`
	// OK is true if no problem was found.
	OK            bool   `json:"ok"`
	ArchiveSize   int64  `json:"archive_size"`
	ArchiveSHA256 string `json:"archive_sha256"`
	// Collection describes the run that produced the archive (from manifest.json).
	Collection *Collection `json:"collection,omitempty"`
	// FilesVerified counts stored files whose size and hashes match.
	FilesVerified int   `json:"files_verified"`
	BytesVerified int64 `json:"bytes_verified"`
	// Records counts file records by status.
	Records         map[string]int `json:"records"`
	Problems        []string       `json:"problems"`
	ProblemsDropped int            `json:"problems_dropped,omitempty"`
}

// Collection summarizes manifest.json.
type Collection struct {
	Tool       string        `json:"tool"`
	Host       string        `json:"host"`
	OS         string        `json:"os"`
	Status     result.Status `json:"status"`
	StartedAt  string        `json:"started_at"`
	JobID      string        `json:"job_id,omitempty"`
	ConfigHash string        `json:"config_sha256,omitempty"`
}

func (r *Report) problem(format string, args ...interface{}) {
	if len(r.Problems) >= maxProblems {
		r.ProblemsDropped++
		return
	}
	r.Problems = append(r.Problems, fmt.Sprintf(format, args...))
}

// Options for Archive.
type Options struct {
	// Expect is the output section of the collection result (--json or
	// --status-file). If set, the archive size and SHA-256 must match it.
	Expect *result.Output
}

// Archive verifies the archive at path. An error is returned only if the
// file cannot be read at all; everything else is reported as a problem.
func Archive(path string, o Options) (*Report, error) {
	rep := &Report{Archive: path, Records: map[string]int{}, Problems: []string{}}
	size, sum, err := fileSHA256(path)
	if err != nil {
		return nil, err
	}
	rep.ArchiveSize, rep.ArchiveSHA256 = size, sum
	if e := o.Expect; e != nil {
		if e.Size != 0 && e.Size != size {
			rep.problem("archive size %d differs from the result (%d)", size, e.Size)
		}
		if e.SHA256 != "" && !strings.EqualFold(e.SHA256, sum) {
			rep.problem("archive SHA-256 %s differs from the result (%s)", sum, e.SHA256)
		}
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		rep.problem("not a readable ZIP archive: %v", err)
		return rep.done(), nil
	}
	defer zr.Close()

	entries := map[string]*zip.File{}
	for _, f := range zr.File {
		if _, dup := entries[f.Name]; dup {
			rep.problem("duplicate archive entry %s", f.Name)
		}
		entries[f.Name] = f
	}
	for _, n := range []string{manifestName, filesName, logName, configEffectiveName} {
		if entries[n] == nil {
			rep.problem("missing %s", n)
		}
	}

	var manifest *result.Result
	if f := entries[manifestName]; f != nil {
		data, err := readAll(f)
		if err != nil {
			rep.problem("%s: %v", manifestName, err)
		} else if err := json.Unmarshal(data, &manifest); err != nil {
			rep.problem("%s is not a valid result: %v", manifestName, err)
			manifest = nil
		} else {
			if manifest.SchemaVersion != result.SchemaVersion {
				rep.problem("%s has schema version %d (this build understands %d)", manifestName, manifest.SchemaVersion, result.SchemaVersion)
			}
			rep.Collection = &Collection{
				Tool:       strings.TrimSpace(manifest.Tool.Name + " " + manifest.Tool.Version),
				Host:       manifest.Host.Hostname,
				OS:         manifest.Host.OS,
				Status:     manifest.Status,
				StartedAt:  manifest.StartedAt.UTC().Format("2006-01-02T15:04:05Z"),
				JobID:      manifest.Job.ID,
				ConfigHash: manifest.Config.SHA256,
			}
		}
	}

	recorded := map[string]bool{}
	if f := entries[filesName]; f != nil {
		rep.checkRecords(f, entries, recorded)
	}
	if manifest != nil {
		rep.checkSummary(manifest)
	}

	// Every stored file must have a record; metadata entries must be readable
	// (reading checks their CRC-32).
	var unrecorded []string
	for name, f := range entries {
		switch {
		case strings.HasPrefix(name, filesPrefix) && !strings.HasSuffix(name, "/"):
			if !recorded[name] {
				unrecorded = append(unrecorded, name)
			}
		case name == manifestName || name == filesName:
			// already read
		default:
			if err := drain(f); err != nil {
				rep.problem("%s: %v", name, err)
			}
		}
	}
	sort.Strings(unrecorded)
	for _, n := range unrecorded {
		rep.problem("%s is stored but has no record in %s", n, filesName)
	}
	return rep.done(), nil
}

func (r *Report) done() *Report {
	r.OK = len(r.Problems) == 0 && r.ProblemsDropped == 0
	return r
}

// checkRecords verifies every record that references stored data.
func (r *Report) checkRecords(f *zip.File, entries map[string]*zip.File, recorded map[string]bool) {
	rc, err := f.Open()
	if err != nil {
		r.problem("%s: %v", filesName, err)
		return
	}
	defer rc.Close()
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	line := 0
	for sc.Scan() {
		line++
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var rec result.FileRecord
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			r.problem("%s line %d: invalid record: %v", filesName, line, err)
			continue
		}
		r.Records[string(rec.Status)]++
		if rec.ArchivePath == "" {
			continue
		}
		if recorded[rec.ArchivePath] {
			r.problem("%s line %d: %s is referenced by more than one record", filesName, line, rec.ArchivePath)
			continue
		}
		recorded[rec.ArchivePath] = true
		e := entries[rec.ArchivePath]
		if e == nil {
			r.problem("%s (%s) is recorded but missing from the archive", rec.ArchivePath, rec.SourcePath)
			continue
		}
		r.checkFile(e, &rec)
	}
	if err := sc.Err(); err != nil {
		r.problem("%s: %v", filesName, err)
	}
}

func (r *Report) checkFile(e *zip.File, rec *result.FileRecord) {
	if len(rec.Hashes) == 0 {
		r.problem("%s (%s) has no recorded hash", rec.ArchivePath, rec.SourcePath)
		return
	}
	names := make([]string, 0, len(rec.Hashes))
	for n := range rec.Hashes {
		names = append(names, n)
	}
	sort.Strings(names)
	hs := hashing.New(names)
	rc, err := e.Open()
	if err != nil {
		r.problem("%s: %v", rec.ArchivePath, err)
		return
	}
	n, err := io.Copy(hs, rc)
	rc.Close()
	if err != nil {
		r.problem("%s: %v", rec.ArchivePath, err)
		return
	}
	ok := true
	if n != rec.BytesStored {
		r.problem("%s (%s): %d bytes stored, record says %d", rec.ArchivePath, rec.SourcePath, n, rec.BytesStored)
		ok = false
	}
	sums := hs.Sums()
	for _, alg := range names {
		got, known := sums[alg]
		if !known {
			r.problem("%s: unsupported hash algorithm %q in record", rec.ArchivePath, alg)
			ok = false
			continue
		}
		if !strings.EqualFold(got, rec.Hashes[alg]) {
			r.problem("%s (%s): %s mismatch (stored data %s, record %s)", rec.ArchivePath, rec.SourcePath, alg, got, rec.Hashes[alg])
			ok = false
		}
	}
	if ok {
		r.FilesVerified++
		r.BytesVerified += n
	}
}

// checkSummary compares the manifest counters with the records.
func (r *Report) checkSummary(m *result.Result) {
	s := m.Summary
	for _, c := range []struct {
		status   result.FileStatus
		manifest int
	}{
		{result.FileCollected, s.FilesCollected},
		{result.FileTruncated, s.FilesTruncated},
	} {
		if got := r.Records[string(c.status)]; got != c.manifest {
			r.problem("%s lists %d %s files, the manifest summary %d", filesName, got, c.status, c.manifest)
		}
	}
}

func fileSHA256(path string) (int64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return 0, "", err
	}
	if fi.IsDir() {
		return 0, "", errors.New("is a directory")
	}
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func readAll(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, 256<<20))
}

func drain(f *zip.File) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(io.Discard, rc)
	return err
}
