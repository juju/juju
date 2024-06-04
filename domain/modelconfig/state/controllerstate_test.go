// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremodeltesting "github.com/juju/juju/core/model/testing"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modeltesting "github.com/juju/juju/domain/model/state/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/secretbackend"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	secretbackendstate "github.com/juju/juju/domain/secretbackend/state"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type controllerStateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&controllerStateSuite{})

func (s *controllerStateSuite) createSecretBackend(c *gc.C) {
	backendSt := secretbackendstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	vaultBackendID := uuid.MustNewUUID().String()
	result, err := backendSt.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   vaultBackendID,
			Name: "my-backend",
		},
		BackendType: "vault",
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, vaultBackendID)
}

func (s *controllerStateSuite) TestSetModelSecretBackend(c *gc.C) {
	db := s.DB()

	modelUUID := modeltesting.CreateTestIAASModel(c, s.TxnRunnerFactory(), "foo")
	s.createSecretBackend(c)

	st := NewControllerState(s.TxnRunnerFactory())
	err := st.SetModelSecretBackend(context.Background(), modelUUID, "my-backend")
	c.Assert(err, gc.IsNil)

	var configuredBackend string
	row := db.QueryRow(`
SELECT sb.name
FROM model_secret_backend msb
JOIN secret_backend sb ON sb.uuid = msb.secret_backend_uuid
WHERE model_uuid = ?`[1:], modelUUID)
	err = row.Scan(&configuredBackend)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(configuredBackend, gc.Equals, "my-backend")
}

func (s *controllerStateSuite) TestSetModelSecretBackendNotFound(c *gc.C) {
	modelUUID := modeltesting.CreateTestIAASModel(c, s.TxnRunnerFactory(), "foo")
	s.createSecretBackend(c)

	st := NewControllerState(s.TxnRunnerFactory())
	err := st.SetModelSecretBackend(context.Background(), modelUUID, "some-backend")
	c.Assert(err, jc.ErrorIs, secretbackenderrors.NotFound)
	c.Assert(err, gc.ErrorMatches, `secret backend not found: "some-backend"`)
}

func (s *controllerStateSuite) TestSetModelSecretBackendModelNotFound(c *gc.C) {
	modeltesting.CreateTestIAASModel(c, s.TxnRunnerFactory(), "foo")
	s.createSecretBackend(c)

	coremodeltesting.GenModelUUID(c)
	st := NewControllerState(s.TxnRunnerFactory())
	err := st.SetModelSecretBackend(context.Background(), "some-uuid", "my-backend")
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
	c.Assert(err, gc.ErrorMatches, `model not found: model "some-uuid"`)
}

func (s *controllerStateSuite) TestGetModelSecretBackendName(c *gc.C) {
	modelUUID := modeltesting.CreateTestIAASModel(c, s.TxnRunnerFactory(), "foo")
	s.createSecretBackend(c)

	st := NewControllerState(s.TxnRunnerFactory())
	err := st.SetModelSecretBackend(context.Background(), modelUUID, "my-backend")
	c.Assert(err, gc.IsNil)

	name, err := st.GetModelSecretBackendName(context.Background(), modelUUID)
	c.Assert(err, gc.IsNil)
	c.Assert(name, gc.Equals, "my-backend")
}
