//go:build linux

package source

import "syscall"

const noatimeFlag = syscall.O_NOATIME

func fillTimes(m *Meta, st *syscall.Stat_t) {
	m.Times.Modified = ts(int64(st.Mtim.Sec), int64(st.Mtim.Nsec))
	m.Times.Accessed = ts(int64(st.Atim.Sec), int64(st.Atim.Nsec))
	m.Times.Changed = ts(int64(st.Ctim.Sec), int64(st.Ctim.Nsec))
}
