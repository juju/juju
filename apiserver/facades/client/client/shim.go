// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import "github.com/juju/juju/core/cache"

type controllerCacheShim struct {
	*cache.Controller
}

type modelCacheShim struct {
	*cache.Model
}

func (s modelCacheShim) Branches() ([]ModelCacheBranch, error) {
	b, err := s.Model.Branches()
	if err != nil {
		return nil, err
	}
	branches := make([]ModelCacheBranch, len(b))
	for k, v := range b {
		branches[k] = &modelCacheBranch{Branch: v}
	}
	return branches, nil
}

type modelCacheBranch struct {
	cache.Branch
}
