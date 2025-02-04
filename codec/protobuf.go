package codec

//type ProtoCodec struct {
//	conn io.ReadWriteCloser
//}
//
//var _ Codec = (*ProtoCodec)(nil)
//
//func NewProtoCodec(conn io.ReadWriteCloser) Codec {
//	return &ProtoCodec{conn: conn}
//}
//
//func (p ProtoCodec) Close() error {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (p ProtoCodec) ReadHeader(h *Header) error {
//	// 读取字节流
//	buf := make([]byte, 1024) // 你可以根据实际情况调整缓冲区大小
//	n, err := p.conn.Read(buf)
//	if err != nil && err != io.EOF {
//		log.Println("rpc codec: protobuf error reading header:", err)
//		return err
//	}
//
//	// 解码 header
//	err = proto.Unmarshal(buf[:n], h) // 直接使用 Unmarshal 解码
//	if err != nil {
//		log.Println("rpc codec: protobuf error decoding header:", err)
//		return err
//	}
//
//	return nil
//}
//
//func (p ProtoCodec) ReadBody(i interface{}) error {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (p ProtoCodec) Write(header *Header, i interface{}) error {
//	//TODO implement me
//	panic("implement me")
//}
