// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"

	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/errors"
)

// UserUUIDResolver resolves a controller username to its user UUID. The v8
// migration import driver supplies one backed by the access user service, so
// the keymanager domain does not depend on access to authorise migrated keys.
type UserUUIDResolver func(context.Context, user.Name) (user.UUID, error)

// ImportAuthorizedKeys adds the SSH public keys authorised for the model to
// their owning users, skipping users in inactiveUsers (the set returned by the
// access user import). User UUIDs are resolved via resolveUser, supplied by the
// caller so this domain need not import the access service.
//
// It is called directly by the v8 migration import driver in
// internal/migration.
func (s *Service) ImportAuthorizedKeys(
	ctx context.Context,
	keys []coremodelmigration.ModelAuthorizedKey,
	inactiveUsers set.Strings,
	resolveUser UserUUIDResolver,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(keys) == 0 {
		return nil
	}

	byUser := make(map[string][]string)
	var order []string
	for _, k := range keys {
		if inactiveUsers.Contains(k.Username) {
			continue
		}
		if _, ok := byUser[k.Username]; !ok {
			order = append(order, k.Username)
		}
		byUser[k.Username] = append(byUser[k.Username], k.PublicKey)
	}

	for _, username := range order {
		name, err := user.NewName(username)
		if err != nil {
			return errors.Errorf("invalid authorized-key username %q: %w", username, err)
		}
		userUUID, err := resolveUser(ctx, name)
		if err != nil {
			return errors.Errorf("resolving user %q for authorized keys: %w", username, err)
		}
		if err := s.AddPublicKeysForUser(ctx, userUUID, byUser[username]...); err != nil {
			return errors.Errorf("adding authorized keys for user %q: %w", username, err)
		}
	}
	return nil
}
