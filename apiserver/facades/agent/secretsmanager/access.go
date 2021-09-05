// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
)

func secretAccessor(authorizer facade.Authorizer) common.GetAuthFunc {
	return func() (common.AuthFunc, error) {
		switch tag := authorizer.GetAuthTag().(type) {
		case names.UnitTag:
			return unitAgentSecretAccessor(tag)
		default:
			return nil, errors.Errorf("expected names.UnitTag, got %T", tag)
		}
	}
}

func unitAgentSecretAccessor(unitTag names.UnitTag) (common.AuthFunc, error) {
	return func(ownerTag names.Tag) bool {
		switch ownerTag.Kind() {
		case names.ApplicationTagKind:
			appName, err := names.UnitApplication(unitTag.Id())
			return err == nil && appName == ownerTag.Id()
		case names.UnitTagKind:
			return unitTag.Id() == ownerTag.Id()
		default:
			return false
		}
	}, nil
}
