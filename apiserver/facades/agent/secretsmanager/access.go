// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
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

func (s *SecretsManagerAPI) checkCanRead(uri *coresecrets.URI) error {
	// TODO(wallyworld)
	return nil
}
