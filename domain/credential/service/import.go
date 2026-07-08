// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"reflect"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/trace"
	coreuser "github.com/juju/juju/core/user"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	"github.com/juju/juju/internal/errors"
)

// ImportModelCredential creates the model's cloud credential on the target if
// absent, or validates that an existing same-key credential matches (auth
// type, attributes, not revoked) rather than overwriting it. It returns the
// resolved credential key for the caller to attach to the imported model.
//
// It is called directly by the v8 migration import driver in
// internal/migration.
func (s *Service) ImportModelCredential(
	ctx context.Context, ref coremodelmigration.ModelCloudCredential,
) (corecredential.Key, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	owner, err := coreuser.NewName(ref.Owner)
	if err != nil {
		return corecredential.Key{}, errors.Errorf("invalid credential owner %q: %w", ref.Owner, err)
	}
	key := corecredential.Key{Cloud: ref.Cloud, Owner: owner, Name: ref.Name}
	if err := key.Validate(); err != nil {
		return corecredential.Key{}, errors.Capture(err)
	}

	existing, err := s.CloudCredential(ctx, key)
	if errors.Is(err, credentialerrors.NotFound) {
		cred := cloud.NewCredential(cloud.AuthType(ref.AuthType), ref.Attributes)
		cred.Revoked = ref.Revoked
		cred.Invalid = ref.Invalid
		cred.InvalidReason = ref.InvalidReason
		if err := s.InsertCloudCredential(ctx, key, cred); err != nil {
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
