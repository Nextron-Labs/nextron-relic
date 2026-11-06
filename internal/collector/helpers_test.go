package collector

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/Nextron-Labs/nextron-relic/internal/catalog"
	"github.com/Nextron-Labs/nextron-relic/internal/pathspec"
	"github.com/Nextron-Labs/nextron-relic/pkg/config"
	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

func custom(name string, paths ...string) config.Artifact {
	return config.Artifact{Name: name, Sources: []config.Source{{Type: "file", OS: []string{"linux", "darwin"}, Paths: paths}}}
}

func baseConfig(t *testing.T, out string, arts ...config.Artifact) *config.Config {
	c := config.Default()
	c.Collect.CustomArtifacts = arts
	c.Collect.Hashes = []string{"md5", "sha256"}
	c.Limits.MinFreeSpace = 0
	c.Limits.Priority = config.PriorityNormal
	c.Output.Path = out
	if is := c.Validate(); is.HasErrors() {
		t.Fatal(is)
	}
	return c
}

func runCollect(t *testing.T, ctx context.Context, c *config.Config, mod func(*Options)) *result.Result {
	t.Helper()
	cat, err := catalog.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	o := Options{Doc: config.FromConfig(c, "test"), Catalog: cat, SkipPriority: true,
		Expander: &pathspec.Expander{Vars: pathspec.Vars{}}}
	if mod != nil {
		mod(&o)
	}
	return Run(ctx, o)
}

type archiveView struct {
	files    map[string][]byte
	records  []result.FileRecord
	manifest result.Result
}

func readArchive(t *testing.T, path string) archiveView {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	v := archiveView{files: map[string][]byte{}}
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		v.files[f.Name] = data
	}
	sc := bufio.NewScanner(bytes.NewReader(v.files[FilesName]))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r result.FileRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		v.records = append(v.records, r)
	}
	if err := json.Unmarshal(v.files[ManifestName], &v.manifest); err != nil {
		t.Fatal(err)
	}
	return v
}

func (v archiveView) record(t *testing.T, suffix string) result.FileRecord {
	t.Helper()
	for _, r := range v.records {
		if strings.HasSuffix(r.SourcePath, suffix) {
			return r
		}
	}
	t.Fatalf("no record for %s", suffix)
	return result.FileRecord{}
}
