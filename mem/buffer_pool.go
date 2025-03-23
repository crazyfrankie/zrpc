package mem

import (
	"math"
	"sync"
)

const (
	defaultMinBufferPoolSize = 1 << 6  // 64 B
	defaultMaxBufferPoolSize = 1 << 20 // 1 MB
)

var defaultBufferPool BufferPool

func init() {
	defaultBufferPool = NewLimitedPool(defaultMinBufferPoolSize, defaultMaxBufferPoolSize)
}

type BufferPool interface {
	Put(buf *[]byte)
	Get(length int) *[]byte
}

// DefaultBufferPool returns the current default buffer pool. It is a BufferPool
// created with NewLimitedPool that uses a set of default sizes optimized for
// expected workflows.
func DefaultBufferPool() BufferPool {
	return defaultBufferPool
}

type levelPool struct {
	size int
	pool sync.Pool
}

// newLevelPool returns a buffer pool of a specific size
func newLevelPool(size int) *levelPool {
	return &levelPool{
		size: size,
		pool: sync.Pool{
			New: func() interface{} {
				data := make([]byte, size)
				return &data
			},
		},
	}
}

// limitedPool is a collection of buffer pools of a specific size,
// with minSize and maxSize controlling the size of the buffer pools within it.
type limitedPool struct {
	minSize int
	maxSize int
	pools   []*levelPool
}

// NewLimitedPool builds a buffer pool of a specific size based on the provided minSize and maxSize,
// and the buffer pool size is multiplied by the multiplier, which is 2 by default.
func NewLimitedPool(minSize, maxSize int) BufferPool {
	if maxSize < minSize {
		panic("maxSize can't be less than minSize")
	}
	const multiplier = 2
	var pools []*levelPool
	curSize := minSize
	for curSize < maxSize {
		pools = append(pools, newLevelPool(curSize))
		curSize *= multiplier
	}
	pools = append(pools, newLevelPool(maxSize))
	return &limitedPool{
		minSize: minSize,
		maxSize: maxSize,
		pools:   pools,
	}
}

func (p *limitedPool) Get(size int) *[]byte {
	sp := p.findGetPool(size)
	if sp == nil {
		data := make([]byte, size)
		return &data
	}
	buf := sp.pool.Get().(*[]byte)
	*buf = (*buf)[:size]
	return buf
}

func (p *limitedPool) Put(b *[]byte) {
	sp := p.findPutPool(cap(*b))
	if sp == nil {
		return
	}
	*b = (*b)[:cap(*b)]
	sp.pool.Put(b)
}

func (p *limitedPool) findGetPool(size int) *levelPool {
	if size > p.maxSize {
		return nil
	}
	idx := int(math.Ceil(math.Log2(float64(size) / float64(p.minSize))))
	if idx < 0 {
		idx = 0
	}
	if idx > len(p.pools)-1 {
		return nil
	}
	return p.pools[idx]
}

func (p *limitedPool) findPutPool(size int) *levelPool {
	if size > p.maxSize {
		return nil
	}
	if size < p.minSize {
		return nil
	}

	idx := int(math.Floor(math.Log2(float64(size) / float64(p.minSize))))
	if idx < 0 {
		idx = 0
	}
	if idx > len(p.pools)-1 {
		return nil
	}
	return p.pools[idx]
}
