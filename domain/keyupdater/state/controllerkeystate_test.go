// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	gc "gopkg.in/check.v1"

	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/controller"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type controllerKeyStateSuite struct {
	schematesting.ControllerSuite
}

var (
	_ = gc.Suite(&controllerKeyStateSuite{})

	controllerSSHKeys = `
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju-client-key
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju-system-key`
)

// ensureControllerConfigSSHKeys is responsible for injecting ssh keys into a
// controllers config with the key defined in [controller.SystemSSHKeys].
func (s *controllerKeyStateSuite) ensureControllerConfigSSHKeys(c *gc.C, keys string) {
	stmt := `
INSERT INTO controller_config (key, value) VALUES(?, ?)
`

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, stmt, controller.SystemSSHKeys, keys)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

// TestControllerConfigKeysEmpty ensures that if we ask for keys that do not
// exist in controller config no errors are returned an empty map is returned.
func (s *controllerKeyStateSuite) TestControllerConfigKeysEmpty(c *gc.C) {
	kv, err := NewControllerKeyState(s.TxnRunnerFactory()).GetControllerConfigKeys(
		context.Background(),
		[]string{"does-not-exist"},
	)
	c.Check(err, jc.ErrorIsNil)
	c.Check(len(kv), gc.Equals, 0)
}

// TestControllerConfigKeys is asserting the happy path that we can extract the
// system ssh keys from controller config when they exist.
func (s *controllerKeyStateSuite) TestControllerConfigKeys(c *gc.C) {
	s.ensureControllerConfigSSHKeys(c, controllerSSHKeys)
	kv, err := NewControllerKeyState(s.TxnRunnerFactory()).GetControllerConfigKeys(
		context.Background(),
		[]string{controller.SystemSSHKeys},
	)
	c.Check(err, jc.ErrorIsNil)
	c.Check(len(kv), gc.Equals, 1)
	c.Check(kv[controller.SystemSSHKeys], gc.Equals, controllerSSHKeys)
}
