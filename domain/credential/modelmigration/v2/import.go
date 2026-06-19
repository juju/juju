// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v2

import (
	"context"
	"reflect"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	coreuser "github.com/juju/juju/core/user"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	"github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/internal/errors"
)

// ImportModelCredential creates the model's cloud credential on the target
// if absent, or validates that an existing same-key credential matches (auth
// type, attributes, not revoked) rather than overwriting it.
func ImportModelCredential(
	ctx context.Context, controllerDB database.TxnRunnerFactory, logger logger.Logger,
	ref coremodelmigration.ModelCloudCredential,
) (corecredential.Key, error) {
	owner, err := coreuser.NewName(ref.Owner)
	if err != nil {
		return corecredential.Key{}, errors.Errorf("invalid credential owner %q: %w", ref.Owner, err)
	}
	key := corecredential.Key{Cloud: ref.Cloud, Owner: owner, Name: ref.Name}
	if err := key.Validate(); err != nil {
		return corecredential.Key{}, errors.Capture(err)
	}

	credSvc := service.NewService(state.NewState(controllerDB), logger)

	existing, err := credSvc.CloudCredential(ctx, key)
	if errors.Is(err, credentialerrors.NotFound) {
		cred := cloud.NewCredential(cloud.AuthType(ref.AuthType), ref.Attributes)
		cred.Revoked = ref.Revoked
		cred.Invalid = ref.Invalid
		cred.InvalidReason = ref.InvalidReason
		if err := credSvc.InsertCloudCredential(ctx, key, cred); err != nil {
			return corecredential.Key{}, errors.Errorf("creating credential %q: %w", key, err)
		}
		return key, nil
	} else if err != nil {
		return corecredential.Key{}, errors.Errorf("looking up credential %q: %w", key, err)
	}

	if existing.AuthType() != cloud.AuthType(ref.AuthType) {
		return corecredential.Key{}, errors.Errorf(
			"credential %q auth type mismatch: %q != %q", key, existing.AuthType(), ref.AuthType)
	}
	if !reflect.DeepEqual(existing.Attributes(), ref.Attributes) {
		return corecredential.Key{}, errors.Errorf("credential %q attribute mismatch", key)
	}
	if existing.Revoked {
		return corecredential.Key{}, errors.Errorf("credential %q is revoked", key)
	}
	return key, nil
}
