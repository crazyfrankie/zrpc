package mem

import "io"

const (
	// 32 KiB is what io.Copy uses.
	readAllBufSize = 32 * 1024
)

type BufferSlice []Buffer

// Len returns the sum of the length of all the Buffers in this slice.
func (s BufferSlice) Len() int {
	length := 0
	for _, b := range s {
		length += b.Len()
	}

	return length
}

// Free invokes Buffer.Free() on each Buffer in the slice.
func (s BufferSlice) Free() {
	for _, b := range s {
		b.Free()
	}
}

// Ref invokes Ref on each buffer in the slice.
func (s BufferSlice) Ref() {
	for _, b := range s {
		b.Ref()
	}
}

// Materialize concatenates all the underlying Buffer's data into a single
// contiguous buffer using CopyTo.
func (s BufferSlice) Materialize() []byte {
	l := s.Len()
	if l == 0 {
		return nil
	}
	out := make([]byte, l)
	s.CopyTo(out)
	return out
}

// MaterializeToBuffer is similar to Materialize, except that
// it works with a single buffer that needs to be pooled.
//
// Special handling of the case where there is only one Buffer, by incrementing the reference count without copying.
// Handle the case of an empty BufferSlice by returning a special emptyBuffer
// Get the right sized buffer from the pool to reduce memory allocation.
func (s BufferSlice) MaterializeToBuffer(pool BufferPool) Buffer {
	if len(s) == 1 {
		s[0].Ref()
		return s[0]
	}
	sLen := s.Len()
	if sLen == 0 {
		return emptyBuffer{}
	}
	buf := pool.Get(sLen)
	s.CopyTo(*buf)
	return NewBuffer(buf, pool)
}

// CopyTo copies the data from the underlying buffer to the given buffer dst,
// returning the number of bytes copied. The semantics are the same as those of
// the copy built-in function; it will copy as many bytes as possible,
// stopping when dst is full or s runs out of data, and returning the minimum of s.Len() and len(dst).
func (s BufferSlice) CopyTo(dst []byte) int {
	off := 0
	for _, b := range s {
		off += copy(dst[off:], b.ReadOnlyData())
	}
	return off
}

// Copy creates a Buffer with the given data,
// and the reference count of the Buffer is one.
func Copy(data []byte, pool BufferPool) Buffer {
	if IsLessBufferPoolThreshold(len(data)) {
		buf := make(SliceBuffer, len(data))
		copy(buf, data)
		return buf
	}

	buf := pool.Get(len(data))
	copy(*buf, data)
	return NewBuffer(buf, pool)
}

// NewReader returns a new Reader for the input slice after taking references to
// each underlying buffer.
func (s BufferSlice) NewReader() Reader {
	s.Ref()
	return &sliceReader{
		data: s,
		len:  s.Len(),
	}
}

type Reader interface {
	io.Reader
	io.ByteReader
	// Close frees the underlying BufferSlice and never returns an error. Subsequent
	// calls to Read will return (0, io.EOF).
	Close() error
	// Remain returns the number of unread bytes remaining in the slice.
	Remain() int
}

type sliceReader struct {
	data      BufferSlice // Referenced BufferSlice
	len       int         // Number of bytes remaining unread
	bufferIdx int         // The offset in the buffer currently being read.
}

func (s *sliceReader) Read(buf []byte) (n int, err error) {
	if s.len == 0 {
		return 0, io.EOF
	}

	// If the target slice length is not null and
	// the remaining length of the current buffer is not zero, then read the data.
	for len(buf) != 0 && s.len != 0 {
		data := s.data[0].ReadOnlyData()
		cp := copy(buf, data[s.bufferIdx:])
		s.len -= cp
		s.bufferIdx += cp
		n += cp
		buf = buf[cp:]

		s.freeFirstBufferIfEmpty()
	}

	return n, nil
}

func (s *sliceReader) ReadByte() (byte, error) {
	if s.len == 0 {
		return 0, io.EOF
	}

	// There may be any number of empty buffers in the slice, clear them all until a
	// non-empty buffer is reached. This is guaranteed to exit since r.len is not 0.
	for s.freeFirstBufferIfEmpty() {
	}

	b := s.data[0].ReadOnlyData()[s.bufferIdx]
	s.bufferIdx++
	s.len--
	// Free the first buffer in the slice if the last byte was read
	s.freeFirstBufferIfEmpty()
	return b, nil
}

func (s *sliceReader) Close() error {
	s.data.Free()
	s.data = nil
	s.len = 0
	return nil
}

func (s *sliceReader) Remain() int {
	return s.len
}

// freeFirstBufferIfEmpty checks and frees the finished buffer,
// automatically moving to the next one.
func (s *sliceReader) freeFirstBufferIfEmpty() bool {
	// false if there is no buffer to free,
	// false if the current buffer has not been read (bufferIdx is not equal to the length of the buffer).
	if len(s.data) == 0 || s.bufferIdx != len(s.data[0].ReadOnlyData()) {
		return false
	}

	// Release the Buffer after it's been read.
	s.data[0].Free()
	// Reset the index, ready to read the next buffer.
	s.data = s.data[1:]
	s.bufferIdx = 0
	return true
}

var _ io.Writer = (*writer)(nil)

type writer struct {
	buffers *BufferSlice
	pool    BufferPool
}

func (w writer) Write(p []byte) (n int, err error) {
	b := Copy(p, w.pool)
	*w.buffers = append(*w.buffers, b)
	return b.Len(), nil
}

// NewWriter returns a writer that encapsulates a BufferSlice and BufferPool to implement the
// io.Writer interface. Each call to Write copies the contents of the given
// BufferSlice and BufferPool into a new Buffer from the given pool,
// and that Buffer added to the given BufferSlice.
func NewWriter(buffers *BufferSlice, pool BufferPool) io.Writer {
	return &writer{
		buffers: buffers,
		pool:    pool,
	}
}

func ReadAll(r io.Reader, pool BufferPool) (BufferSlice, error) {
	var result BufferSlice
	if wt, ok := r.(io.WriterTo); ok {
		// This is more optimal since wt knows the size of chunks it wants to
		// write and, hence, we can allocate buffers of an optimal size to fit
		// them. E.g. might be a single big chunk, and we wouldn't chop it
		// into pieces.
		w := NewWriter(&result, pool)
		_, err := wt.WriteTo(w)
		return result, err
	}
nextBuffer:
	for {
		buf := pool.Get(readAllBufSize)
		// We asked for 32KiB but may have been given a bigger buffer.
		// Use all of it if that's the case.
		*buf = (*buf)[:cap(*buf)]
		usedCap := 0
		for {
			n, err := r.Read((*buf)[usedCap:])
			usedCap += n
			if err != nil {
				if usedCap == 0 {
					// Nothing in this buf, put it back
					pool.Put(buf)
				} else {
					*buf = (*buf)[:usedCap]
					result = append(result, NewBuffer(buf, pool))
				}
				if err == io.EOF {
					err = nil
				}
				return result, err
			}
			if len(*buf) == usedCap {
				result = append(result, NewBuffer(buf, pool))
				continue nextBuffer
			}
		}
	}
}
