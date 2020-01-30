/*
 * // Copyright 2020 Canonical Ltd.
 * // Licensed under the AGPLv3, see LICENCE file for details.
 */

package spaces

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/settings"
	"github.com/juju/juju/state"
)

// RenameSpaceModelOp describes a model operation for committing a branch.
type RenameSpaceModelOp interface {
	state.ModelOperation
}

// RenameSpace describes a space that can be renamed..
type RenameSpace interface {
	Refresh() error
	Id() string
	Name() string
	RenameSpaceCompleteOps(toName string) ([]txn.Op, error)
}

// CommitBranchState describes state operations required
// to execute the renameSpace operation.
// * This allows us to indirect state at the operation level instead of the
// * whole API level as currently done in interface.go
type RenameSpaceState interface {
	// ControllerConfig returns current ControllerConfig.
	ControllerConfig() (jujucontroller.Config, error)

	//ConstraintsBySpace returns current constraints using the given spaceName.
	ConstraintsBySpace(spaceName string) (map[string]constraints.Value, error)

	ControllerSettingsGlobalKey() string

	// GetConstraintsOps gets the database transaction operations for the given constraints.
	// Cons is  a map keyed by the DocID.
	GetConstraintsOps(cons map[string]constraints.Value) ([]txn.Op, error)
}

// Settings describes methods for interacting with settings to apply
// branch-based configuration deltas.
type Settings interface {
	DeltaOps(key string, delta settings.ItemChanges) ([]txn.Op, error)
}

type spaceRenameModelOp struct {
	st       RenameSpaceState
	space    RenameSpace
	settings Settings
	toName   string
}

func (o *spaceRenameModelOp) Done(err error) error {
	return err
}

func NewRenameSpaceModelOp(settings Settings, st RenameSpaceState, space RenameSpace, toName string) *spaceRenameModelOp {
	return &spaceRenameModelOp{
		st:       st,
		settings: settings,
		space:    space,
		toName:   toName,
	}
}

type renameSpaceStateShim struct {
	*state.State
}

func (r *renameSpaceStateShim) ConstraintsBySpace(spaceName string) (map[string]constraints.Value, error) {
	return r.State.ConstraintsBySpaceName(spaceName)
}

func (r *renameSpaceStateShim) ControllerConfig() (jujucontroller.Config, error) {
	result, err := r.State.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
}

func (r *renameSpaceStateShim) GetConstraintsOps(cons map[string]constraints.Value) ([]txn.Op, error) {
	ops, err := r.State.GetConstraintsOps(cons)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ops, nil
}

func (r *renameSpaceStateShim) ControllerSettingsGlobalKey() string {
	return r.State.ControllerSettingsGlobalKey()
}

// Build (state.ModelOperation) creates and returns a slice of transaction
// operations necessary to rename a space.
func (o *spaceRenameModelOp) Build(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		if err := o.space.Refresh(); err != nil {
			return nil, errors.Trace(err)
		}
	}

	var totalOps []txn.Op

	settingsDelta, err := o.getSettingsChanges(o.space.Name(), o.toName)
	if err != nil {
		newErr := errors.Annotatef(err, "retrieving setting changes")
		return nil, errors.Trace(newErr)
	}
	newConstraints, err := o.getConstraintsChanges(o.space.Name(), o.toName)
	if err != nil {
		newErr := errors.Annotatef(err, "retrieving constraint changes")
		return nil, errors.Trace(newErr)
	}

	newConstraintsOps, err := o.st.GetConstraintsOps(newConstraints)
	if err != nil {
		return nil, errors.Trace(err)
	}
	newSettingsOps, err := o.settings.DeltaOps(o.st.ControllerSettingsGlobalKey(), settingsDelta)
	if err != nil {
		return nil, errors.Trace(err)
	}

	completeOps, err := o.space.RenameSpaceCompleteOps(o.toName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	totalOps = append(totalOps, completeOps...)
	totalOps = append(totalOps, newConstraintsOps...)
	totalOps = append(totalOps, newSettingsOps...)

	return totalOps, nil
}

// getConstraintsChanges gets the current constraints to update.
func (o *spaceRenameModelOp) getConstraintsChanges(fromSpaceName, toName string) (map[string]constraints.Value, error) {
	currentConstraints, err := o.st.ConstraintsBySpace(fromSpaceName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	toConstraints := make(map[string]*constraints.Value, len(currentConstraints))
	for id, constraint := range currentConstraints {
		toConstraints[id] = &constraint
		spaces := *constraint.Spaces
		for i, space := range spaces {
			if space == fromSpaceName {
				spaces[i] = toName
				constraint.Spaces = &spaces
				break
			}
		}
	}
	return currentConstraints, nil
}

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
	isAdmin, err := api.auth.HasPermission(permission.AdminAccess, api.backing.ModelTag())
	if err != nil && !errors.IsNotFound(err) {
		return params.ErrorResults{}, errors.Trace(err)
	}
	if !isAdmin {
		return params.ErrorResults{}, common.ServerError(common.ErrPerm)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	if err = api.checkSupportsProviderSpaces(); err != nil {
		return params.ErrorResults{}, common.ServerError(errors.Trace(err))
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
