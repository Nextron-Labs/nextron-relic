package collector

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nextron-Labs/nextron-relic/internal/ntfsraw"
	"github.com/Nextron-Labs/nextron-relic/internal/pathspec"
	"github.com/Nextron-Labs/nextron-relic/internal/source"
	"github.com/Nextron-Labs/nextron-relic/pkg/config"
	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

// TestRawNTFSPipeline runs Windows-style patterns with method raw against
// the go-ntfs test image mounted as drive C: (see internal/ntfsraw/testdata).
func TestRawNTFSPipeline(t *testing.T) {
	f, err := os.Open("../ntfsraw/testdata/test.ntfs.dd.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, _ := gzip.NewReader(f)
	img, _ := io.ReadAll(zr)
	vol, err := ntfsraw.Open(bytes.NewReader(img), nil, "C:")
	if err != nil {
		t.Fatal(err)
	}
	acc := source.NewWithRaw(&ntfsraw.FS{Opener: func(string) (*ntfsraw.Volume, error) { return vol, nil }})

	out := filepath.Join(t.TempDir(), "out.zip")
	raw := func(name string, trim bool, paths ...string) config.Artifact {
		return config.Artifact{Name: name, Sources: []config.Source{{Type: "file", OS: []string{"windows"}, Method: "raw", TrimSparse: trim, Paths: paths}}}
	}
	c := baseConfig(t, out,
		raw("custom.mft", false, `%SystemDrive%\$MFT`, `%SystemDrive%\$Secure:$SDS`),
		raw("custom.docs", false, `C:\Folder A\**\*.txt`, `C:\Folder A\Folder B\*:goodbye.txt`),
		raw("custom.sparse", true, `C:\$BadClus:$Bad`),
	)
	exp := &pathspec.Expander{Vars: pathspec.Vars{}}
	exp.Vars.Set("SystemDrive", "C:")
	res := runCollect(t, context.Background(), c, func(o *Options) {
		o.GOOS = "windows"
		o.Accessor = acc
		o.Expander = exp
	})
	if res.Status != result.StatusComplete {
		t.Fatalf("status %s %s %+v", res.Status, res.StatusReason, res.Artifacts)
	}
	v := readArchive(t, out)
	mft := v.record(t, `$MFT`)
	if mft.Method != source.MethodRaw || mft.ArchivePath != "files/C/$MFT" || !bytes.HasPrefix(v.files[mft.ArchivePath], []byte("FILE")) {
		t.Errorf("$MFT: %+v", mft)
	}
	if mft.Times.FNCreated == nil || mft.Inode == "" {
		t.Errorf("$MFT metadata missing: %+v", mft)
	}
	ads := v.record(t, `:goodbye.txt`)
	if ads.ArchivePath != "files/C/Folder A/Folder B/Hello world text document.txt%3Agoodbye.txt" || len(v.files[ads.ArchivePath]) == 0 {
		t.Errorf("ADS: %+v", ads)
	}
	sparse := v.record(t, `$BadClus:$Bad`)
	if sparse.DataOffset != sparse.Size || sparse.BytesStored != 0 || sparse.Status != result.FileCollected {
		t.Errorf("sparse trim: %+v", sparse)
	}
}
