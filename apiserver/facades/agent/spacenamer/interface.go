// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spacenamer

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/state"
)

type SpaceNamerAPI struct {
	st          SpaceNamerState
	model       ModelCache
	resources   facade.Resources
	authorizer  facade.Authorizer
	getAuthFunc common.GetAuthFunc
}

// ModelCache represents point of use methods from the cache
// model
type ModelCache interface {
	WatchConfig(keys ...string) *cache.ConfigWatcher
}

type SpaceNamerState interface {
	state.EntityFinder

	SpaceByID(id string) (Space, error)
	Model() (Model, error)
}

type Space interface {
	Name() string
	SetName(string) error
}

type Model interface {
	Config() (Config, error)
}

type Config interface {
	DefaultSpace() string
}
