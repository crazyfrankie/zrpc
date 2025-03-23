package codec

import (
	"fmt"

	"github.com/crazyfrankie/zrpc/mem"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"
)

var ErrInvalidProtoMessage = fmt.Errorf("invalid proto message")

var DefaultCodec Codec = &protobufCodec{}

type Codec interface {
	// Marshal encodes the data structure v into a stream of bytes.
	Marshal(v any) (mem.BufferSlice, error)
	// Unmarshal parses byte stream data to v
	Unmarshal(data mem.BufferSlice, v any) error
	// Name returns name of codec
	Name() string
}

type protobufCodec struct{}

// Marshal serializes structure v to []byte in protobuf format.
func (p *protobufCodec) Marshal(v any) (data mem.BufferSlice, err error) {
	vv := messageV2Of(v)
	if vv == nil {
		return nil, ErrInvalidProtoMessage
	}

	size := proto.Size(vv)
	if mem.IsLessBufferPoolThreshold(size) {
		buf, err := proto.Marshal(vv)
		if err != nil {
			return nil, err
		}
		data = append(data, mem.SliceBuffer(buf))
	} else {
		// For big data, use memory pools
		pool := mem.DefaultBufferPool()
		buf := pool.Get(size)
		_, err := proto.MarshalOptions{}.MarshalAppend((*buf)[:0], vv)
		if err != nil {
			pool.Put(buf)
			return nil, err
		}
		data = append(data, mem.NewBuffer(buf, pool))
	}

	return data, nil
}

// Unmarshal parses protobuf data to v
func (p *protobufCodec) Unmarshal(data mem.BufferSlice, v any) error {
	vv := messageV2Of(v)
	if vv == nil {
		return ErrInvalidProtoMessage
	}

	buf := data.MaterializeToBuffer(mem.DefaultBufferPool())

	return proto.Unmarshal(buf.ReadOnlyData(), vv)
}

// Name returns name of codec
func (p *protobufCodec) Name() string {
	return "protobuf"
}

func messageV2Of(v any) proto.Message {
	switch v := v.(type) {
	case protoadapt.MessageV1:
		return protoadapt.MessageV2Of(v)
	case protoadapt.MessageV2:
		return v
	}

	return nil
}
