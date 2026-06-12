// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

const sshServerHostKeyID = "sshServerHostKey"

// InsertInitialSSHServerHostKey inserts the controller jump host key into the
// controller database during bootstrap.
func InsertInitialSSHServerHostKey(sshServerHostKey string) internaldatabase.BootstrapOpt {
	return func(ctx context.Context, controller, model database.TxnRunner) error {
		if sshServerHostKey == "" {
			return errors.Errorf("empty SSHServerHostKey").Add(coreerrors.NotValid)
		}

		record := controllerSSHHostKey{ID: sshServerHostKeyID, SSHKey: sshServerHostKey}
		stmt, err := sqlair.Prepare(`
INSERT INTO controller_ssh_host_key (id, ssh_key)
VALUES ($controllerSSHHostKey.*)`, controllerSSHHostKey{})
		if err != nil {
			return errors.Capture(err)
		}

		return errors.Capture(controller.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			if err := tx.Query(ctx, stmt, record).Run(); err != nil {
				return errors.Errorf("inserting controller SSH host key: %w", err)
			}
			return nil
		}))
	}
}

type controllerSSHHostKey struct {
	ID     string `db:"id"`
	SSHKey string `db:"ssh_key"`
}
