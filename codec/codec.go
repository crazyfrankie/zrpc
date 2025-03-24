package codec

import (
	"fmt"
	"sync"

	"github.com/crazyfrankie/zrpc/mem"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"
)

var ErrInvalidProtoMessage = fmt.Errorf("invalid proto message")

var DefaultCodec Codec = &protobufCodec{}

// 预分配缓冲区大小，避免频繁创建和释放小缓冲区
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

	// 预估大小更准确，避免内存重新分配
	size := proto.Size(vv)

	// 针对小消息使用预分配缓冲区
	if size < 1024 {
		bufPtr := smallBufferPool.Get().(*[]byte)
		buf := *bufPtr
		buf = buf[:0] // 重置但保持容量

		// 使用 MarshalAppend 而不是 Marshal 避免额外内存分配
		buf, err = proto.MarshalOptions{}.MarshalAppend(buf, vv)
		if err != nil {
			smallBufferPool.Put(bufPtr)
			return nil, err
		}

		// 创建副本，以便可以将原始缓冲区返回到池
		result := make([]byte, len(buf))
		copy(result, buf)

		// 将缓冲区放回池
		*bufPtr = buf[:0]
		smallBufferPool.Put(bufPtr)

		return append(data, mem.SliceBuffer(result)), nil
	}

	// 对于大消息，使用内存池
	pool := mem.DefaultBufferPool()
	buf := pool.Get(size)

	// 直接将消息编码到预分配的缓冲区
	marshaledBuf, err := proto.MarshalOptions{}.MarshalAppend((*buf)[:0], vv)
	if err != nil {
		pool.Put(buf)
		return nil, err
	}

	// 确保我们使用准确的缓冲区大小
	*buf = marshaledBuf
	data = append(data, mem.NewBuffer(buf, pool))

	return data, nil
}

// Unmarshal parses protobuf data to v
func (p *protobufCodec) Unmarshal(data mem.BufferSlice, v any) error {
	vv := messageV2Of(v)
	if vv == nil {
		return ErrInvalidProtoMessage
	}

	buf := data.MaterializeToBuffer(mem.DefaultBufferPool())
	// 使用 UnmarshalOptions 可以提供更好的控制
	opts := proto.UnmarshalOptions{
		// 如果消息已经被部分解析，可以决定是否丢弃这些未知字段
		DiscardUnknown: true,
		// 对于大消息禁用合并，减少内存分配
		Merge: false,
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
