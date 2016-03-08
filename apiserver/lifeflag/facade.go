// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

type Backend interface {
	ModelUUID() string
	state.EntityFinder
}

func NewFacade(backend Backend, resources *common.Resources, authorizer common.Authorizer) (*Facade, error) {
	if !authorizer.AuthModelManager() {
		return nil, common.ErrPerm
	}
	expect := names.NewModelTag(backend.ModelUUID())
	access := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag == expect
		}, nil
	}
	life := common.NewLifeGetter(backend, access)
	watch := common.NewAgentEntityWatcher(backend, resources, access)
	return &Facade{
		LifeGetter:         life,
		AgentEntityWatcher: watch,
	}, nil
}

type Facade struct {
	*common.LifeGetter
	*common.AgentEntityWatcher
}
