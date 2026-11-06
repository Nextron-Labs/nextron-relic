//go:build darwin

package source

import "syscall"

const noatimeFlag = 0

func fillTimes(m *Meta, st *syscall.Stat_t) {
	m.Times.Modified = ts(st.Mtimespec.Sec, st.Mtimespec.Nsec)
	m.Times.Accessed = ts(st.Atimespec.Sec, st.Atimespec.Nsec)
	m.Times.Changed = ts(st.Ctimespec.Sec, st.Ctimespec.Nsec)
	m.Times.Created = ts(st.Birthtimespec.Sec, st.Birthtimespec.Nsec)
}
