package protocol

import (
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
	return binary.LittleEndian.Uint64(h[3:])
}

// SetSeq sets sequence number
func (h *Header) SetSeq(seq uint64) {
	binary.LittleEndian.PutUint64(h[3:], seq)
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
	var metaL int
	if len(m.Metadata) > 0 {
		for k, vs := range m.Metadata {
			metaL += 4 + len(k) + 1
			for _, v := range vs {
				metaL += 4 + len(v)
			}
		}
	}

	sNL := len(m.ServiceName)
	sMtdL := len(m.ServiceMethod)

	var payload []byte
	payloadSize := len(m.Payload)

	shouldCompress := m.GetCompressType() != None && payloadSize >= CompressThreshold

	if shouldCompress {
		compressor := Compressors[m.GetCompressType()]
		if compressor == nil {
			m.SetCompressType(None)
			payload = m.Payload
		} else {
			var err error
			payload, err = compressor.Zip(m.Payload)
			if err != nil || len(payload) >= payloadSize {
				m.SetCompressType(None)
				payload = m.Payload
			}
		}
	} else {
		if m.GetCompressType() != None && payloadSize < CompressThreshold {
			m.SetCompressType(None)
		}
		payload = m.Payload
	}

	payloadL := len(payload)

	// total data len
	dataL := (4 + sNL) + (4 + sMtdL) + (4 + metaL) + (4 + payloadL)

	totalL := 11 + 4 + dataL

	pool := mem.DefaultBufferPool()
	buf := pool.Get(totalL)
	offset := 0

	copy((*buf)[offset:offset+11], m.Header[:])
	offset += 11

	binary.LittleEndian.PutUint32((*buf)[offset:offset+4], uint32(dataL))
	offset += 4

	// write service name
	binary.LittleEndian.PutUint32((*buf)[offset:offset+4], uint32(sNL))
	offset += 4
	copy((*buf)[offset:offset+sNL], StringToSliceByte(m.ServiceName))
	offset += sNL

	// write method name
	binary.LittleEndian.PutUint32((*buf)[offset:offset+4], uint32(sMtdL))
	offset += 4
	copy((*buf)[offset:offset+sMtdL], StringToSliceByte(m.ServiceMethod))
	offset += sMtdL

	// write metadata
	binary.LittleEndian.PutUint32((*buf)[offset:offset+4], uint32(metaL))
	offset += 4
	if metaL > 0 {
		for k, vs := range m.Metadata {
			binary.LittleEndian.PutUint32((*buf)[offset:offset+4], uint32(len(k)))
			offset += 4
			copy((*buf)[offset:offset+len(k)], StringToSliceByte(k))
			offset += len(k)
			(*buf)[offset] = byte(len(vs))
			offset++
			for _, v := range vs {
				binary.LittleEndian.PutUint32((*buf)[offset:offset+4], uint32(len(v)))
				offset += 4
				copy((*buf)[offset:offset+len(v)], StringToSliceByte(v))
				offset += len(v)
			}
		}
	}

	// write payload
	binary.LittleEndian.PutUint32((*buf)[offset:offset+4], uint32(payloadL))
	offset += 4
	if payloadL > 0 {
		copy((*buf)[offset:offset+payloadL], payload)
	}

	return mem.NewBuffer(buf, pool)
}

func (m *Message) Decode(r io.Reader, maxLength int) error {
	defer func() {
		if err := recover(); err != nil {
			var errStack = make([]byte, 1024)
			n := runtime.Stack(errStack, true)
			zap.L().Error(fmt.Sprintf("panic in message decode: %v, stack: %s", err, errStack[:n]))
		}
	}()

	// read header
	headerBuf := make([]byte, 11)
	if _, err := io.ReadFull(r, headerBuf); err != nil {
		return err
	}

	copy(m.Header[:], headerBuf)

	if !m.Header.CheckMagicNumber() {
		return fmt.Errorf("wrong magic number: %v", m.Header[0])
	}

	// read the total length of the data
	lenData := make([]byte, 4)
	if _, err := io.ReadFull(r, lenData); err != nil {
		return err
	}
	dataLen := binary.LittleEndian.Uint32(lenData)

	// check if the data length exceeds the limit
	if maxLength > 0 && maxLength < int(dataLen) {
		return fmt.Errorf("the max receive message length is %d, but receive %d", maxLength, dataLen)
	}

	// use stack memory directly under 1KB
	if dataLen <= 1024 {
		buf := make([]byte, dataLen)
		if _, err := io.ReadFull(r, buf); err != nil {
			return err
		}

		return m.decodeMessageData(buf)
	}

	pool := mem.DefaultBufferPool()
	restBuf := pool.Get(int(dataLen))

	if _, err := io.ReadFull(r, *restBuf); err != nil {
		pool.Put(restBuf)
		return err
	}

	dataBuf := mem.NewBuffer(restBuf, pool)
	defer dataBuf.Free()

	return m.decodeMessageData(dataBuf.ReadOnlyData())
}

func (m *Message) decodeMessageData(data []byte) error {
	offset := 0
	dataLen := len(data)

	// parse service name
	if offset+4 > dataLen {
		return errors.New("invalid message format: insufficient data for service name length")
	}
	svcNameLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	if offset+int(svcNameLen) > dataLen {
		return errors.New("invalid message format: insufficient data for service name")
	}
	m.ServiceName = SliceByteToString(data[offset : offset+int(svcNameLen)])
	offset += int(svcNameLen)

	// parse service method
	if offset+4 > dataLen {
		return errors.New("invalid message format: insufficient data for service method length")
	}
	methodLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	if offset+int(methodLen) > dataLen {
		return errors.New("invalid message format: insufficient data for service method")
	}
	m.ServiceMethod = SliceByteToString(data[offset : offset+int(methodLen)])
	offset += int(methodLen)

	// parse metadata
	if offset+4 > dataLen {
		return errors.New("invalid message format: insufficient data for metadata length")
	}
	metaLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	if m.Metadata == nil {
		m.Metadata = metadata.MD{}
	} else {
		for k := range m.Metadata {
			delete(m.Metadata, k)
		}
	}
	if metaLen > 0 {
		if offset+int(metaLen) > dataLen {
			return errors.New("invalid message format: insufficient data for metadata")
		}

		metaEnd := offset + int(metaLen)
		for offset < metaEnd {
			if offset+4 > metaEnd {
				return errors.New("invalid metadata format: insufficient data for key length")
			}
			keyLen := binary.LittleEndian.Uint32(data[offset : offset+4])
			offset += 4

			if offset+int(keyLen) > metaEnd {
				return errors.New("invalid metadata format: insufficient data for key")
			}
			key := SliceByteToString(data[offset : offset+int(keyLen)])
			offset += int(keyLen)

			if offset >= metaEnd {
				return errors.New("invalid metadata format: insufficient data for value count")
			}
			valueCount := int(data[offset])
			offset++

			for i := 0; i < valueCount; i++ {
				if offset+4 > metaEnd {
					return errors.New("invalid metadata format: insufficient data for value length")
				}
				valueLen := binary.LittleEndian.Uint32(data[offset : offset+4])
				offset += 4

				if offset+int(valueLen) > metaEnd {
					return errors.New("invalid metadata format: insufficient data for value")
				}
				value := SliceByteToString(data[offset : offset+int(valueLen)])
				offset += int(valueLen)

				m.Metadata.Append(key, value)
			}
		}
	}

	// parse payload
	if offset+4 > dataLen {
		return errors.New("invalid message format: insufficient data for payload length")
	}
	payloadLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	if offset+int(payloadLen) > dataLen {
		return errors.New("invalid message format: insufficient data for payload")
	}

	if payloadLen > 0 {
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
	} else {
		m.Payload = nil
	}

	return nil
}
