package codec

import (
	"fmt"

	"google.golang.org/protobuf/proto"
)

var DefaultCodec Codec = &protobufCodec{}

type Codec interface {
	// Marshal encodes the data structure v into a stream of bytes.
	Marshal(v any) (BufferSlice, error)
	// Unmarshal parses byte stream data to v
	Unmarshal(data BufferSlice, v any) error
	// Name returns name of codec
	Name() string
}

type protobufCodec struct{}

// Marshal serializes structure v to []byte in protobuf format.
func (p *protobufCodec) Marshal(v any) (BufferSlice, error) {
	if msg, ok := v.(proto.Message); ok {
		buf := bufferPool.Get().(*BufferSlice)
		data, err := proto.Marshal(msg)
		if err != nil {
			bufferPool.Put(buf)
			return BufferSlice{}, err
		}
		buf.Data = append(buf.Data[:0], data...)
		return *buf, nil
	}
	return BufferSlice{}, ErrInvalidProtoMessage
}

// Unmarshal parses protobuf data to v
func (p *protobufCodec) Unmarshal(data BufferSlice, v any) error {
	if msg, ok := v.(proto.Message); ok {
		return proto.Unmarshal(data.Data, msg)
	}
	return ErrInvalidProtoMessage
}

// Name returns name of codec
func (p *protobufCodec) Name() string {
	return "protobuf"
}

var ErrInvalidProtoMessage = fmt.Errorf("invalid proto message")
