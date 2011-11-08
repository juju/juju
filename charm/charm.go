package charm

import (
	"errors"
	"fmt"
)

// The Charm interface is implemented by any type that
// may be handled as a charm.
type Charm interface {
	Meta() *Meta
	Config() *Config
}

func errorf(format string, args ...interface{}) error {
	return errors.New(fmt.Sprintf(format, args...))
}
