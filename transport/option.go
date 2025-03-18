package transport

import "time"

type transportOpt struct {
	connectionTimeout time.Duration
	readTimeout       time.Duration
	writeTimeout      time.Duration
}
