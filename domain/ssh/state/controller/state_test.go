// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"

	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/user"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainssh "github.com/juju/juju/domain/ssh"
	sshbootstrap "github.com/juju/juju/domain/ssh/bootstrap"
	sshcontrollerstate "github.com/juju/juju/domain/ssh/state/controller"
	jujutesting "github.com/juju/juju/internal/testing"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

func TestStateSuite(t *stdtesting.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestGetSSHServerHostKeyMissing(c *tc.C) {
	st := sshcontrollerstate.NewState(txRunnerFactory(s.ControllerTxnRunner()))

	key, err := st.GetSSHServerHostKey(c.Context())
	c.Check(key, tc.Equals, "")
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

func (s *stateSuite) TestGetSSHServerHostKeyExisting(c *tc.C) {
	err := sshbootstrap.InsertInitialSSHServerHostKey(jujutesting.SSHServerHostKey)(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	st := sshcontrollerstate.NewState(txRunnerFactory(s.ControllerTxnRunner()))

	key, err := st.GetSSHServerHostKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, jujutesting.SSHServerHostKey)

	var (
		storedID        string
		algorithmTypeID int
	)
	row := s.DB().QueryRow(`SELECT id, algorithm_type_id FROM controller_ssh_host_key`)
	c.Assert(row.Scan(&storedID, &algorithmTypeID), tc.ErrorIsNil)
	c.Check(storedID, tc.Equals, domainssh.SSHServerHostKeyUUID)
	c.Check(algorithmTypeID, tc.Equals, domainssh.SSHKeyAlgorithmTypeED25519ID)
}

func (s *stateSuite) TestGetSSHServerHostPublicKeyMissing(c *tc.C) {
	st := sshcontrollerstate.NewState(txRunnerFactory(s.ControllerTxnRunner()))

	key, err := st.GetSSHServerHostPublicKey(c.Context())
	c.Check(key, tc.IsNil)
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

func (s *stateSuite) TestGetSSHServerHostPublicKeyExisting(c *tc.C) {
	err := sshbootstrap.InsertInitialSSHServerHostKey(jujutesting.SSHServerHostKey)(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	st := sshcontrollerstate.NewState(txRunnerFactory(s.ControllerTxnRunner()))

	// Derive the expected public key from the same private key that bootstrap
	// stored, so we can verify the stored value matches.
	signer, err := gossh.ParsePrivateKey([]byte(jujutesting.SSHServerHostKey))
	c.Assert(err, tc.ErrorIsNil)
	want := signer.PublicKey().Marshal()

	got, err := st.GetSSHServerHostPublicKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, want)
}

// TestGetSSHServerHostPublicKeyEmpty checks that a row with an empty
// public_key column returns an empty byte slice without error. This can
// happen if a row was inserted without deriving the public key.
func (s *stateSuite) TestGetSSHServerHostPublicKeyEmpty(c *tc.C) {
	// Insert a row directly with an empty public_key.
	_, err := s.DB().Exec(
		`INSERT INTO controller_ssh_host_key (id, algorithm_type_id, ssh_key, public_key) VALUES (?, ?, ?, x'')`,
		domainssh.SSHServerHostKeyUUID, domainssh.SSHKeyAlgorithmTypeED25519ID, "dummy-key",
	)
	c.Assert(err, tc.ErrorIsNil)

	st := sshcontrollerstate.NewState(txRunnerFactory(s.ControllerTxnRunner()))

	got, err := st.GetSSHServerHostPublicKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, []byte{})
}

func (s *stateSuite) TestGetPublicKeysForUserInModel(c *tc.C) {
	modelUUID := internaluuid.MustNewUUID().String()
	otherModelUUID := internaluuid.MustNewUUID().String()
	s.addModel(c, modelUUID, "test-model")
	s.addModel(c, otherModelUUID, "other-model")
	userUUID := s.addUser(c, "alice")
	keyID := s.addUserPublicKey(c, userUUID, "ssh-ed25519 AAAAtest-key")
	otherKeyID := s.addUserPublicKey(c, userUUID, "ssh-ed25519 AAAAother-key")

	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO model_authorized_keys (model_uuid, user_public_ssh_key_id)
VALUES (?, ?), (?, ?)`, modelUUID, keyID, otherModelUUID, otherKeyID)
	c.Assert(err, tc.ErrorIsNil)

	keys, err := sshcontrollerstate.NewState(txRunnerFactory(s.ControllerTxnRunner())).GetPublicKeysForUserInModel(c.Context(), modelUUID, "alice")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(keys, tc.DeepEquals, []coressh.PublicKey{{Fingerprint: "ssh-ed25519 AAAAtest-key"}})
}

func (s *stateSuite) TestGetPublicKeysForUserIncludesFingerprint(c *tc.C) {
	userUUID := s.addUser(c, "alice")
	s.addUserPublicKey(c, userUUID, "ssh-ed25519 AAAAtest-key")
	_, err := s.DB().ExecContext(c.Context(), `
	INSERT INTO user_authentication (user_uuid, disabled)
VALUES (?, FALSE)`, userUUID)
	c.Assert(err, tc.ErrorIsNil)

	username, err := user.NewName("alice")
	c.Assert(err, tc.ErrorIsNil)
	keys, err := sshcontrollerstate.NewState(txRunnerFactory(s.ControllerTxnRunner())).GetPublicKeysForUser(c.Context(), username)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(keys, tc.DeepEquals, []coressh.PublicKey{{
		Comment:     "test-key",
		Fingerprint: "ssh-ed25519 AAAAtest-key",
		Key:         "ssh-ed25519 AAAAtest-key",
	}})
}

func (s *stateSuite) TestGetPublicKeysForUserInModelMissingModel(c *tc.C) {
	s.addUser(c, "alice")

	keys, err := sshcontrollerstate.NewState(txRunnerFactory(s.ControllerTxnRunner())).GetPublicKeysForUserInModel(c.Context(), internaluuid.MustNewUUID().String(), "alice")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(keys, tc.HasLen, 0)
}

func (s *stateSuite) TestGetPublicKeysForUserInModelMissingUser(c *tc.C) {
	modelUUID := internaluuid.MustNewUUID().String()
	s.addModel(c, modelUUID, "test-model")

	keys, err := sshcontrollerstate.NewState(txRunnerFactory(s.ControllerTxnRunner())).GetPublicKeysForUserInModel(c.Context(), modelUUID, "alice")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(keys, tc.HasLen, 0)
}

func (s *stateSuite) addModel(c *tc.C, uuid, name string) {
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO cloud (uuid, name, cloud_type_id, endpoint, skip_tls_verify)
VALUES (?, ?, 0, '', FALSE)`, "cloud-"+uuid, "cloud-"+name)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(c.Context(), `
INSERT INTO model (uuid, activated, cloud_uuid, model_type_id, life_id, name, qualifier)
VALUES (?, TRUE, ?, 0, 0, ?, 'prod')`, uuid, "cloud-"+uuid, name)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) addUser(c *tc.C, username string) string {
	userUUID := internaluuid.MustNewUUID().String()
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO user (uuid, name, display_name, external, removed, created_by_uuid, created_at)
VALUES (?, ?, ?, FALSE, FALSE, ?, ?)`, userUUID, username, username, userUUID, time.Now())
	c.Assert(err, tc.ErrorIsNil)
	return userUUID
}

func (s *stateSuite) addUserPublicKey(c *tc.C, userUUID, key string) int64 {
	var keyID int64
	err := s.DB().QueryRowContext(c.Context(), `
INSERT INTO user_public_ssh_key (comment, fingerprint_hash_algorithm_id, fingerprint, public_key, user_uuid)
VALUES ('test-key', 1, ?, ?, ?) RETURNING id`, key, key, userUUID).Scan(&keyID)
	c.Assert(err, tc.ErrorIsNil)
	return keyID
}

func txRunnerFactory(runner coredatabase.TxnRunner) coredatabase.TxnRunnerFactory {
	return func(context.Context) (coredatabase.TxnRunner, error) {
		return runner, nil
	}
}
