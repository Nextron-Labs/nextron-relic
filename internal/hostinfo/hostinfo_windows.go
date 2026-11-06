//go:build windows

package hostinfo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/Nextron-Labs/nextron-relic/internal/pathspec"
)

func elevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return strings.TrimRight(v, `\`)
	}
	return def
}

func platformExpander(v pathspec.Vars) *pathspec.Expander {
	sysDrive := env("SystemDrive", "C:")
	sysRoot := env("SystemRoot", sysDrive+`\Windows`)
	v.Set("SystemDrive", sysDrive)
	v.Set("SystemRoot", sysRoot)
	v.Set("WinDir", env("windir", sysRoot))
	v.Set("ProgramData", env("ProgramData", sysDrive+`\ProgramData`))
	v.Set("ProgramFiles", env("ProgramFiles", sysDrive+`\Program Files`))
	// Not present on 32-bit Windows: the variable is known but has no value,
	// so patterns using it expand to nothing instead of failing.
	if pf86 := env("ProgramFiles(x86)", ""); pf86 != "" {
		v.Set("ProgramFiles(x86)", pf86)
	} else {
		v.Set("ProgramFiles(x86)")
	}
	return &pathspec.Expander{Vars: v, Env: func(name string) (string, bool) {
		val, ok := os.LookupEnv(name)
		return strings.TrimRight(val, `\`), ok
	}}
}

// UserHomes returns profile directories from the registry ProfileList
// (including LocalSystem, LocalService and NetworkService) and the
// directories below %SystemDrive%\Users.
func UserHomes() []string {
	var cands []string
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList`, registry.ENUMERATE_SUB_KEYS|registry.READ)
	if err == nil {
		names, _ := k.ReadSubKeyNames(-1)
		for _, n := range names {
			sk, err := registry.OpenKey(k, n, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			p, _, err := sk.GetStringValue("ProfileImagePath")
			sk.Close()
			if err != nil || p == "" {
				continue
			}
			if exp, err := registry.ExpandString(p); err == nil {
				p = exp
			}
			cands = append(cands, p)
		}
		k.Close()
	}
	usersDir := env("SystemDrive", "C:") + `\Users`
	if entries, err := os.ReadDir(usersDir); err == nil {
		for _, e := range entries {
			// Junctions such as "All Users" and "Default User" are skipped.
			if e.IsDir() && e.Type()&(os.ModeSymlink|os.ModeIrregular) == 0 {
				cands = append(cands, filepath.Join(usersDir, e.Name()))
			}
		}
	}
	return dedupeDirs(cands, true)
}

// FreeSpace returns the bytes available to the process on the volume of dir.
func FreeSpace(dir string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var avail, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &total, &free); err != nil {
		return 0, err
	}
	return avail, nil
}

// LowerPriority sets BELOW_NORMAL_PRIORITY_CLASS and low I/O priority.
func LowerPriority() error {
	if err := windows.SetPriorityClass(windows.CurrentProcess(), windows.BELOW_NORMAL_PRIORITY_CLASS); err != nil {
		return err
	}
	ioPriority := uint32(1) // IoPriorityLow
	return windows.NtSetInformationProcess(windows.CurrentProcess(), windows.ProcessIoPriority,
		unsafe.Pointer(&ioPriority), uint32(unsafe.Sizeof(ioPriority)))
}

func osVersion() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return "Windows"
	}
	defer k.Close()
	name, _, _ := k.GetStringValue("ProductName")
	display, _, _ := k.GetStringValue("DisplayVersion")
	build, _, _ := k.GetStringValue("CurrentBuild")
	ubr, _, _ := k.GetIntegerValue("UBR")
	// ProductName still says "Windows 10" on Windows 11; build >= 22000 is 11.
	if strings.HasPrefix(name, "Windows 10") && build >= "22000" && len(build) >= 5 {
		name = "Windows 11" + strings.TrimPrefix(name, "Windows 10")
	}
	v := strings.TrimSpace(name + " " + display)
	if build != "" {
		v += fmt.Sprintf(" (build %s.%d)", build, ubr)
	}
	return v
}
