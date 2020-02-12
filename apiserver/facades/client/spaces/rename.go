// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/settings"
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
// * This allows us to indirect state at the operation level instead of the
// * whole API level as currently done in interface.go
type RenameSpaceState interface {
	// ControllerConfig returns current ControllerConfig.
	ControllerConfig() (jujucontroller.Config, error)

	// ConstraintsOpsForSpaceNameChange returns all the database transaction operation required
	// to transform a constraints spaces from `a` to `b`
	ConstraintsOpsForSpaceNameChange(spaceName, toName string) ([]txn.Op, error)
}

// Settings describes methods for interacting with settings to apply
// space-based configuration deltas.
type Settings interface {
	DeltaOps(key string, delta settings.ItemChanges) ([]txn.Op, error)
}

type spaceRenameModelOp struct {
	st           RenameSpaceState
	isController bool
	space        RenameSpace
	settings     Settings
	toName       string
}

func (o *spaceRenameModelOp) Done(err error) error {
	return err
}

func NewRenameSpaceModelOp(isController bool, settings Settings, st RenameSpaceState, space RenameSpace, toName string) *spaceRenameModelOp {
	return &spaceRenameModelOp{
		st:           st,
		settings:     settings,
		space:        space,
		isController: isController,
		toName:       toName,
	}
}

type renameSpaceStateShim struct {
	*state.State
}

// Build (state.ModelOperation) creates and returns a slice of transaction
// operations necessary to rename a space.
func (o *spaceRenameModelOp) Build(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		if err := o.space.Refresh(); err != nil {
			return nil, errors.Trace(err)
		}
	}

	newConstraintsOps, err := o.st.ConstraintsOpsForSpaceNameChange(o.space.Name(), o.toName)
	if err != nil {
		newErr := errors.Annotatef(err, "retrieving constraint changes")
		return nil, errors.Trace(newErr)
	}

	completeOps := o.space.RenameSpaceOps(o.toName)

	totalOps := append(completeOps, newConstraintsOps...)

	if o.isController {
		settingsDelta, err := o.getSettingsChanges(o.space.Name(), o.toName)
		if err != nil {
			return nil, errors.Annotatef(err, "retrieving setting changes")
		}

		newSettingsOps, err := o.settings.DeltaOps(state.ControllerSettingsGlobalKey, settingsDelta)
		if err != nil {
			return nil, errors.Trace(err)
		}

		totalOps = append(totalOps, newSettingsOps...)
	}

	return totalOps, nil
}

// getSettingsChanges get's skipped and returns nil if we are not in the controllerModel
func (o *spaceRenameModelOp) getSettingsChanges(fromSpaceName, toName string) (settings.ItemChanges, error) {
	currentControllerConfig, err := o.st.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var deltas settings.ItemChanges

	if mgmtSpace := currentControllerConfig.JujuManagementSpace(); mgmtSpace == fromSpaceName {
		change := settings.MakeModification(jujucontroller.JujuManagementSpace, fromSpaceName, toName)
		deltas = append(deltas, change)
	}
	if haSpace := currentControllerConfig.JujuHASpace(); haSpace == fromSpaceName {
		change := settings.MakeModification(jujucontroller.JujuHASpace, fromSpaceName, toName)
		deltas = append(deltas, change)
	}
	return deltas, nil
}

// RenameSpace renames a space.
func (api *API) RenameSpace(args params.RenameSpacesParams) (params.ErrorResults, error) {
	var errorResults params.ErrorResults

	err := api.checkSpacesCRUDPermissions()
	if err != nil {
		return errorResults, err
	}

	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.SpacesRenames)),
	}

	for i, spaceRename := range args.SpacesRenames {
		fromTag, err := names.ParseSpaceTag(spaceRename.FromSpaceTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}
		if fromTag.Id() == network.AlphaSpaceName {
			newErr := errors.New("the alpha space cannot be renamed")
			results.Results[i].Error = common.ServerError(newErr)
			continue
		}
		toTag, err := names.ParseSpaceTag(spaceRename.ToSpaceTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}
		toSpace, err := api.backing.SpaceByName(toTag.Id())
		if err != nil && !errors.IsNotFound(err) {
			newErr := errors.Annotatef(err, "retrieving space: %q unexpected error, besides not found", toTag.Id())
			results.Results[i].Error = common.ServerError(errors.Trace(newErr))
			continue
		}
		if toSpace != nil {
			newErr := errors.AlreadyExistsf("space: %q", toTag.Id())
			results.Results[i].Error = common.ServerError(errors.Trace(newErr))
			continue
		}
		operation, err := api.opFactory.NewRenameSpaceModelOp(fromTag.Id(), toTag.Id())
		if err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}
		if err = api.backing.ApplyOperation(operation); err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}
	}
	return results, nil
}
