package mem

import (
	"bytes"
	"sync"
)

const (
	defaultMaxSize = 256 * 1024
)

var (
	once   sync.Once
	Buffer *BufferPool
)

func GetBuffer() *BufferPool {
	once.Do(func() {
		Buffer = NewBufferPool(defaultMaxSize)
	})
	return Buffer
}

type BufferPool struct {
	pool    sync.Pool
	maxSize int
}

func NewBufferPool(maxSize int) *BufferPool {
	return &BufferPool{
		maxSize: maxSize,
		pool: sync.Pool{
			New: func() interface{} {
				return &bytes.Buffer{}
			},
		},
	}
}

func (bp *BufferPool) Get(size int) *[]byte {
	buf := bp.pool.Get().(*bytes.Buffer)

	buf.Reset()

	if size > buf.Cap() {
		buf.Grow(min(size, bp.maxSize) - buf.Cap())
	}

	f := buf.Bytes()
	return &f
}

func (bp *BufferPool) Put(buffer *[]byte) {
	buf := bytes.NewBuffer(*buffer)
	buf.Reset()

	bp.pool.Put(buf)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
