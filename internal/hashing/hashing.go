// Package hashing computes several hashes of a byte stream in one pass.
package hashing

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
)

// Set hashes data with several algorithms at once. It implements io.Writer.
type Set struct {
	names []string
	hs    []hash.Hash
}

// New returns a Set for the given algorithm names (md5, sha1, sha256,
// sha512). Unknown names are ignored.
func New(names []string) *Set {
	s := &Set{}
	for _, n := range names {
		var h hash.Hash
		switch n {
		case "md5":
			h = md5.New()
		case "sha1":
			h = sha1.New()
		case "sha256":
			h = sha256.New()
		case "sha512":
			h = sha512.New()
		default:
			continue
		}
		s.names = append(s.names, n)
		s.hs = append(s.hs, h)
	}
	return s
}

// Write adds p to every hash. It never fails.
func (s *Set) Write(p []byte) (int, error) {
	for _, h := range s.hs {
		h.Write(p)
	}
	return len(p), nil
}

// Sums returns the hex digests by algorithm name.
func (s *Set) Sums() map[string]string {
	out := make(map[string]string, len(s.hs))
	for i, h := range s.hs {
		out[s.names[i]] = hex.EncodeToString(h.Sum(nil))
	}
	return out
}
