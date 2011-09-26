package charm

import (
	"fmt"
	"os"
)

// The Charm interface is implemented by any type that
// may be handled as a charm.
type Charm interface {
	Meta() *Meta
	Config() *Config
}

func errorf(format string, args ...interface{}) os.Error {
	return os.NewError(fmt.Sprintf(format, args...))
}
