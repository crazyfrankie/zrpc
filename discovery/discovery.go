package discovery

import "errors"

type SelectMode int

const (
	RandomSelect     SelectMode = iota // select randomly
	RoundRobinSelect                   // select using Robbin algorithm
)

var (
	ErrNoServers       = errors.New("registry discovery: no available servers")
	ErrUnsupportedMode = errors.New("registry discovery: unsupported select mode")
)

type Discovery interface {
	Get(mode SelectMode) (string, error)
	Update(servers []string) error
	GetAll() ([]string, error)
	Refresh() error // refresh from remote registry
}
