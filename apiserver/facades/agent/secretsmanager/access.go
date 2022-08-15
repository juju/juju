// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	coresecrets "github.com/juju/juju/core/secrets"
)

func secretOwner(agentAppName string) common.GetAuthFunc {
	return func() (common.AuthFunc, error) {
		return func(secretOwnerTag names.Tag) bool {
			// We currently only support secrets owned by applications.
			if secretOwnerTag.Kind() != names.ApplicationTagKind {
				return false
			}
			return agentAppName == secretOwnerTag.Id()
		}, nil
	}
}

type canReadSecretFunc func(consumer names.Tag, uri *coresecrets.URI) bool

func canReadSecret(authorizer facade.Authorizer) canReadSecretFunc {
	return func(consumer names.Tag, uri *coresecrets.URI) bool {
		if !authorizer.AuthOwner(consumer) {
			return false
		}
		// TODO(wallyworld)
		return true
	}
}
