package protocol

import "sync/atomic"

type Compressor interface {
	Zip([]byte) ([]byte, error)
	Unzip([]byte) ([]byte, error)
}

// Compression statistics for performance monitoring
var (
	// Total number of compressed requests
	compressionRequestCount int64
	// Total number of skipped compressions (less than threshold or larger after compression)
	compressionSkippedCount int64
	// Total number of compression failures
	compressionFailureCount int64
)

func GetCompressionStats() (requests, skipped, failures int64) {
	return atomic.LoadInt64(&compressionRequestCount),
		atomic.LoadInt64(&compressionSkippedCount),
		atomic.LoadInt64(&compressionFailureCount)
}

func ResetCompressionStats() {
	atomic.StoreInt64(&compressionRequestCount, 0)
	atomic.StoreInt64(&compressionSkippedCount, 0)
	atomic.StoreInt64(&compressionFailureCount, 0)
}

// GzipCompressor implements gzip compressor.
type GzipCompressor struct{}

func (g GzipCompressor) Zip(data []byte) ([]byte, error) {
	atomic.AddInt64(&compressionRequestCount, 1)

	if len(data) < CompressThreshold {
		atomic.AddInt64(&compressionSkippedCount, 1)
		return data, nil
	}

	compressed, err := zip(data)
	if err != nil {
		atomic.AddInt64(&compressionFailureCount, 1)
		return data, nil
	}

	if len(compressed) >= len(data) {
		atomic.AddInt64(&compressionSkippedCount, 1)
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
