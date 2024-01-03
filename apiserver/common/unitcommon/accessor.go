// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitcommon

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

// ApplicationGetter provides a method
// to determine if an application exists.
type ApplicationGetter interface {
	ApplicationExists(string) error
}

type stateApplicationGetter interface {
	Application(string) (*state.Application, error)
}

// Backend returns an application abstraction for a
// given state.State instance.
func Backend(st stateApplicationGetter) ApplicationGetter {
	return backend{st}
}

type backend struct {
	stateApplicationGetter
}

// ApplicationExists implements ApplicationGetter.
func (b backend) ApplicationExists(name string) error {
	_, err := b.stateApplicationGetter.Application(name)
	return err
}

// UnitAccessor returns an auth function which determines if the
// authenticated entity can access a unit or application.
func UnitAccessor(authorizer facade.Authorizer, st ApplicationGetter) common.GetAuthFunc {
	return func() (common.AuthFunc, error) {
		switch authTag := authorizer.GetAuthTag().(type) {
		case names.ApplicationTag:
			// If called by an application agent, any of the units
			// belonging to that application can be accessed.
			appName := authTag.Name
			err := st.ApplicationExists(appName)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return func(tag names.Tag) bool {
				if tag.Kind() != names.UnitTagKind {
					return false
				}
				unitApp, err := names.UnitApplication(tag.Id())
				if err != nil {
					return false
				}
				return unitApp == appName
			}, nil
		case names.UnitTag:
			return func(tag names.Tag) bool {
				if tag.Kind() == names.ApplicationTagKind {
					unitApp, err := names.UnitApplication(authTag.Id())
					if err != nil {
						return false
					}
					return unitApp == tag.Id()
				}
				return authorizer.AuthOwner(tag)
			}, nil
		default:
			return nil, errors.Errorf("expected names.UnitTag or names.ApplicationTag, got %T", authTag)
		}
	}
}
