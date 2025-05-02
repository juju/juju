// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitcommon

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
)

// ApplicationService describes the ability to check if an
// application exists in the model.
type ApplicationService interface {
	// GetApplicationIDByName returns nil if the application exists.
	// Otherwise, it returns an error.
	GetApplicationIDByName(ctx context.Context, name string) (application.ID, error)
}

// UnitAccessor returns an auth function which determines if the
// authenticated entity can access a unit or application.
func UnitAccessor(authorizer facade.Authorizer, applicationService ApplicationService) common.GetAuthFunc {
	return func(ctx context.Context) (common.AuthFunc, error) {
		switch authTag := authorizer.GetAuthTag().(type) {
		case names.ApplicationTag:
			// If called by an application agent, any of the units
			// belonging to that application can be accessed.
			appName := authTag.Name
			if _, err := applicationService.GetApplicationIDByName(ctx, appName); errors.Is(err, applicationerrors.ApplicationNotFound) {
				return nil, errors.NotFoundf("application %q", appName)
			} else if err != nil {
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
