package mem

import (
	"sync"
)

// BufferPool is a self-managed pool with various buffer sizes.
type BufferPool interface {
	// Get returns a buffer with the size.
	Get(size int) *[]byte
	// Put returns the buffer back to the pool.
	Put(buffer *[]byte)
}

type PoolConfig struct {
	MaxPoolSize int
}

var DefaultConfig = &PoolConfig{
	MaxPoolSize: 1024 * 1024 * 4, // 4MB
}

var defaultPool bufferPool

var bufferPoolSizes = []int{
	1 << 7,  // 128B
	1 << 8,  // 256B
	1 << 9,  // 512B
	1 << 10, // 1KB
	1 << 11, // 2KB
	1 << 12, // 4KB
	1 << 13, // 8KB
	1 << 14, // 16KB
	1 << 15, // 32KB
	1 << 16, // 64KB
	1 << 17, // 128KB
	1 << 18, // 256KB
	1 << 19, // 512KB
	1 << 20, // 1MB
	1 << 21, // 2MB
	1 << 22, // 4MB
}

type bufferPool struct {
	config  *PoolConfig
	pools   []*sync.Pool
	maxSize int
}

func init() {
	defaultPool.config = DefaultConfig
	defaultPool.maxSize = bufferPoolSizes[len(bufferPoolSizes)-1]
	defaultPool.pools = make([]*sync.Pool, len(bufferPoolSizes))

	for i := range bufferPoolSizes {
		size := bufferPoolSizes[i]
		defaultPool.pools[i] = &sync.Pool{
			New: func() interface{} {
				buf := make([]byte, 0, size)
				return &buf
			},
		}
	}
}

// DefaultBufferPool returns the default pool
func DefaultBufferPool() BufferPool {
	return &defaultPool
}

// Get returns a buffer with the size.
func (p *bufferPool) Get(size int) *[]byte {
	if size <= 0 {
		return &[]byte{}
	}

	if i, exactMatch := p.findPool(size); exactMatch {
		buf := p.pools[i].Get().(*[]byte)
		*buf = (*buf)[0:size]
		return buf
	}

	index := p.findBestFitPool(size)
	if index >= 0 {
		buf := p.pools[index].Get().(*[]byte)
		*buf = (*buf)[0:size]
		return buf
	}

	buf := make([]byte, size)
	return &buf
}

// Put returns the buffer to the pool.
func (p *bufferPool) Put(buffer *[]byte) {
	if buffer == nil {
		return
	}

	size := cap(*buffer)
	if size <= 0 || size > p.maxSize {
		return
	}

	*buffer = (*buffer)[:0]

	if index, exact := p.findPool(size); exact {
		p.pools[index].Put(buffer)
		return
	}

	index := p.findClosestPool(size)
	if index >= 0 {
		p.pools[index].Put(buffer)
	}
}

func (p *bufferPool) findPool(size int) (int, bool) {
	for i, poolSize := range bufferPoolSizes {
		if size == poolSize {
			return i, true
		}
	}
	return -1, false
}

func (p *bufferPool) findBestFitPool(size int) int {
	for i, poolSize := range bufferPoolSizes {
		if size <= poolSize {
			return i
		}
	}
	return -1
}

func (p *bufferPool) findClosestPool(size int) int {
	for i := len(bufferPoolSizes) - 1; i >= 0; i-- {
		if size >= bufferPoolSizes[i] {
			return i
		}
	}
	return 0
}
