package codec

import (
	"sync"

	"github.com/crazyfrankie/zrpc/protocol"
)

// BufferSlice wraps a []byte representing the buffer used to store the data
type BufferSlice struct {
	Data []byte
}

// bufferPool is a memory pool used to reuse BufferSlice
var bufferPool = sync.Pool{
	New: func() interface{} { return &BufferSlice{Data: make([]byte, 0, 4096)} },
}

// ToBytes returns raw byte slice
func (bs *BufferSlice) ToBytes() []byte {
	return bs.Data
}

// GetBufferSliceFromRequest extracts the payload from the request and converts it to a BufferSlice
func GetBufferSliceFromRequest(msg *protocol.Message) BufferSlice {
	buf := bufferPool.Get().(*BufferSlice)
	if cap(buf.Data) < len(msg.Payload) {
		buf.Data = make([]byte, len(msg.Payload))
	} else {
		buf.Data = buf.Data[:len(msg.Payload)]
	}
	copy(buf.Data, msg.Payload)
	return *buf
}

func PutBufferSlice(buf *BufferSlice) {
	buf.Data = buf.Data[:0]
	bufferPool.Put(buf)
}
