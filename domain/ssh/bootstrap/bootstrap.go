// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	domainssh "github.com/juju/juju/domain/ssh"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	pkissh "github.com/juju/juju/internal/pki/ssh"
)

// InsertInitialSSHServerHostKey inserts the controller jump host key into the
// controller database during bootstrap. The marshalled public host key is
// derived from the private key once here and stored alongside it, so it can be
// served to clients without re-parsing the private key on every retrieval.
func InsertInitialSSHServerHostKey(sshServerHostKey string) internaldatabase.BootstrapOpt {
	return func(ctx context.Context, controller, model database.TxnRunner) error {
		if sshServerHostKey == "" {
			return errors.Errorf("empty SSHServerHostKey").Add(coreerrors.NotValid)
		}

		algorithmTypeID, err := sshKeyAlgorithmTypeID(sshServerHostKey)
		if err != nil {
			return errors.Errorf("determining controller SSH host key algorithm: %w", err)
		}

		publicKey, err := pkissh.MarshalPublicKey([]byte(sshServerHostKey))
		if err != nil {
			return errors.Errorf("deriving controller SSH host public key: %w", err)
		}

		record := controllerSSHHostKey{
			ID:              domainssh.SSHServerHostKeyUUID,
			AlgorithmTypeID: algorithmTypeID,
			SSHKey:          sshServerHostKey,
			PublicKey:       publicKey,
		}
		stmt, err := sqlair.Prepare(`
INSERT INTO controller_ssh_host_key (id, algorithm_type_id, ssh_key, public_key)
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
	ID              string `db:"id"`
	AlgorithmTypeID int    `db:"algorithm_type_id"`
	SSHKey          string `db:"ssh_key"`
	PublicKey       []byte `db:"public_key"`
}

func sshKeyAlgorithmTypeID(sshKey string) (int, error) {
	algorithm, err := pkissh.PrivateKeyAlgorithm([]byte(sshKey))
	if err != nil {
		return 0, errors.Capture(err)
	}
	switch algorithm {
	case pkissh.AlgorithmRSA:
		return domainssh.SSHKeyAlgorithmTypeRSAID, nil
	case pkissh.AlgorithmECDSA256:
		return domainssh.SSHKeyAlgorithmTypeECDSA256ID, nil
	case pkissh.AlgorithmED25519:
		return domainssh.SSHKeyAlgorithmTypeED25519ID, nil
	default:
		return 0, errors.Errorf("unsupported SSH key algorithm %q", algorithm)
	}
}
