// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CrossModelRelations", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStateCrossModelRelationsAPI(ctx) // Removes remote spaces
	}, reflect.TypeOf((*CrossModelRelationsAPIv3)(nil)))
}

// newStateCrossModelRelationsAPI creates a new server-side CrossModelRelations API facade
// backed by global state.
func newStateCrossModelRelationsAPI(ctx facade.ModelContext) (*CrossModelRelationsAPIv3, error) {
	authCtxt := ctx.Resources().Get("offerAccessAuthContext").(*common.ValueResource).Value
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, err
	}

	modelLogger, err := ctx.ModelLogger(model.UUID(), model.Name(), model.Owner().Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewCrossModelRelationsAPI(
		stateShim{
			st:      st,
			Backend: commoncrossmodel.GetBackend(st),
		},
		firewall.StateShim(st, model),
		ctx.Resources(), ctx.Auth(),
		authCtxt.(*commoncrossmodel.AuthContext),
		firewall.WatchEgressAddressesForRelations,
		watchRelationLifeSuspendedStatus,
		watchOfferStatus,
		watchConsumedSecrets,
		ctx.Logger().ChildWithTags("crossmodelrelations", corelogger.CMR),
		common.NewStatusHistoryRecorder(ctx.MachineTag().String(), modelLogger),
	)
}
