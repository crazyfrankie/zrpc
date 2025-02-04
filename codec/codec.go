package codec

import "io"

type Header struct {
	Seq         uint64
	ServiceName string
	Err         string
}

type Codec interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header, interface{}) error
}

type NewCodecFunc func(io.ReadWriteCloser) Codec

type Typ string

var (
	GobType  Typ = "application/gob"
	JSONType Typ = "application/json"
)

var NewCodecFuncMap = map[Typ]NewCodecFunc{
	GobType:  NewGobCodec,
	JSONType: NewJSONCodec,
}
