// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

// Backend represents the interface required for this facade to retried entity
// information.
type Backend interface {
	state.EntityFinder
}

// NewFacade constructs a new life flag facade.
func NewFacade(backend Backend, watcherRegistry facade.WatcherRegistry, authorizer facade.Authorizer) (*Facade, error) {
	if !authorizer.AuthUnitAgent() && authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}
	getCanAccess := func(context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return authorizer.AuthOwner(tag)
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
