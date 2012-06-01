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

// Resolve takes a (potentially vaguely-specified) charm name, the path to the
// local charm repository, and the environment's default Ubuntu series, and
// assembles them into a charm URL and a repository which is likely to contain
// a charm matching that URL.
func Resolve(name, repoPath, defaultSeries string) (repo Repository, curl *URL, err error) {
	if curl, err = InferURL(name, defaultSeries); err != nil {
		return
	}
	switch curl.Schema {
	case "cs":
		repo = Store()
	case "local":
		repo = &LocalRepository{repoPath}
	default:
		panic(fmt.Errorf("unknown schema for charm URL %q", curl))
	}
	return
}
