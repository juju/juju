// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"slices"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	userstate "github.com/juju/juju/domain/access/state"
	"github.com/juju/juju/domain/keymanager"
	keymanagerstate "github.com/juju/juju/domain/keymanager/state"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modelstate "github.com/juju/juju/domain/model/state"
	modelstatetesting "github.com/juju/juju/domain/model/state/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/ssh"
)

type controllerStateSuite struct {
	schematesting.ControllerSuite

	modelUUID model.UUID
	userUUID  user.UUID
}

func TestControllerStateSuite(t *stdtesting.T) {
	tc.Run(t, &controllerStateSuite{})
}

var (
	controllerSSHKeys = `
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju-client-key
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju-system-key`
)

// ensureControllerConfigSSHKeys is responsible for injecting ssh keys into a
// controllers config with the key defined in [controller.SystemSSHKeys].
func (s *controllerStateSuite) ensureControllerConfigSSHKeys(c *tc.C, keys string) {
	stmt := `
INSERT INTO controller_config (key, value) VALUES(?, ?)
`

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, stmt, controller.SystemSSHKeys, keys)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func generatePublicKeys(c *tc.C, publicKeys []string) []keymanager.PublicKey {
	rval := make([]keymanager.PublicKey, 0, len(publicKeys))
	for _, pk := range publicKeys {
		parsedKey, err := ssh.ParsePublicKey(pk)
		c.Assert(err, tc.ErrorIsNil)

		rval = append(rval, keymanager.PublicKey{
			Comment:         parsedKey.Comment,
			FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
			Fingerprint:     parsedKey.Fingerprint(),
			Key:             pk,
		})
	}

	return rval
}

func (s *controllerStateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.SeedControllerUUID(c)

	s.modelUUID = modelstatetesting.CreateTestModel(c, s.TxnRunnerFactory(), "keys")

	model, err := modelstate.NewState(s.TxnRunnerFactory()).GetModel(
		c.Context(), s.modelUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	s.userUUID = model.Owner
}

// TestControllerConfigKeysEmpty ensures that if we ask for keys that do not
// exist in controller config no errors are returned an empty map is returned.
func (s *controllerStateSuite) TestControllerConfigKeysEmpty(c *tc.C) {
	kv, err := NewControllerState(s.TxnRunnerFactory()).GetControllerConfigKeys(
		c.Context(),
		[]string{"does-not-exist"},
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(len(kv), tc.Equals, 0)
}

// TestControllerConfigKeys is asserting the happy path that we can extract the
// system ssh keys from controller config when they exist.
func (s *controllerStateSuite) TestControllerConfigKeys(c *tc.C) {
	s.ensureControllerConfigSSHKeys(c, controllerSSHKeys)
	kv, err := NewControllerState(s.TxnRunnerFactory()).GetControllerConfigKeys(
		c.Context(),
		[]string{controller.SystemSSHKeys},
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(len(kv), tc.Equals, 1)
	c.Check(kv[controller.SystemSSHKeys], tc.Equals, controllerSSHKeys)
}

// TestGetUserAuthorizedKeysForModelNotFound is asserting that is we ask for
// keys on a model that doesn't exist we get back a [modelerrors.NotFound] error.
func (s *controllerStateSuite) TestGetUserAuthorizedKeysForModelNotFound(c *tc.C) {
	st := NewControllerState(s.TxnRunnerFactory())
	_, err := st.GetUserAuthorizedKeysForModel(c.Context(), modeltesting.GenModelUUID(c))
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestGetUserAuthorizedKeysForModel is asserting the happy path of getting all
// user authorized keys for a model. We purposefully setup multiple users on the
// model in this test to make this scenario more realisticlty.
func (s *controllerStateSuite) TestGetUserAuthorizedKeysForModel(c *tc.C) {
	kmSt := keymanagerstate.NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := kmSt.AddPublicKeysForUser(c.Context(), s.modelUUID, s.userUUID, keysToAdd[0:1])
	c.Check(err, tc.ErrorIsNil)

	secondUserId := usertesting.GenUserUUID(c)
	userSt := userstate.NewUserState(s.TxnRunnerFactory())
	err = userSt.AddUser(
		c.Context(),
		secondUserId,
		usertesting.GenNewName(c, "second"),
		"second",
		false,
		s.userUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	err = kmSt.AddPublicKeysForUser(c.Context(), s.modelUUID, secondUserId, keysToAdd[1:3])
	c.Check(err, tc.ErrorIsNil)

	st := NewControllerState(s.TxnRunnerFactory())
	keys, err := st.GetUserAuthorizedKeysForModel(c.Context(), s.modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(keys, tc.DeepEquals, testingPublicKeys)
}
