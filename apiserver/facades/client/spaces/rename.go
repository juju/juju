// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/settings"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// RenameSpace describes a space that can be renamed.
type RenameSpace interface {
	Refresh() error
	Id() string
	Name() string
	RenameSpaceOps(toName string) []txn.Op
}

// RenameSpaceState describes state operations required
// to execute the renameSpace operation.
type RenameSpaceState interface {
	// ConstraintsBySpaceName returns all the constraints
	// that refer to the input space name.
	ConstraintsBySpaceName(spaceName string) ([]Constraints, error)
}

// Settings describes methods for interacting with settings to apply
// space-based configuration deltas.
type Settings interface {
	DeltaOps(key string, delta settings.ItemChanges) ([]txn.Op, error)
}

type renameSpaceState struct {
	*state.State
}

func (st renameSpaceState) ConstraintsBySpaceName(spaceName string) ([]Constraints, error) {
	stateCons, err := st.State.ConstraintsBySpaceName(spaceName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	spaceCons := make([]Constraints, len(stateCons))
	for i, cons := range stateCons {
		spaceCons[i] = cons
	}
	return spaceCons, nil
}

type spaceRenameModelOp struct {
	st                      RenameSpaceState
	controllerConfigService ControllerConfigService
	isController            bool
	space                   RenameSpace
	settings                Settings
	toName                  string
}

func NewRenameSpaceOp(
	isController bool,
	settings Settings,
	st RenameSpaceState,
	controllerConfigService ControllerConfigService,
	space RenameSpace,
	toName string,
) *spaceRenameModelOp {
	return &spaceRenameModelOp{
		st:                      st,
		controllerConfigService: controllerConfigService,
		settings:                settings,
		space:                   space,
		isController:            isController,
		toName:                  toName,
	}
}

// Build (state.ModelOperation) creates and returns a slice of transaction
// operations necessary to rename a space.
func (o *spaceRenameModelOp) Build(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		if err := o.space.Refresh(); err != nil {
			return nil, errors.Trace(err)
		}
	}

	var ops []txn.Op

	constraintsWithSpace, err := o.st.ConstraintsBySpaceName(o.space.Name())
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, cons := range constraintsWithSpace {
		ops = append(ops, cons.ChangeSpaceNameOps(o.space.Name(), o.toName)...)
	}

	if o.isController {
		settingsDelta, err := o.getSettingsChanges(context.TODO(), o.space.Name(), o.toName)
		if err != nil {
			return nil, errors.Annotatef(err, "retrieving settings changes")
		}

		newSettingsOps, err := o.settings.DeltaOps(state.ControllerSettingsGlobalKey, settingsDelta)
		if err != nil {
			return nil, errors.Trace(err)
		}

		ops = append(ops, newSettingsOps...)
	}

	return ops, nil
}

// getSettingsChanges get's skipped and returns nil if we are not in the controllerModel
func (o *spaceRenameModelOp) getSettingsChanges(ctx context.Context, fromSpaceName, toName string) (settings.ItemChanges, error) {
	currentControllerConfig, err := o.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var deltas settings.ItemChanges

	if mgmtSpace := currentControllerConfig.JujuManagementSpace(); mgmtSpace == fromSpaceName {
		change := settings.MakeModification(controller.JujuManagementSpace, fromSpaceName, toName)
		deltas = append(deltas, change)
	}
	if haSpace := currentControllerConfig.JujuHASpace(); haSpace == fromSpaceName {
		change := settings.MakeModification(controller.JujuHASpace, fromSpaceName, toName)
		deltas = append(deltas, change)
	}
	return deltas, nil
}

// RenameSpace renames a space.
func (api *API) RenameSpace(ctx context.Context, args params.RenameSpacesParams) (params.ErrorResults, error) {
	result := params.ErrorResults{}

	if err := api.ensureSpacesAreMutable(ctx); err != nil {
		return result, err
	}

	result.Results = make([]params.ErrorResult, len(args.Changes))
	for i, spaceRename := range args.Changes {
		fromTag, err := names.ParseSpaceTag(spaceRename.FromSpaceTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}
		if fromTag.Id() == network.AlphaSpaceName {
			newErr := errors.Errorf("the %q space cannot be renamed", network.AlphaSpaceName)
			result.Results[i].Error = apiservererrors.ServerError(newErr)
			continue
		}
		toTag, err := names.ParseSpaceTag(spaceRename.ToSpaceTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}
		toSpace, err := api.networkService.SpaceByName(ctx, toTag.Id())
		if err != nil && !errors.Is(err, errors.NotFound) {
			newErr := errors.Annotatef(err, "retrieving space %q", toTag.Id())
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(newErr))
			continue
		}
		if toSpace != nil {
			newErr := errors.AlreadyExistsf("space %q", toTag.Id())
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(newErr))
			continue
		}

		// TODO(nvinuesa): We still perform the legacy rename operation
		// because it takes into account the checks on space constraints.
		// We are aware that until we migrate those to dqlite, this is
		// not transactional, but at least it allows us to update the
		// space constraints in mongodb. If an error occurs in the
		// new dqlite domain `UpdateSpace` then we will have to deal
		// with the inconsistency.
		operation, err := api.opFactory.NewRenameSpaceOp(fromTag.Id(), toTag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}
		if err = api.backing.ApplyOperation(operation); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}

		fromSpace, err := api.networkService.SpaceByName(ctx, fromTag.Id())
		if err != nil {
			newErr := errors.Annotatef(err, "retrieving space %q", fromTag.Id())
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(newErr))
			continue
		}
		if err := api.networkService.UpdateSpace(ctx, fromSpace.ID, toTag.Id()); err != nil {
			// Here we perform a naive rollback of the changes
			// performed by the previous operation (constraints and
			// controller settings), by simply inverting the from
			// and to space names.
			// TODO(nvinuesa): Same situation as with the orignal
			// operation, this needs to go when we migrate the
			// constraints and settings to dqlite.
			rollbackOperation, err := api.opFactory.NewRenameSpaceOp(toTag.Id(), fromTag.Id())
			if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(errors.Trace(err))
				continue
			}
			if err = api.backing.ApplyOperation(rollbackOperation); err != nil {
				result.Results[i].Error = apiservererrors.ServerError(errors.Trace(err))
				continue
			}

			newErr := errors.Annotatef(err, "updating space %q", fromTag.Id())
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(newErr))
			continue
		}
	}
	return result, nil
}

func (o *spaceRenameModelOp) Done(err error) error {
	return err
}
