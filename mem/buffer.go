package mem

import (
	"sync"
	"sync/atomic"
)

var (
	bufferPoolingThreshold = 1 << 10

	bufferObjectPool = sync.Pool{New: func() any { return new(buffer) }}
	refObjectPool    = sync.Pool{New: func() any { return new(atomic.Int32) }}
)

type Buffer interface {
	// ReadOnlyData returns the underlying byte slice,
	// note that it is immutable.
	ReadOnlyData() []byte
	// Ref increases the reference counter for this Buffer.
	Ref()
	// Free decrements this Buffer's reference counter and frees the underlying
	// byte slice if the counter reaches 0 as a result of this call.
	Free()
	// Len returns the Buffer's size.
	Len() int

	read([]byte) (int, Buffer)
	split(int) (Buffer, Buffer)
}

// NewBuffer initializes the Buffer with the given data
// and initializes the counter to 1. When all the references held by the Buffer are released,
// the Buffer is returned to the pool.
func NewBuffer(data *[]byte, pool BufferPool) Buffer {
	if pool == nil && IsLessBufferPoolThreshold(cap(*data)) {
		return (SliceBuffer)(*data)
	}

	b := newBuffer()
	b.originData = data
	b.data = *data
	b.pool = pool
	b.refs = refObjectPool.Get().(*atomic.Int32)
	b.refs.Add(1)
	return b
}

// Split modifies the receiver to point to the first n bytes while it
// returns a new reference to the remaining bytes. The returned Buffer
// functions just like a normal reference acquired using Ref().
func Split(buf Buffer, n int) (Buffer, Buffer) {
	return buf.split(n)
}

// Read reads bytes from the given Buffer into the provided slice.
func Read(buf Buffer, data []byte) (int, Buffer) {
	return buf.read(data)
}

type buffer struct {
	originData *[]byte
	data       []byte
	refs       *atomic.Int32
	pool       BufferPool
}

func newBuffer() *buffer {
	return bufferObjectPool.Get().(*buffer)
}

func (b *buffer) ReadOnlyData() []byte {
	if b.refs == nil {
		panic("cannot read freed buffer")
	}
	return b.data
}

func (b *buffer) read(buf []byte) (int, Buffer) {
	if b.refs == nil {
		panic("cannot read freed buffer")
	}

	n := copy(buf, b.data)
	if n == len(b.data) {
		b.Free()
		return n, nil
	}

	b.data = b.data[n:]
	return n, b
}

func (b *buffer) Ref() {
	if b.refs == nil {
		panic("cannot ref freed buffer")
	}
	b.refs.Add(1)
}

func (b *buffer) Free() {
	if b.refs == nil {
		panic("cannot fred freed buffer")
	}

	refs := b.refs.Add(-1)
	switch {
	case refs > 0:
		return
	case refs == 0:
		if b.pool != nil {
			b.pool.Put(b.originData)
		}

		refObjectPool.Put(b.refs)
		b.originData = nil
		b.data = nil
		b.refs = nil
		b.pool = nil
		bufferObjectPool.Put(b)
	default:
		panic("cannot free freed buffer")
	}
}

func (b *buffer) Len() int {
	return len(b.ReadOnlyData())
}

func (b *buffer) split(n int) (Buffer, Buffer) {
	if b.refs == nil {
		panic("Cannot split freed buffer")
	}

	b.refs.Add(1)
	split := newBuffer()
	split.originData = b.originData
	split.data = b.data[n:]
	split.refs = b.refs
	split.pool = b.pool

	b.data = b.data[:n]

	return b, split
}

// IsLessBufferPoolThreshold is used to determine if the current required size
// reaches the threshold size for adopting a buffer.
func IsLessBufferPoolThreshold(size int) bool {
	return size <= bufferPoolingThreshold
}

// SliceBuffer is a Buffer implementation that wraps a byte slice. It provides
// methods for reading, splitting, and managing the byte slice.
// Use it when the required size does not reach the threshold.
type SliceBuffer []byte

func (s SliceBuffer) ReadOnlyData() []byte {
	return s
}

func (s SliceBuffer) Ref() {}

func (s SliceBuffer) Free() {}

func (s SliceBuffer) Len() int {
	return len(s)
}

func (s SliceBuffer) read(buf []byte) (int, Buffer) {
	n := copy(buf, s)
	if n == len(s) {
		return n, nil
	}

	return n, s[n:]
}

func (s SliceBuffer) split(n int) (Buffer, Buffer) {
	return s[:n], s[n:]
}

type emptyBuffer struct{}

func (e emptyBuffer) ReadOnlyData() []byte {
	return nil
}

func (e emptyBuffer) Ref()  {}
func (e emptyBuffer) Free() {}

func (e emptyBuffer) Len() int {
	return 0
}

func (e emptyBuffer) split(int) (left, right Buffer) {
	return e, e
}

func (e emptyBuffer) read([]byte) (int, Buffer) {
	return 0, e
}
