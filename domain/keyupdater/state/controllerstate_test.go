// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"slices"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
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

	modelId model.UUID
	userId  user.UUID
}

var (
	_ = gc.Suite(&controllerStateSuite{})

	controllerSSHKeys = `
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju-client-key
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju-system-key`
)

// ensureControllerConfigSSHKeys is responsible for injecting ssh keys into a
// controllers config with the key defined in [controller.SystemSSHKeys].
func (s *controllerStateSuite) ensureControllerConfigSSHKeys(c *gc.C, keys string) {
	stmt := `
INSERT INTO controller_config (key, value) VALUES(?, ?)
`

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, stmt, controller.SystemSSHKeys, keys)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

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

func (s *controllerStateSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.modelId = modelstatetesting.CreateTestModel(c, s.TxnRunnerFactory(), "keys")

	model, err := modelstate.NewState(s.TxnRunnerFactory()).GetModel(
		context.Background(), s.modelId,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.userId = model.Owner
}

// TestControllerConfigKeysEmpty ensures that if we ask for keys that do not
// exist in controller config no errors are returned an empty map is returned.
func (s *controllerStateSuite) TestControllerConfigKeysEmpty(c *gc.C) {
	kv, err := NewControllerState(s.TxnRunnerFactory()).GetControllerConfigKeys(
		context.Background(),
		[]string{"does-not-exist"},
	)
	c.Check(err, jc.ErrorIsNil)
	c.Check(len(kv), gc.Equals, 0)
}

// TestControllerConfigKeys is asserting the happy path that we can extract the
// system ssh keys from controller config when they exist.
func (s *controllerStateSuite) TestControllerConfigKeys(c *gc.C) {
	s.ensureControllerConfigSSHKeys(c, controllerSSHKeys)
	kv, err := NewControllerState(s.TxnRunnerFactory()).GetControllerConfigKeys(
		context.Background(),
		[]string{controller.SystemSSHKeys},
	)
	c.Check(err, jc.ErrorIsNil)
	c.Check(len(kv), gc.Equals, 1)
	c.Check(kv[controller.SystemSSHKeys], gc.Equals, controllerSSHKeys)
}

// TestGetUserAuthorizedKeysForModelNotFound is asserting that is we ask for
// keys on a model that doesn't exist we get back a [modelerrors.NotFound] error.
func (s *controllerStateSuite) TestGetUserAuthorizedKeysForModelNotFound(c *gc.C) {
	st := NewControllerState(s.TxnRunnerFactory())
	_, err := st.GetUserAuthorizedKeysForModel(context.Background(), modeltesting.GenModelUUID(c))
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestGetUserAuthorizedKeysForModel is asserting the happy path of getting all
// user authorized keys for a model. We purposefully setup multiple users on the
// model in this test to make this scenario more realisticlty.
func (s *controllerStateSuite) TestGetUserAuthorizedKeysForModel(c *gc.C) {
	kmSt := keymanagerstate.NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := kmSt.AddPublicKeysForUser(context.Background(), s.modelId, s.userId, keysToAdd[0:1])
	c.Check(err, jc.ErrorIsNil)

	secondUserId := usertesting.GenUserUUID(c)
	userSt := userstate.NewUserState(s.TxnRunnerFactory())
	err = userSt.AddUser(
		context.Background(),
		secondUserId,
		usertesting.GenNewName(c, "second"),
		"second",
		false,
		s.userId,
		permission.AccessSpec{
			Access: permission.AdminAccess,
			Target: permission.ID{
				ObjectType: permission.Model,
				Key:        s.modelId.String(),
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = kmSt.AddPublicKeysForUser(context.Background(), s.modelId, secondUserId, keysToAdd[1:3])
	c.Check(err, jc.ErrorIsNil)

	st := NewControllerState(s.TxnRunnerFactory())
	keys, err := st.GetUserAuthorizedKeysForModel(context.Background(), s.modelId)
	c.Assert(err, jc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(keys, jc.DeepEquals, testingPublicKeys)
}
