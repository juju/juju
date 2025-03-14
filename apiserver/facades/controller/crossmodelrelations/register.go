// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
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
	authCtxt := ctx.Resources().Get("offerAccessAuthContext").(*common.ValueResource).Value
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, err
	}

	modelInfo, err := ctx.DomainServices().ModelInfo().GetModelInfo(stdCtx)
	if err != nil {
		return nil, fmt.Errorf("retrieving model info: %w", err)
	}

	return NewCrossModelRelationsAPI(
		modelInfo.UUID,
		stateShim{
			st:      st,
			Backend: commoncrossmodel.GetBackend(st),
		},
		firewall.StateShim(st, m),
		ctx.Resources(), ctx.Auth(),
		authCtxt.(*commoncrossmodel.AuthContext),
		ctx.DomainServices().Secret(),
		ctx.DomainServices().Config(),
		ctx.DomainServices().Application(),
		ctx.DomainServices().Status(),
		firewall.WatchEgressAddressesForRelations,
		watchRelationLifeSuspendedStatus,
		watchOfferStatus,
		watchConsumedSecrets,
		ctx.Logger().Child("crossmodelrelations", corelogger.CMR),
	)
}
