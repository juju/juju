// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/state"
)

// unitAccessor creates a accessUnit function for accessing a unit
func unitAccessor(authorizer facade.Authorizer, st *state.State) common.GetAuthFunc {
	return func() (common.AuthFunc, error) {
		switch tag := authorizer.GetAuthTag().(type) {
		case names.ApplicationTag:
			// If called by an application agent, any of the units
			// belonging to that application can be accessed.
			app, err := st.Application(tag.Name)
			if err != nil {
				return nil, errors.Trace(err)
			}
			allUnits, err := app.AllUnits()
			if err != nil {
				return nil, errors.Trace(err)
			}
			return func(tag names.Tag) bool {
				for _, u := range allUnits {
					if u.Tag() == tag {
						return true
					}
				}
				return false
			}, nil
		case names.UnitTag:
			return func(tag names.Tag) bool {
				return authorizer.AuthOwner(tag)
			}, nil
		default:
			return nil, errors.Errorf("expected names.UnitTag or names.ApplicationTag, got %T", tag)
		}
	}
}

func applicationAccessor(authorizer facade.Authorizer, st *state.State) common.GetAuthFunc {
	return func() (common.AuthFunc, error) {
		switch tag := authorizer.GetAuthTag().(type) {
		case names.ApplicationTag:
			return func(applicationTag names.Tag) bool {
				return tag == applicationTag
			}, nil
		case names.UnitTag:
			entity, err := st.Unit(tag.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
			applicationName := entity.ApplicationName()
			applicationTag := names.NewApplicationTag(applicationName)
			return func(tag names.Tag) bool {
				return tag == applicationTag
			}, nil
		default:
			return nil, errors.Errorf("expected names.UnitTag or names.ApplicationTag, got %T", tag)
		}
	}
}

func machineAccessor(authorizer facade.Authorizer, st *state.State) common.GetAuthFunc {
	return func() (common.AuthFunc, error) {
		switch tag := authorizer.GetAuthTag().(type) {
		// Application agents can't access machines.
		case names.ApplicationTag:
			return func(tag names.Tag) bool {
				return false
			}, nil
		case names.UnitTag:
			entity, err := st.Unit(tag.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
			machineId, err := entity.AssignedMachineId()
			if err != nil {
				return nil, errors.Trace(err)
			}
			machineTag := names.NewMachineTag(machineId)
			return func(tag names.Tag) bool {
				return tag == machineTag
			}, nil
		default:
			return nil, errors.Errorf("expected names.UnitTag or names.ApplicationTag, got %T", tag)
		}
	}
}

func cloudSpecAccessor(authorizer facade.Authorizer, st *state.State) func() (func() bool, error) {
	return func() (func() bool, error) {
		var appName string
		var err error

		switch tag := authorizer.GetAuthTag().(type) {
		case names.ApplicationTag:
			appName = tag.Id()
		case names.UnitTag:
			entity, err := st.Unit(tag.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
			appName = entity.ApplicationName()
		default:
			return nil, errors.Errorf("expected names.UnitTag or names.ApplicationTag, got %T", tag)
		}

		app, err := st.Application(appName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		config, err := app.ApplicationConfig()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return func() bool {
			return config.GetBool(application.TrustConfigOptionName, false)
		}, nil
	}
}
