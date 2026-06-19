// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v2

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/collections/set"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	coreuser "github.com/juju/juju/core/user"
	accessservice "github.com/juju/juju/domain/access/service"
	accessstate "github.com/juju/juju/domain/access/state"
	"github.com/juju/juju/domain/keymanager/service"
	"github.com/juju/juju/domain/keymanager/state"
	"github.com/juju/juju/internal/errors"
)

// ImportAuthorizedKeys adds the SSH public keys authorised for the model to
// their owning users, skipping users in inactiveUsers (the set returned by
// [github.com/juju/juju/domain/access/modelmigration/v2.ImportModelUsers]).
func ImportAuthorizedKeys(
	ctx context.Context, controllerDB database.TxnRunnerFactory, clock clock.Clock,
	modelUUID coremodel.UUID, keys []coremodelmigration.ModelAuthorizedKey, inactiveUsers set.Strings,
) error {
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

	userSvc := accessservice.NewUserService(accessstate.NewUserState(controllerDB, clock), clock)
	keyManagerSvc := service.NewService(modelUUID, state.NewState(controllerDB))
	for _, username := range order {
		name, err := coreuser.NewName(username)
		if err != nil {
			return errors.Errorf("invalid authorized-key username %q: %w", username, err)
		}
		userUUID, err := userSvc.GetUserUUIDByName(ctx, name)
		if err != nil {
			return errors.Errorf("resolving user %q for authorized keys: %w", username, err)
		}
		if err := keyManagerSvc.AddPublicKeysForUser(ctx, userUUID, byUser[username]...); err != nil {
			return errors.Errorf("adding authorized keys for user %q: %w", username, err)
		}
	}
	return nil
}
