package protocol

type Compressor interface {
	Zip([]byte) ([]byte, error)
	Unzip([]byte) ([]byte, error)
}

// GzipCompressor implements gzip compressor.
type GzipCompressor struct{}

func (g GzipCompressor) Zip(data []byte) ([]byte, error) {
	if len(data) < CompressThreshold {
		return data, nil
	}

	compressed, err := zip(data)
	if err != nil {
		return data, nil
	}

	if len(compressed) >= len(data) {
		return data, nil
	}

	return compressed, nil
}

func (g GzipCompressor) Unzip(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	return unzip(data)
}

type RawDataCompressor struct{}

func (c RawDataCompressor) Zip(data []byte) ([]byte, error) {
	return data, nil
}

func (c RawDataCompressor) Unzip(data []byte) ([]byte, error) {
	return data, nil
}
