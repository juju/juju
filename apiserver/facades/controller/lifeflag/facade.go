// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/state"
)

type Backend interface {
	state.EntityFinder
}

func NewFacade(
	modelUUID coremodel.UUID,
	backend Backend,
	watcherRegistry facade.WatcherRegistry,
	authorizer facade.Authorizer,
) (*Facade, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	expect := names.NewModelTag(modelUUID.String())
	getCanAccess := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag == expect
		}, nil
	}
	life := common.NewLifeGetter(backend, getCanAccess)
	watch := common.NewAgentEntityWatcher(backend, watcherRegistry, getCanAccess)
	return &Facade{
		LifeGetter:         life,
		AgentEntityWatcher: watch,
	}, nil
}

type Facade struct {
	*common.LifeGetter
	*common.AgentEntityWatcher
}
