package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"runtime"

	"go.uber.org/zap"

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

func (m *Message) Decode(r io.Reader, maxLength int) error {
	defer func() {
		if err := recover(); err != nil {
			var errStack = make([]byte, 1024)
			n := runtime.Stack(errStack, true)
			zap.L().Error(fmt.Sprintf("panic in message decode: %v, stack: %s", err, errStack[:n]))
		}
	}()

	// parse header
	_, err := io.ReadFull(r, m.Header[:1])
	if err != nil {
		return err
	}
	if !m.Header.CheckMagicNumber() {
		return fmt.Errorf("wrong magic number: %v", m.Header[0])
	}

	_, err = io.ReadFull(r, m.Header[1:])
	if err != nil {
		return err
	}

	// total message body
	lenData := make([]byte, 4)
	_, err = io.ReadFull(r, lenData)
	if err != nil {
		return err
	}
	l := binary.BigEndian.Uint32(lenData)

	if maxLength > 0 && maxLength < int(l) {
		return fmt.Errorf("the max receive message length is %d, but receive %d", maxLength, l)
	}

	totalLength := int(l)
	if cap(m.buf) >= totalLength {
		m.buf = m.buf[:totalLength]
	} else {
		m.buf = make([]byte, totalLength)
	}
	// In fact, we don't need the local variable buf,
	// because m.buf itself is already a buffer of
	// all the data excluding the header.
	// The reason for using buf again is to
	// make it refer to the same underlying array as m.buf,
	// and to cut it without modifying the data already read,
	// also more readable
	buf := m.buf
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return err
	}

	n := 0
	// parse serviceName
	l = binary.BigEndian.Uint32(buf[n:4])
	n += 4
	nEnd := n + int(l)
	m.ServiceName = SliceByteToString(buf[n:nEnd])
	n = nEnd

	// parse serviceMethod
	l = binary.BigEndian.Uint32(buf[n : n+4])
	n += 4
	nEnd = n + int(l)
	m.ServiceMethod = SliceByteToString(buf[n:nEnd])
	n = nEnd

	// parse metadata
	l = binary.BigEndian.Uint32(buf[n : n+4])
	n += 4
	nEnd = n + int(l)

	if l > 0 {
		m.Metadata, err = decodeMetadata(l, buf[n:nEnd])
		if err != nil {
			return err
		}
	}
	n = nEnd

	// parse payload
	l = binary.BigEndian.Uint32(buf[n : n+4])
	n += 4
	m.Payload = buf[n:]

	if m.GetCompressType() != None {
		compressor := Compressors[m.GetCompressType()]
		if compressor == nil {
			return ErrUnsupportedCompressor
		}
		m.Payload, err = compressor.Unzip(m.Payload)
		if err != nil {
			return err
		}
	}

	return err
}

// decodeMetadata reads out the data and maps it to the MD
func decodeMetadata(l uint32, data []byte) (metadata.MD, error) {
	m := make(map[string]string, 10)
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

		// value
		sl = binary.BigEndian.Uint32(data[n : n+4])
		n = n + 4
		if n+sl > l {
			return nil, ErrMetaKVMissing
		}
		v := string(data[n : n+sl])
		n = n + sl
		m[k] = v
	}

	return metadata.New(m), nil
}
