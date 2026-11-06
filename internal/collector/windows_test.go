//go:build windows

package collector

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/Nextron-Labs/nextron-relic/internal/hostinfo"
	"github.com/Nextron-Labs/nextron-relic/internal/source"
	"github.com/Nextron-Labs/nextron-relic/pkg/config"
	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

// TestLiveWindowsCollection collects locked registry hives, the $MFT and
// event logs from the running system with the built-in artifact definitions.
func TestLiveWindowsCollection(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("requires an elevated process")
	}
	out := filepath.Join(t.TempDir(), "out.zip")
	c := config.Default()
	c.Collect.Artifacts = []string{"windows.registry.system", "windows.ntfs.mft", "windows.ntfs.usnjrnl", "windows.eventlogs"}
	c.Limits.MinFreeSpace = 0
	c.Limits.MaxFileSize = 512 * config.MiB
	c.Limits.Oversize = config.OversizeTruncate
	c.Output.Path = out
	res := runCollect(t, context.Background(), c, func(o *Options) { o.Expander = hostinfo.Expander() })
	if res.Status == result.StatusFailed || res.Output == nil {
		t.Fatalf("status %s: %s %+v", res.Status, res.StatusReason, res.Artifacts)
	}
	v := readArchive(t, out)
	sys := v.record(t, `config\SYSTEM`)
	if sys.Status != result.FileCollected || !strings.HasPrefix(string(v.files[sys.ArchivePath]), "regf") {
		t.Errorf("locked SYSTEM hive not collected: %+v", sys)
	}
	t.Logf("SYSTEM hive method=%s fallback_reason=%q", sys.Method, sys.FallbackReason)
	if mft := v.record(t, `\$MFT`); mft.Method != source.MethodRaw || mft.BytesStored == 0 {
		t.Errorf("$MFT: %+v", mft)
	}
	for _, a := range res.Artifacts {
		t.Logf("%s: %s collected=%d failed=%d bytes=%d errors=%v", a.Name, a.Status, a.FilesCollected, a.FilesFailed, a.BytesCollected, a.Errors)
	}
}
