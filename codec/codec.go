package codec

import (
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"

	"github.com/crazyfrankie/zrpc/mem"
)

var ErrInvalidProtoMessage = fmt.Errorf("invalid proto message")

var DefaultCodec Codec = &protobufCodec{}

var smallBufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 0, 1024) // 1KB 预分配
		return &buf
	},
}

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
		bufPtr := smallBufferPool.Get().(*[]byte)
		buf := *bufPtr
		buf = buf[:0]

		buf, err = proto.MarshalOptions{}.MarshalAppend(buf, vv)
		if err != nil {
			smallBufferPool.Put(bufPtr)
			return nil, err
		}

		result := make([]byte, len(buf))
		copy(result, buf)

		*bufPtr = buf[:0]
		smallBufferPool.Put(bufPtr)

		data = append(data, mem.SliceBuffer(result))
	} else {
		pool := mem.DefaultBufferPool()
		buf := pool.Get(size)
		if _, err := (proto.MarshalOptions{}).MarshalAppend((*buf)[:0], vv); err != nil {
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
	opts := proto.UnmarshalOptions{
		DiscardUnknown: true,
		Merge:          false,
	}

	return opts.Unmarshal(buf.ReadOnlyData(), vv)
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
