package cloudinit

import (
	"github.com/juju/errors"

	"github.com/juju/juju/version"
)

type Renderer interface {

	// Mkdir returns an OS specific script for creating a directory
	Mkdir(path string) []string

	// WriteFile returns a command to write data
	WriteFile(filename string, contents string, permission int) []string

	// Render renders the userdata script for a particular OS type
	Render(conf *Config) ([]byte, error)

	// FromSlash returns the result of replacing each slash ('/') character
	// in path with a separator character. Multiple slashes are replaced by
	// multiple separators.

	FromSlash(path string) string
	// PathJoin will join a path using OS specific path separator.
	// This works for selected OS instead of current OS

	PathJoin(path ...string) string
}

// NewRenderer returns a Renderer interface for selected series
func NewRenderer(series string) (Renderer, error) {
	operatingSystem, err := version.GetOSFromSeries(series)
	if err != nil {
		return nil, err
	}

	switch operatingSystem {
	case version.Windows:
		return &WindowsRenderer{}, nil
	case version.Ubuntu:
		return &UbuntuRenderer{}, nil
	default:
		return nil, errors.Errorf("No renderer could be found for %s", series)
	}
}
