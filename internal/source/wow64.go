package source

import "strings"

// sysnativePath rewrites paths below <systemRoot>\System32 to
// <systemRoot>\Sysnative. A 32-bit process on 64-bit Windows (WoW64) is
// silently redirected from System32 to SysWOW64 by the file system
// redirector; the Sysnative alias reaches the real System32 directory.
// Other paths are returned unchanged. The comparison is case-insensitive.
func sysnativePath(p, systemRoot string) string {
	systemRoot = strings.TrimRight(systemRoot, `\/`)
	if systemRoot == "" {
		return p
	}
	prefix := systemRoot + `\System32`
	if len(p) < len(prefix) || !strings.EqualFold(p[:len(prefix)], prefix) {
		return p
	}
	if len(p) > len(prefix) && p[len(prefix)] != '\\' && p[len(prefix)] != '/' {
		return p // e.g. System32Backup
	}
	return systemRoot + `\Sysnative` + p[len(prefix):]
}
