// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package modelupgrader defines an API end point for functions dealing with
// upgrading models.
package modelupgrader

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/upgrades/upgradevalidation"
)

// ModelUpgraderAPI implements the model upgrader interface and is
// the concrete implementation of the api end point.
type ModelUpgraderAPI struct {
	statePool   StatePool
	check       common.BlockCheckerInterface
	authorizer  facade.Authorizer
	toolsFinder common.ToolsFinder
	apiUser     names.UserTag
	isAdmin     bool
	callContext context.ProviderCallContext
	newEnviron  common.NewEnvironFunc
}

// NewModelUpgraderAPI creates a new api server endpoint for managing
// models.
func NewModelUpgraderAPI(
	controllerTag names.ControllerTag,
	stPool StatePool,
	toolsFinder common.ToolsFinder,
	newEnviron common.NewEnvironFunc,
	blockChecker common.BlockCheckerInterface,
	authorizer facade.Authorizer,
	callCtx context.ProviderCallContext,
) (*ModelUpgraderAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := authorizer.GetAuthTag().(names.UserTag)

	isAdmin, err := authorizer.HasPermission(permission.SuperuserAccess, controllerTag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &ModelUpgraderAPI{
		statePool:   stPool,
		check:       blockChecker,
		authorizer:  authorizer,
		toolsFinder: toolsFinder,
		apiUser:     apiUser,
		isAdmin:     isAdmin,
		callContext: callCtx,
		newEnviron:  newEnviron,
	}, nil
}

func (m *ModelUpgraderAPI) hasWriteAccess(modelTag names.ModelTag) (bool, error) {
	canWrite, err := m.authorizer.HasPermission(permission.WriteAccess, modelTag)
	if errors.IsNotFound(err) {
		return false, nil
	}
	return canWrite, err
}

// ConfigSource describes a type that is able to provide config.
// Abstracted primarily for testing.
type ConfigSource interface {
	Config() (*config.Config, error)
}

// AbortModelUpgrade aborts and archives the model upgrade
// synchronisation record, if any.
func (m *ModelUpgraderAPI) AbortModelUpgrade(arg params.ModelParam) error {
	modelTag, err := names.ParseModelTag(arg.ModelTag)
	if err != nil {
		return errors.Trace(err)
	}
	if canWrite, err := m.hasWriteAccess(modelTag); err != nil {
		return errors.Trace(err)
	} else if !canWrite && !m.isAdmin {
		return apiservererrors.ErrPerm
	}

	if err := m.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	st, err := m.statePool.Get(modelTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	defer st.Release()
	return st.AbortCurrentUpgrade()
}

// UpgradeModel upgrades a model.
func (m *ModelUpgraderAPI) UpgradeModel(arg params.UpgradeModel) error {
	modelTag, err := names.ParseModelTag(arg.ModelTag)
	if err != nil {
		return errors.Trace(err)
	}
	if canWrite, err := m.hasWriteAccess(modelTag); err != nil {
		return errors.Trace(err)
	} else if !canWrite && !m.isAdmin {
		return apiservererrors.ErrPerm
	}

	if err := m.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}

	// Before changing the agent version to trigger an upgrade or downgrade,
	// we'll do a very basic check to ensure the environment is accessible.
	envOrBroker, err := m.newEnviron()
	if err != nil {
		return errors.Trace(err)
	}
	if err := environs.CheckProviderAPI(envOrBroker, m.callContext); err != nil {
		return errors.Trace(err)
	}
	if err := m.validateModelUpgrade(false, modelTag, arg.ToVersion); err != nil {
		return errors.Trace(err)
	}
	if arg.DryRun {
		return nil
	}
	st, err := m.statePool.Get(modelTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	defer st.Release()
	return st.SetModelAgentVersion(arg.ToVersion, &arg.AgentStream, arg.IgnoreAgentVersions)
}

func (m *ModelUpgraderAPI) validateModelUpgrade(force bool, modelTag names.ModelTag, targetVersion version.Number) (err error) {
	var blockers *upgradevalidation.ModelUpgradeBlockers
	defer func() {
		if err == nil && blockers != nil {
			err = apiservererrors.ServerError(
				errors.NewNotSupported(nil,
					fmt.Sprintf(
						"cannot upgrade to %q due to issues with these models:\n%s",
						targetVersion, blockers,
					),
				),
			)
		}
	}()

	// We now need to access the state pool for that given model.
	st, err := m.statePool.Get(modelTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	defer st.Release()

	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	isControllerModel := model.IsControllerModel()
	if !isControllerModel {
		validators := upgradevalidation.ValidatorsForModelUpgrade(force, targetVersion)
		checker := upgradevalidation.NewModelUpgradeCheck(modelTag.Id(), m.statePool, st, model, validators...)
		blockers, err = checker.Validate()
		if err != nil {
			return errors.Trace(err)
		}
		return
	}

	checker := upgradevalidation.NewModelUpgradeCheck(
		modelTag.Id(), m.statePool, st, model,
		upgradevalidation.ValidatorsForControllerUpgrade(true, targetVersion)...,
	)
	blockers, err = checker.Validate()
	if err != nil {
		return errors.Trace(err)
	}

	modelUUIDs, err := st.AllModelUUIDs()
	if err != nil {
		return errors.Trace(err)
	}
	validators := upgradevalidation.ValidatorsForControllerUpgrade(false, targetVersion)
	for _, modelUUID := range modelUUIDs {
		if modelUUID == modelTag.Id() {
			// We have done checks for controller model above already.
			continue
		}
		st, err := m.statePool.Get(modelUUID)
		if err != nil {
			return errors.Trace(err)
		}
		defer st.Release()
		model, err := st.Model()
		if err != nil {
			return errors.Trace(err)
		}
		checker := upgradevalidation.NewModelUpgradeCheck(modelUUID, m.statePool, st, model, validators...)
		blockersForModel, err := checker.Validate()
		if err != nil {
			return errors.Trace(err)
		}
		if blockersForModel == nil {
			// all good.
			continue
		}
		if blockers == nil {
			blockers = blockersForModel
			continue
		}
		blockers.Join(blockersForModel)
	}
	return
}
