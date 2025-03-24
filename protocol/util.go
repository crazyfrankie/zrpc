package protocol

import (
	"bytes"
	"compress/gzip"
	"io"
	"sync"
	"unsafe"
)

const (
	FileChunkSize     = 16 * 1024
	CompressThreshold = 1024 // 1KB
)

var (
	spWriter sync.Pool
	spReader sync.Pool

	bufferPool = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, FileChunkSize)
			return &buf
		},
	}
)

func init() {
	spWriter = sync.Pool{New: func() interface{} {
		w, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
		return w
	}}
	spReader = sync.Pool{New: func() interface{} {
		return new(gzip.Reader)
	}}
}

func unzip(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	gr := spReader.Get().(*gzip.Reader)

	br := bytes.NewReader(data)

	if err := gr.Reset(br); err != nil {
		spReader.Put(gr)
		return nil, err
	}

	estimatedSize := len(data) * 5
	if estimatedSize < 4*1024 {
		estimatedSize = 4 * 1024
	}

	result := bytes.NewBuffer(make([]byte, 0, estimatedSize))

	tmpBufPtr := bufferPool.Get().(*[]byte)
	tmpBuf := *tmpBufPtr

	var err error

	defer func() {
		gr.Close()
		spReader.Put(gr)
		bufferPool.Put(tmpBufPtr)
	}()

	for {
		n, readErr := gr.Read(tmpBuf)
		if n > 0 {
			if _, writeErr := result.Write(tmpBuf[:n]); writeErr != nil {
				return nil, writeErr
			}
		}

		if readErr == io.EOF {
			break
		}

		if readErr != nil {
			err = readErr
			break
		}
	}

	if err != nil {
		return nil, err
	}

	return result.Bytes(), nil
}

func zip(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if len(data) < CompressThreshold {
	}

	estimatedSize := int(float64(len(data)) * 0.8)
	if estimatedSize < 1024 {
		estimatedSize = 1024
	}

	w := spWriter.Get().(*gzip.Writer)

	result := bytes.NewBuffer(make([]byte, 0, estimatedSize))

	w.Reset(result)

	defer func() {
		w.Close()
		spWriter.Put(w)
	}()

	if _, err := w.Write(data); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return result.Bytes(), nil
}

func SliceByteToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func StringToSliceByte(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}
