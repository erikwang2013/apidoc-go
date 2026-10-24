// Package gxbytes is the subset of github.com/dubbogo/gost/bytes that
// dubbo-go-hessian2 uses. AcquireBytes returns a slice of length >= size,
// matching the upstream pool's contract.
package gxbytes

import "sync"

var defaultPool = sync.Pool{New: func() any { b := make([]byte, 0, 65536); return &b }}

// AcquireBytes returns a length-size slice with capacity >= size.
func AcquireBytes(size int) *[]byte {
	p := defaultPool.Get().(*[]byte)
	b := (*p)[:size]
	if cap(b) < size {
		b = make([]byte, size)
	}
	return &b
}

// ReleaseBytes returns a slice to the pool.
func ReleaseBytes(b *[]byte) { defaultPool.Put(b) }
