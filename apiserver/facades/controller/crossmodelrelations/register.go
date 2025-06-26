// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CrossModelRelations", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		api, err := makeStateCrossModelRelationsAPI(stdCtx, ctx) // Removes remote spaces
		if err != nil {
			return nil, fmt.Errorf("creating CrossModelRelations facade: %w", err)
		}
		return api, nil
	}, reflect.TypeOf((*CrossModelRelationsAPIv3)(nil)))
}

// makeStateCrossModelRelationsAPI creates a new server-side CrossModelRelations API facade
// backed by global state.
func makeStateCrossModelRelationsAPI(stdCtx context.Context, ctx facade.ModelContext) (*CrossModelRelationsAPIv3, error) {
	return NewCrossModelRelationsAPI()
}
