package core

import (
	cryptoRand "crypto/rand"
	"encoding/hex"
	"time"
)

func NewID(prefix string) string {
	var b [10]byte
	_, _ = cryptoRand.Read(b[:])
	ts := time.Now().UnixMilli()
	return prefix + "_" + itoa(ts) + "_" + hex.EncodeToString(b[:])
}

func itoa(n int64) string {
	return (func(x int64) string { return string([]byte(fmtInt(x))) })(n)
}

func fmtInt(n int64) []byte {
	var a [20]byte
	i := len(a)
	neg := n < 0
	if neg {
		n = -n
	}
	for {
		i--
		a[i] = byte('0' + n%10)
		n /= 10
		if n == 0 {
			break
		}
	}
	if neg {
		i--
		a[i] = '-'
	}
	return a[i:]
}
