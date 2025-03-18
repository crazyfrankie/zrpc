package protocol

type Compressor interface {
	Zip([]byte) ([]byte, error)
	Unzip([]byte) ([]byte, error)
}

// GzipCompressor implements gzip compressor.
type GzipCompressor struct {
}

func (g GzipCompressor) Zip(data []byte) ([]byte, error) {
	return zip(data)
}

func (g GzipCompressor) Unzip(data []byte) ([]byte, error) {
	return unzip(data)
}

type RawDataCompressor struct {
}

func (c RawDataCompressor) Zip(data []byte) ([]byte, error) {
	return data, nil
}

func (c RawDataCompressor) Unzip(data []byte) ([]byte, error) {
	return data, nil
}
