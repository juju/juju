// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

//go:generate mockgen -package mocks -destination mocks/cache.go github.com/juju/juju/apiserver/facades/client/client ModelCache,ModelCacheBranch

// ModelCache represents point of use methods from the cache
// model.
type ModelCache interface {
	Branches() ([]ModelCacheBranch, error)
}

// ModelCache represents point of use methods from a cache
// branch.
type ModelCacheBranch interface {
	AssignedUnits() map[string][]string
	Created() int64
	Name() string
}
