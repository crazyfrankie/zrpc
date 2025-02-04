package codec

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
)

type JSONCodec struct {
	conn io.ReadWriteCloser
	buf  *bufio.Writer
	dec  *json.Decoder
	enc  *json.Encoder
}

var _ Codec = (*JSONCodec)(nil)

func NewJSONCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)

	return &JSONCodec{
		conn: conn,
		buf:  buf,
		dec:  json.NewDecoder(conn),
		enc:  json.NewEncoder(buf),
	}
}

func (j *JSONCodec) Close() error {
	return j.conn.Close()
}

func (j *JSONCodec) ReadHeader(h *Header) error {
	return j.dec.Decode(h)
}

func (j *JSONCodec) ReadBody(body interface{}) error {
	return j.dec.Decode(body)
}

func (j *JSONCodec) Write(h *Header, body interface{}) (err error) {
	defer func() {
		err = j.buf.Flush()
		if err != nil {
			_ = j.conn.Close()
		}
	}()
	if err := j.enc.Encode(h); err != nil {
		log.Println("rpc codec: json error encoding header:", err)
		return err
	}
	if err := j.enc.Encode(body); err != nil {
		log.Println("rpc codec: json error encoding body:", err)
		return err
	}

	return nil
}
