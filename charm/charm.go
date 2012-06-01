package charm

import (
	"errors"
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

// InferRepository returns a charm repository and URL inferred from the provided
// parameters. charmAlias may hold an exact charm URL, or an alias in a
// format supported by InferURL.
func InferRepository(charmAlias, defaultSeries, localRepoPath string) (repo Repository, curl *URL, err error) {
	if curl, err = InferURL(charmAlias, defaultSeries); err != nil {
		return
	}
	switch curl.Schema {
	case "cs":
		repo = Store()
	case "local":
		if localRepoPath == "" {
			return nil, nil, errors.New("path to local repository not specified")
		}
		repo = &LocalRepository{localRepoPath}
	default:
		panic(fmt.Errorf("unknown schema for charm URL %q", curl))
	}
	return
}
