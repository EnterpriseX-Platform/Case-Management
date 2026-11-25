// Package id generates collision-resistant CUID2-style identifiers
// (lower-case, starts with a letter, 24 chars). All public ids, including
// those used in URLs, are CUIDs.
package id

import (
	"crypto/rand"
	"crypto/sha3"
	"encoding/binary"
	"math/big"
	"strconv"
	"sync/atomic"
	"time"
)

var counter atomic.Uint64

func init() {
	var b [8]byte
	_, _ = rand.Read(b[:])
	counter.Store(binary.BigEndian.Uint64(b[:]) >> 16)
}

const length = 24

// New returns a new CUID2-style id.
func New() string {
	var salt [32]byte
	_, _ = rand.Read(salt[:])
	h := sha3.New512()
	h.Write([]byte(strconv.FormatInt(time.Now().UnixNano(), 36)))
	h.Write([]byte(strconv.FormatUint(counter.Add(1), 36)))
	h.Write(salt[:])
	s := new(big.Int).SetBytes(h.Sum(nil)).Text(36)
	var first [1]byte
	_, _ = rand.Read(first[:])
	return string(rune('a'+int(first[0])%26)) + s[1:length]
}
