// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	"github.com/juju/errors"
	"gopkg.in/macaroon.v2"

	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/rpc/params"
)

// MigrationMacaroonMinter mints a directly-presentable login macaroon on the
// target controller, in exchange for the credentials already used to open the
// precheck connection. Satisfied by api/controller/migrationtarget.Client.
type MigrationMacaroonMinter interface {
	// CreateMigrationMacaroon asks the target controller to mint a 24h login
	// macaroon for the user authenticated on the current connection.
	CreateMigrationMacaroon(ctx context.Context) ([]macaroon.Slice, error)
}

// HarvestMigrationMacaroon exchanges the admin password in targetInfo for a
// directly-presentable 24h login macaroon minted by the target controller.
// On success, targetInfo.Macaroons is populated and targetInfo.Password is
// cleared so the cleartext credential is never persisted.
//
// It is a no-op when a token or macaroons are already present (those flows
// do not need a password exchange). When the target is too old to support
// the method, an error is returned — callers must NOT fall back to
// persisting the cleartext password.
func HarvestMigrationMacaroon(ctx context.Context, targetInfo *coremigration.TargetInfo, client MigrationMacaroonMinter) error {
	if targetInfo.Password == "" || len(targetInfo.Macaroons) != 0 || targetInfo.Token != "" {
		return nil
	}
	macs, err := client.CreateMigrationMacaroon(ctx)
	if err != nil {
		if params.IsCodeNotImplemented(err) {
			return errors.New("target controller is too old to support password-authenticated migration; upgrade the target to a version that exposes MigrationTarget v8 or later")
		}
		return errors.Annotate(err, "cannot obtain migration macaroon from target")
	}
	targetInfo.Macaroons = macs
	targetInfo.Password = ""
	return nil
}
