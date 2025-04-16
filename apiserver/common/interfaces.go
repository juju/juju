// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

// AuthFunc returns whether the given entity is available to some operation.
type AuthFunc func(tag names.Tag) bool

// GetAuthFunc returns an AuthFunc.
type GetAuthFunc func() (AuthFunc, error)

// AuthAny returns an AuthFunc generator that returns an AuthFunc that
// accepts any tag authorized by any of its arguments. If no arguments
// are passed this is equivalent to AuthNever.
func AuthAny(getFuncs ...GetAuthFunc) GetAuthFunc {
	return func() (AuthFunc, error) {
		funcs := make([]AuthFunc, len(getFuncs))
		for i, getFunc := range getFuncs {
			f, err := getFunc()
			if err != nil {
				return nil, errors.Trace(err)
			}
			funcs[i] = f
		}
		combined := func(tag names.Tag) bool {
			for _, f := range funcs {
				if f(tag) {
					return true
				}
			}
			return false
		}
		return combined, nil
	}
}

// AuthAlways returns an authentication function that always returns true iff it is passed a valid tag.
func AuthAlways() GetAuthFunc {
	return func() (AuthFunc, error) {
		return func(tag names.Tag) bool {
			return true
		}, nil
	}
}

// AuthFuncForTag returns an authentication function that always returns true iff it is passed a specific tag.
func AuthFuncForTag(valid names.Tag) GetAuthFunc {
	return func() (AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag == valid
		}, nil
	}
}

// AuthFuncForTagKind returns a GetAuthFunc which creates an AuthFunc
// allowing only the given tag kind and denies all others. Passing an
// empty kind is an error.
func AuthFuncForTagKind(kind string) GetAuthFunc {
	return func() (AuthFunc, error) {
		if kind == "" {
			return nil, errors.Errorf("tag kind cannot be empty")
		}
		return func(tag names.Tag) bool {
			// Allow only the given tag kind.
			if tag == nil {
				return false
			}
			return tag.Kind() == kind
		}, nil
	}
}

// Authorizer represents the authenticated entity using the API server.
type Authorizer interface {

	// AuthController returns whether the authenticated entity is
	// a machine acting as a controller. Can't be removed from this
	// interface without introducing a dependency on something else
	// to look up that property: it's not inherent in the result of
	// GetAuthTag, as the other methods all are.
	AuthController() bool

	// AuthMachineAgent returns true if the entity is a machine agent.
	AuthMachineAgent() bool

	// GetAuthTag returns the entity's tag.
	GetAuthTag() names.Tag
}

// AuthFuncForMachineAgent returns a GetAuthFunc which creates an AuthFunc
// allowing only machine agents and their controllers
func AuthFuncForMachineAgent(authorizer Authorizer) GetAuthFunc {
	return func() (AuthFunc, error) {
		isModelManager := authorizer.AuthController()
		isMachineAgent := authorizer.AuthMachineAgent()
		authEntityTag := authorizer.GetAuthTag()

		return func(tag names.Tag) bool {
			if isMachineAgent && tag == authEntityTag {
				// A machine agent can always access its own machine.
				return true
			}

			switch tag := tag.(type) {
			case names.MachineTag:
				parentId := container.ParentId(tag.Id())
				if parentId == "" {
					// All top-level machines are accessible by the controller.
					return isModelManager
				}
				// All containers with the authenticated machine as a
				// parent are accessible by it.
				// TODO(dfc) sometimes authEntity tag is nil, which is fine because nil is
				// only equal to nil, but it suggests someone is passing an authorizer
				// with a nil tag.
				return isMachineAgent && names.NewMachineTag(parentId) == authEntityTag
			default:
				return false
			}
		}, nil
	}
}

// ControllerConfigState defines the methods needed by
// ControllerConfigAPI
type ControllerConfigState interface {
	APIHostPortsForAgents(controller.Config) ([]network.SpaceHostPorts, error)
	CompletedMigrationForModel(string) (state.ModelMigration, error)
}

type controllerInfoState interface {
	APIHostPortsForAgents(controller.Config) ([]network.SpaceHostPorts, error)
}
