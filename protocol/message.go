package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"runtime"

	"go.uber.org/zap"

	"github.com/crazyfrankie/zrpc/mem"
	"github.com/crazyfrankie/zrpc/metadata"
)

var Compressors = map[CompressType]Compressor{
	None: &RawDataCompressor{},
	Gzip: &GzipCompressor{},
}

var (
	ErrMetaKVMissing         = errors.New("wrong metadata lines. some keys or values are missing")
	ErrUnsupportedCompressor = errors.New("unsupported compressor")
)

const (
	// ServiceError contains error info of service invocation
	ServiceError = "__zrpc_error__"
)

const (
	// magicNumber is used to identify that this is a zrpc request
	magicNumber byte = 0x26
)

// MessageType is message type of requests and responses.
type MessageType byte

const (
	// Request is message type of request
	Request MessageType = iota
	// Response is message type of response
	Response
)

// MessageStatusType is status of messages
type MessageStatusType byte

const (
	Normal MessageStatusType = iota
	Error
)

// CompressType defines decompression type.
type CompressType byte

const (
	// None does not compress.
	None CompressType = iota
	// Gzip uses gzip compression.
	Gzip
)

func MagicNumber() byte { return magicNumber }

type Message struct {
	*Header
	ServiceName   string
	ServiceMethod string
	Metadata      metadata.MD
	Payload       []byte
	buf           []byte
}

func NewMessage() *Message {
	header := Header([11]byte{})
	header[0] = magicNumber

	return &Message{
		Header: &header,
	}
}

// Header is the first part of Message and has fixed size.
// Format:
type Header [11]byte

// CheckMagicNumber Checks for zrpc message
func (h *Header) CheckMagicNumber() bool {
	return h[0] == magicNumber
}

// SetVersion set zrpc's version
func (h *Header) SetVersion(version byte) { h[1] = version }

// Version returns zrpc's version
func (h *Header) Version() byte {
	return h[1]
}

// SetMessageType set message type
func (h *Header) SetMessageType(mt MessageType) {
	h[2] = (h[2] &^ 0x80) | (byte(mt) << 7)
}

// GetMessageType returns message's type
func (h *Header) GetMessageType() MessageType {
	return MessageType(h[2]&0x80) >> 7
}

// SetMessageStatusType set status of message
func (h *Header) SetMessageStatusType(mst MessageStatusType) {
	h[2] = (h[2] &^ 0x40) | (byte(mst) << 6)
}

// GetMessageStatusType returns the status of message
func (h *Header) GetMessageStatusType() MessageStatusType {
	return MessageStatusType(h[2]&0x40) >> 6
}

// IsHeartBeat returns the message is heartbeat or not
func (h *Header) IsHeartBeat() bool {
	return h[2]&0x20 == 0x20
}

// SetHeartBeat sets whether the message is a heartbeat request or not
func (h *Header) SetHeartBeat(hb bool) {
	if hb {
		h[2] = h[2] | 0x20
	} else {
		h[2] = h[2] &^ 0x20
	}
}

// SetCompressType set compress type
func (h *Header) SetCompressType(ct CompressType) {
	h[2] = (h[2] &^ 0x10) | (byte(ct) << 4)
}

// GetCompressType returns compress type of message
func (h *Header) GetCompressType() CompressType {
	return CompressType(h[2]&0x10) >> 4
}

// GetSeq returns sequence number of message
func (h *Header) GetSeq() uint64 {
	return binary.BigEndian.Uint64(h[3:])
}

// SetSeq sets sequence number
func (h *Header) SetSeq(seq uint64) {
	binary.BigEndian.PutUint64(h[3:], seq)
}

// Clone clones from a message
func (m *Message) Clone() *Message {
	header := *m.Header
	header.SetCompressType(None)
	c := NewMessage()
	c.Header = &header
	c.ServiceName = m.ServiceName
	c.ServiceMethod = m.ServiceMethod

	return c
}

func (m *Message) Encode() mem.Buffer {
	// meta encode
	buf := bytes.NewBuffer(make([]byte, 0, len(m.Metadata)*64))
	encodeMetadata(m.Metadata, buf)
	meta := buf.Bytes()
	metaL := len(meta)

	sNL := len(m.ServiceName)
	sMtdL := len(m.ServiceMethod)

	// payload compress
	payload := m.Payload
	if m.GetCompressType() != None {
		compressor := Compressors[m.GetCompressType()]
		if compressor == nil {
			m.SetCompressType(None)
		} else {
			var err error
			payload, err = compressor.Zip(m.Payload)
			if err != nil {
				m.SetCompressType(None)
				payload = m.Payload
			}
		}
	}
	payloadL := len(payload)

	// total data len
	dataL := (4 + sNL) + (4 + sMtdL) + (4 + metaL) + (4 + payloadL)

	// headBuf size (header + dataLen + serviceNameLen + serviceName + methodLen + method)
	headBufSize := 11 + 4 + 4 + sNL + 4 + sMtdL

	// get a pool
	pool := mem.DefaultBufferPool()

	// total buf slice
	var bufSlice mem.BufferSlice

	headBuf := pool.Get(headBufSize)
	offset := 0

	// write header
	copy((*headBuf)[offset:offset+11], m.Header[:])
	offset += 11
	binary.BigEndian.PutUint32((*headBuf)[offset:offset+4], uint32(dataL))
	offset += 4

	// write serviceName and it's length
	binary.BigEndian.PutUint32((*headBuf)[offset:offset+4], uint32(sNL))
	offset += 4
	copy((*headBuf)[offset:offset+sNL], StringToSliceByte(m.ServiceName))
	offset += sNL

	// write serviceMethod and it's length
	binary.BigEndian.PutUint32((*headBuf)[offset:offset+4], uint32(sMtdL))
	offset += 4
	copy((*headBuf)[offset:offset+sMtdL], StringToSliceByte(m.ServiceMethod))
	offset += sMtdL

	*headBuf = (*headBuf)[:offset]
	bufSlice = append(bufSlice, mem.NewBuffer(headBuf, pool))

	// write meta
	metaBuf := pool.Get(4 + metaL)
	binary.BigEndian.PutUint32(*metaBuf, uint32(metaL))
	copy((*metaBuf)[4:], meta)
	bufSlice = append(bufSlice, mem.NewBuffer(metaBuf, pool))

	// write payload
	payloadBuf := pool.Get(4 + payloadL)
	binary.BigEndian.PutUint32(*payloadBuf, uint32(payloadL))
	copy((*payloadBuf)[4:], payload)
	bufSlice = append(bufSlice, mem.NewBuffer(payloadBuf, pool))

	return bufSlice.MaterializeToBuffer(pool)
}

func encodeMetadata(md metadata.MD, buf *bytes.Buffer) {
	if len(md) == 0 {
		return
	}

	d := make([]byte, 4)

	for k, values := range md {
		binary.BigEndian.PutUint32(d, uint32(len(k)))
		buf.Write(d)
		buf.Write(StringToSliceByte(k))

		buf.Write([]byte{byte(len(values))})

		for _, v := range values {
			binary.BigEndian.PutUint32(d, uint32(len(v)))
			buf.Write(d)
			buf.Write(StringToSliceByte(v))
		}
	}
}

func (m *Message) Decode(r io.Reader, maxLength int) error {
	defer func() {
		if err := recover(); err != nil {
			var errStack = make([]byte, 1024)
			n := runtime.Stack(errStack, true)
			zap.L().Error(fmt.Sprintf("panic in message decode: %v, stack: %s", err, errStack[:n]))
		}
	}()

	// check magic number
	if _, err := io.ReadFull(r, m.Header[:1]); err != nil {
		return err
	}
	if !m.Header.CheckMagicNumber() {
		return fmt.Errorf("wrong magic number: %v", m.Header[0])
	}
	// read the remaining header
	if _, err := io.ReadFull(r, m.Header[1:]); err != nil {
		return err
	}

	// total message body
	lenData := make([]byte, 4)
	if _, err := io.ReadFull(r, lenData); err != nil {
		return err
	}
	l := binary.BigEndian.Uint32(lenData)

	if maxLength > 0 && maxLength < int(l) {
		return fmt.Errorf("the max receive message length is %d, but receive %d", maxLength, l)
	}

	// use mem.BufferPool to read the remaining data
	pool := mem.DefaultBufferPool()
	restBuf := pool.Get(int(l))

	if _, err := io.ReadFull(r, *restBuf); err != nil {
		pool.Put(restBuf)
		return err
	}

	dataBuf := mem.NewBuffer(restBuf, pool)
	defer dataBuf.Free()

	data := dataBuf.ReadOnlyData()
	offset := 0

	// parse service name
	if offset+4 > len(data) {
		return errors.New("invalid message format: insufficient data for service name length")
	}
	svcNameLen := binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4

	if offset+int(svcNameLen) > len(data) {
		return errors.New("invalid message format: insufficient data for service name")
	}
	m.ServiceName = SliceByteToString(data[offset : offset+int(svcNameLen)])
	offset += int(svcNameLen)

	// parse method name
	if offset+4 > len(data) {
		return errors.New("invalid message format: insufficient data for service method length")
	}
	methodLen := binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4

	if offset+int(methodLen) > len(data) {
		return errors.New("invalid message format: insufficient data for service method")
	}
	m.ServiceMethod = SliceByteToString(data[offset : offset+int(methodLen)])
	offset += int(methodLen)

	// parse metadata
	if offset+4 > len(data) {
		return errors.New("invalid message format: insufficient data for metadata length")
	}
	metaLen := binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4

	if metaLen > 0 {
		if offset+int(metaLen) > len(data) {
			return errors.New("invalid message format: insufficient data for metadata")
		}
		var err error
		m.Metadata, err = decodeMetadata(metaLen, data[offset:offset+int(metaLen)])
		if err != nil {
			return err
		}
		offset += int(metaLen)
	}

	// parse payload
	if offset+4 > len(data) {
		return errors.New("invalid message format: insufficient data for payload length")
	}
	payloadLen := binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4

	if offset+int(payloadLen) > len(data) {
		return errors.New("invalid message format: insufficient data for payload")
	}
	m.Payload = make([]byte, payloadLen)
	copy(m.Payload, data[offset:offset+int(payloadLen)])

	if m.GetCompressType() != None {
		compressor := Compressors[m.GetCompressType()]
		if compressor == nil {
			return ErrUnsupportedCompressor
		}
		var err error
		m.Payload, err = compressor.Unzip(m.Payload)
		if err != nil {
			return err
		}
	}

	return nil
}

// decodeMetadata reads out the data and maps it to the MD
func decodeMetadata(l uint32, data []byte) (metadata.MD, error) {
	m := make(metadata.MD)
	n := uint32(0)
	for n < l {
		// parse one key and value
		// key
		sl := binary.BigEndian.Uint32(data[n : n+4])
		n = n + 4
		if n+sl > l-4 {
			return nil, ErrMetaKVMissing
		}
		k := string(data[n : n+sl])
		n = n + sl

		// number of values
		numValues := data[n]
		n = n + 1
		values := make([]string, numValues)

		// read each value
		for i := uint8(0); i < numValues; i++ {
			sl = binary.BigEndian.Uint32(data[n : n+4])
			n = n + 4
			if n+sl > l {
				return nil, ErrMetaKVMissing
			}
			v := string(data[n : n+sl])
			n = n + sl
			values[i] = v
		}

		m[k] = values
	}

	return m, nil
}
