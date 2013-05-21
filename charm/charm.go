// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

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

// InferRepository returns a charm repository inferred from
// the provided URL. Local URLs will use the provided path.
func InferRepository(curl *URL, localRepoPath string) (repo Repository, err error) {
	switch curl.Schema {
	case "cs":
		repo = Store
	case "local":
		if localRepoPath == "" {
			return nil, errors.New("path to local repository not specified")
		}
		repo = &LocalRepository{localRepoPath}
	default:
		return nil, fmt.Errorf("unknown schema for charm URL %q", curl)
	}
	return
}
