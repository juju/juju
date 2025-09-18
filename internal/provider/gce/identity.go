// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"context"

	"github.com/juju/errors"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

// SupportsInstanceRoles indicates if Instance Roles are supported by this
// environ.
func (env *environ) SupportsInstanceRoles(_ context.Context) bool {
	return true
}

// CreateAutoInstanceRole is responsible for setting up an instance role on
// behalf of the user.
func (env *environ) CreateAutoInstanceRole(
	ctx context.Context,
	args environs.BootstrapParams,
) (string, error) {
	serviceAccount, err := env.gce.DefaultServiceAccount(ctx)
	return serviceAccount, errors.Trace(err)
}

// FinaliseBootstrapCredential is responsible for performing and finalisation
// steps to a credential being passed to a newly bootstrapped controller. This
// was introduced to help with the transformation to instance roles.
func (env *environ) FinaliseBootstrapCredential(
	ctx environs.BootstrapContext,
	args environs.BootstrapParams,
	cred *jujucloud.Credential,
) (*jujucloud.Credential, error) {
	if !args.BootstrapConstraints.HasInstanceRole() ||
		cred == nil || cred.AuthType() == jujucloud.ServiceAccountAuthType {
		return cred, nil
	}

	serviceAccountName := *args.BootstrapConstraints.InstanceRole
	newCred := jujucloud.NewCredential(jujucloud.ServiceAccountAuthType, map[string]string{
		credServiceAccount: serviceAccountName,
	})
	return &newCred, nil
}
