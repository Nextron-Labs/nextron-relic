package source

import "testing"

func TestSysnativePath(t *testing.T) {
	cases := []struct{ in, root, want string }{
		{`C:\Windows\System32\config\SAM`, `C:\Windows`, `C:\Windows\Sysnative\config\SAM`},
		{`c:\windows\system32\winevt\Logs\System.evtx`, `C:\Windows`, `C:\Windows\Sysnative\winevt\Logs\System.evtx`},
		{`C:\Windows\System32`, `C:\Windows\`, `C:\Windows\Sysnative`},
		{`C:\Windows\System32Backup\x`, `C:\Windows`, `C:\Windows\System32Backup\x`},
		{`C:\Windows\SysWOW64\x`, `C:\Windows`, `C:\Windows\SysWOW64\x`},
		{`C:\Windows\Prefetch\A.pf`, `C:\Windows`, `C:\Windows\Prefetch\A.pf`},
		{`D:\Windows\System32\x`, `C:\Windows`, `D:\Windows\System32\x`},
		{`C:\Windows\System32\x`, ``, `C:\Windows\System32\x`},
	}
	for _, c := range cases {
		if got := sysnativePath(c.in, c.root); got != c.want {
			t.Errorf("sysnativePath(%q, %q) = %q, want %q", c.in, c.root, got, c.want)
		}
	}
}
