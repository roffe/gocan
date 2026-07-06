package canlib

/*
#include <stdlib.h>
*/
import "C"

import (
	"runtime"
	"sync"
	"unsafe"
)

func prepFrameBufferC(data []byte) (ptr unsafe.Pointer, length uintptr) {
	if len(data) == 0 {
		return nil, 0
	}

	mem := C.malloc(C.size_t(len(data)))
	if mem == nil {
		panic("malloc failed")
	}

	// copy from Go slice into C memory
	dst := unsafe.Slice((*byte)(mem), len(data))
	copy(dst, data)

	return mem, uintptr(len(data))
}

// releaseFrameBufferC frees the C memory allocated by prepFrameBufferC.
func releaseFrameBufferC(ptr unsafe.Pointer) {
	if ptr == nil {
		return
	}
	C.free(unsafe.Pointer(ptr))
}

const (
	// Tune this. For classical CAN it's 8 bytes, CAN FD up to 64.
	// You can bump it if your adapter supports larger payloads.
	cbufDefaultCap = 64
)

// cBuf is a reusable C-allocated buffer.
type cBuf struct {
	ptr unsafe.Pointer // C.malloc'd memory
	cap int            // how many bytes fit in ptr
}

// global pool of *cBuf
var cbufPool = sync.Pool{
	New: func() any {
		mem := C.malloc(C.size_t(cbufDefaultCap))
		if mem == nil {
			panic("malloc failed")
		}
		cb := &cBuf{
			ptr: mem,
			cap: cbufDefaultCap,
		}

		// Safety net: if we *forget* to put it back,
		// GC of cb will eventually free the C memory.
		runtime.SetFinalizer(cb, func(x *cBuf) {
			if x.ptr != nil {
				C.free(unsafe.Pointer(x.ptr))
				x.ptr = nil
				x.cap = 0
			}
		})

		return cb
	},
}

// getCBuf returns a buffer that can hold at least n bytes.
// If the pooled buffer is too small, we malloc a bigger one just for this call.
func getCBuf(n int) (*cBuf, bool /*pooled*/) {
	raw := cbufPool.Get().(*cBuf)
	if n <= raw.cap {
		return raw, true
	}

	// pooled buffer too small for this frame.
	// We'll allocate a throwaway 'largeBuf' that won't go back in the pool.
	mem := C.malloc(C.size_t(n))
	if mem == nil {
		// Return the small buffer so it's not lost.
		cbufPool.Put(raw)
		panic("malloc failed (oversize frame)")
	}

	large := &cBuf{
		ptr: mem,
		cap: n,
	}
	// attach finalizer so even if caller forgets, it won't leak forever.
	runtime.SetFinalizer(large, func(x *cBuf) {
		if x.ptr != nil {
			C.free(unsafe.Pointer(x.ptr))
			x.ptr = nil
			x.cap = 0
		}
	})

	return large, false
}

// putCBuf returns a *pooled* buffer back into the pool.
// Oversize throwaway buffers are freed immediately instead.
func putCBuf(cb *cBuf, pooled bool) {
	if cb == nil || cb.ptr == nil {
		return
	}
	if pooled {
		// keep memory; just put the wrapper back
		cbufPool.Put(cb)
		return
	}

	// not pooled (oversize one-off) → free now
	C.free(unsafe.Pointer(cb.ptr))
	cb.ptr = nil
	cb.cap = 0
}
