package hashing

import "testing"

func TestSet(t *testing.T) {
	s := New([]string{"md5", "sha1", "sha256", "sha512", "crc32"})
	s.Write([]byte("ab"))
	s.Write([]byte("c"))
	got := s.Sums()
	want := map[string]string{
		"md5":    "900150983cd24fb0d6963f7d28e17f72",
		"sha1":   "a9993e364706816aba3e25717850c26c9cd0d89d",
		"sha256": "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		"sha512": "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f",
	}
	if len(got) != len(want) {
		t.Fatalf("unknown algorithms must be ignored: %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %s, want %s", k, got[k], v)
		}
	}
}
