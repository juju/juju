package charm

import "os"

// The Charm interface is implemented by any type that
// may be handled as a charm.
type Charm interface {
	Meta() *Meta
	Config() *Config
	Revision() int
}

// Read reads a Charm from path, which can point to either a charm bundle or a
// charm directory.
func Read(path string) (Charm, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return ReadDir(path)
	}
	return ReadBundle(path)
}
