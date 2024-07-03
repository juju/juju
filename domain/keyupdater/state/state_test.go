// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"slices"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremachine "github.com/juju/juju/core/machine"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/keymanager"
	keymanagerstate "github.com/juju/juju/domain/keymanager/state"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/ssh"
)

type stateSuite struct {
	schematesting.ModelSuite

	machineName coremachine.Name
}

var _ = gc.Suite(&stateSuite{})

var (
	testingPublicKeys = []string{
		// ecdsa testing public key
		"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBG00bYFLb/sxPcmVRMg8NXZK/ldefElAkC9wD41vABdHZiSRvp+2y9BMNVYzE/FnzKObHtSvGRX65YQgRn7k5p0= juju1@example.com",

		// ed25519 testing public key
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju2@example.com",

		// rsa testing public key
		"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDvplNOK3UBpULZKvZf/I5JHci/DufpSxj8yR4yKE2grescJxu6754jPT3xztSeLGD31/oJApJZGkMUAMRenvDqIaq+taRfOUo/l19AlGZc+Edv4bTlJzZ1Lzwex1vvL1doaLb/f76IIUHClGUgIXRceQH1ovHiIWj6nGltuLanG8YTWxlzzK33yhitmZt142DmpX1VUVF5c/Hct6Rav5lKmwej1TDed1KmHzXVoTHEsmWhKsOK27ue5yTuq0GX6LrAYDucF+2MqZCsuddXsPAW1tj5GNZSR7RrKW5q1CI0G7k9gSomuCsRMlCJ3BqID/vUSs/0qOWg4he0HUsYKQSrXIhckuZu+jYP8B80MoXT50ftRidoG/zh/PugBdXTk46FloVClQopG5A2fbqrphADcUUbRUxZ2lWQN+OVHKfEsfV2b8L2aSqZUGlryfW1cirB5JCTDvtv7rUy9/ny9iKA+8tAyKSDF0I901RDDqKc9dSkrHCg2bLnJZDoiRoWczE= juju3@example.com",
	}
)

func generatePublicKeys(c *gc.C, publicKeys []string) []keymanager.PublicKey {
	rval := make([]keymanager.PublicKey, 0, len(publicKeys))
	for _, pk := range publicKeys {
		parsedKey, err := ssh.ParsePublicKey(pk)
		c.Assert(err, jc.ErrorIsNil)

		rval = append(rval, keymanager.PublicKey{
			Comment:         parsedKey.Comment,
			FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
			Fingerprint:     parsedKey.Fingerprint(),
			Key:             pk,
		})
	}

	return rval
}

// ensureNetNode inserts a row into the net_node table, mostly used as a foreign key for entries in
// other tables (e.g. machine)
func (s *stateSuite) ensureNetNode(c *gc.C, uuid string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
		INSERT INTO net_node (uuid)
		VALUES (?)`, uuid)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) ensureMachine(c *gc.C, name coremachine.Name, uuid string) {
	s.ensureNetNode(c, "node2")
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
		INSERT INTO machine (uuid, net_node_uuid, name, life_id)
		VALUES (?, "node2", ?, "0")`, uuid, name)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.machineName = coremachine.Name("0")
	s.ensureMachine(c, s.machineName, "123")
}

// TestAuthorisedKeysForUnknownMachine is assertint that if we ask for
// authorised keys for a machine that doesn't exist we get back a
// [machineerrors.NotFound] error.
func (s *stateSuite) TestAuthorisedKeysForUnknownMachine(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())
	_, err := state.AuthorisedKeysForMachine(context.Background(), coremachine.Name("100"))
	c.Check(err, jc.ErrorIs, machineerrors.NotFound)
}

// TestEmptyAuthorisedKeysForMachine tests that if there are no authorised keys
// for machine this does not produce an error.
func (s *stateSuite) TestEmptyAuthorisedKeysForMachine(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())
	keys, err := state.AuthorisedKeysForMachine(context.Background(), s.machineName)
	c.Check(err, jc.ErrorIsNil)
	c.Check(len(keys), gc.Equals, 0)
}

// TestAuthorisedKeysForMachine is asserting the happy path of fetching
// authorised keys for a given machine with no errors.
func (s *stateSuite) TestAuthorisedKeysForMachine(c *gc.C) {
	keyManagerState := keymanagerstate.NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)
	userID := usertesting.GenUserUUID(c)

	err := keyManagerState.AddPublicKeysForUser(context.Background(), userID, keysToAdd)
	c.Check(err, jc.ErrorIsNil)

	keys, err := NewState(s.TxnRunnerFactory()).AuthorisedKeysForMachine(
		context.Background(),
		s.machineName,
	)
	c.Check(err, jc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(keys, jc.DeepEquals, testingPublicKeys)
}

// TestAllPublicKeysQuery is testing the query from [State.AllPublicKeysQuery]
// to make sure that it is returning all public keys in the model.
func (s *stateSuite) TestAllPublicKeysQuery(c *gc.C) {
	keyManagerState := keymanagerstate.NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)
	userID := usertesting.GenUserUUID(c)

	err := keyManagerState.AddPublicKeysForUser(context.Background(), userID, keysToAdd)
	c.Check(err, jc.ErrorIsNil)

	state := NewState(s.TxnRunnerFactory())

	keys := []string{}
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, state.AllPublicKeysQuery())
		if err != nil {
			return err
		}

		defer rows.Close()
		var key string
		for rows.Next() {
			if err := rows.Scan(&key); err != nil {
				return err
			}

			keys = append(keys, key)
		}

		return rows.Err()
	})

	c.Check(err, jc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(keys, jc.DeepEquals, testingPublicKeys)
}
