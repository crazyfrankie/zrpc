package protocol

import (
	"bytes"
	"compress/gzip"
	"io"
	"sync"
	"unsafe"
)

var (
	spWriter sync.Pool
	spReader sync.Pool
	spBuffer sync.Pool
)

func init() {
	spWriter = sync.Pool{New: func() interface{} {
		return gzip.NewWriter(nil)
	}}
	spReader = sync.Pool{New: func() interface{} {
		return new(gzip.Reader)
	}}
	spBuffer = sync.Pool{New: func() interface{} {
		return bytes.NewBuffer(nil)
	}}
}

// unzip unzips data.
func unzip(data []byte) ([]byte, error) {
	buf := bytes.NewBuffer(data)

	gr := spReader.Get().(*gzip.Reader)
	defer func() {
		spReader.Put(gr)
	}()
	err := gr.Reset(buf)
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	data, err = io.ReadAll(gr)
	if err != nil {
		return nil, err
	}
	return data, err
}

// zip zips data.
func zip(data []byte) ([]byte, error) {
	buf := spBuffer.Get().(*bytes.Buffer)
	w := spWriter.Get().(*gzip.Writer)
	w.Reset(buf)

	defer func() {
		buf.Reset()
		spBuffer.Put(buf)
		w.Close()
		spWriter.Put(w)
	}()
	_, err := w.Write(data)
	if err != nil {
		return nil, err
	}
	err = w.Flush()
	if err != nil {
		return nil, err
	}
	err = w.Close()
	if err != nil {
		return nil, err
	}
	dec := buf.Bytes()
	return dec, nil
}

func SliceByteToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func StringToSliceByte(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}
