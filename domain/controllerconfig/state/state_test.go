// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	ctx "context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/controller"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/controllerconfig/bootstrap"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	jujutesting "github.com/juju/juju/internal/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = tc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)

	cfg := controller.Config{
		controller.ControllerUUIDKey: jujutesting.ControllerTag.Id(),
		controller.CACertKey:         jujutesting.CACert,
	}
	controllerModelUUID := coremodel.UUID(jujutesting.ModelTag.Id())
	err := bootstrap.InsertInitialControllerConfig(cfg, controllerModelUUID)(ctx.Background(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestControllerConfigRead(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	ctrlConfig := map[string]string{
		controller.ControllerUUIDKey:   jujutesting.ControllerTag.Id(),
		controller.CACertKey:           jujutesting.CACert,
		controller.AuditingEnabled:     "1",
		controller.AuditLogCaptureArgs: "0",
		controller.AuditLogMaxBackups:  "10",
		controller.PublicDNSAddress:    "controller.test.com:1234",
		controller.APIPortOpenDelay:    "100ms",
	}

	err := st.UpdateControllerConfig(ctx.Background(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, jc.ErrorIsNil)

	controllerConfig, err := st.ControllerConfig(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(controllerConfig, jc.DeepEquals, ctrlConfig)
}

func (s *stateSuite) TestControllerConfigReadWithoutData(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	controllerConfig, err := st.ControllerConfig(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)

	// This is set at bootstrap time.
	c.Check(controllerConfig, jc.DeepEquals, map[string]string{
		controller.ControllerUUIDKey: jujutesting.ControllerTag.Id(),
		controller.CACertKey:         jujutesting.CACert,
	})
}

func (s *stateSuite) TestControllerConfigUpdateTwice(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	ctrlConfig := map[string]string{
		controller.ControllerUUIDKey:   jujutesting.ControllerTag.Id(),
		controller.CACertKey:           jujutesting.CACert,
		controller.AuditingEnabled:     "1",
		controller.AuditLogCaptureArgs: "0",
		controller.AuditLogMaxBackups:  "10",
		controller.PublicDNSAddress:    "controller.test.com:1234",
		controller.APIPortOpenDelay:    "100ms",
	}

	err := st.UpdateControllerConfig(ctx.Background(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateControllerConfig(ctx.Background(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, jc.ErrorIsNil)

	controllerConfig, err := st.ControllerConfig(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(controllerConfig, jc.DeepEquals, ctrlConfig)
}

func (s *stateSuite) TestControllerConfigUpdate(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	ctrlConfig := map[string]string{
		controller.ControllerUUIDKey:   jujutesting.ControllerTag.Id(),
		controller.CACertKey:           jujutesting.CACert,
		controller.AuditingEnabled:     "1",
		controller.AuditLogCaptureArgs: "0",
		controller.AuditLogMaxBackups:  "10",
		controller.PublicDNSAddress:    "controller.test.com:1234",
		controller.APIPortOpenDelay:    "100ms",
	}

	err := st.UpdateControllerConfig(ctx.Background(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, jc.ErrorIsNil)

	ctrlConfig[controller.AuditLogMaxBackups] = "11"

	err = st.UpdateControllerConfig(ctx.Background(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, jc.ErrorIsNil)

	controllerConfig, err := st.ControllerConfig(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(controllerConfig, jc.DeepEquals, ctrlConfig)
}

func (s *stateSuite) TestControllerConfigUpdateTwiceWithDifferentControllerUUID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	ctrlConfig := map[string]string{
		controller.ControllerUUIDKey:   jujutesting.ControllerTag.Id(),
		controller.CACertKey:           jujutesting.CACert,
		controller.AuditingEnabled:     "1",
		controller.AuditLogCaptureArgs: "0",
		controller.AuditLogMaxBackups:  "10",
		controller.PublicDNSAddress:    "controller.test.com:1234",
		controller.APIPortOpenDelay:    "100ms",
	}

	err := st.UpdateControllerConfig(ctx.Background(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, jc.ErrorIsNil)

	// This is just ignored, the service layer will not allow this.

	ctrlConfig[controller.ControllerUUIDKey] = "new-controller-uuid"

	err = st.UpdateControllerConfig(ctx.Background(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestUpdateControllerConfigNewData(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.UpdateControllerConfig(ctx.Background(), map[string]string{
		controller.PublicDNSAddress: "controller.test.com:1234",
		controller.APIPortOpenDelay: "100ms",
	}, nil, alwaysValid)
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateControllerConfig(ctx.Background(), map[string]string{
		controller.AuditLogMaxBackups: "10",
	}, nil, alwaysValid)
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()

	// Check the controller record.
	row := db.QueryRow("SELECT value FROM controller_config WHERE key = ?", controller.AuditLogMaxBackups)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var auditLogMaxBackups string
	err = row.Scan(&auditLogMaxBackups)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(auditLogMaxBackups, tc.Equals, "10")

}

func (s *stateSuite) TestUpdateControllerUpsertAndReplace(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	ctrlConfig := map[string]string{
		controller.PublicDNSAddress: "controller.test.com:1234",
		controller.APIPortOpenDelay: "100ms",
	}

	// Initial values.
	err := st.UpdateControllerConfig(ctx.Background(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, jc.ErrorIsNil)

	// Now with different DNS address and API port open delay.
	ctrlConfig[controller.PublicDNSAddress] = "updated-controller.test.com:1234"
	ctrlConfig[controller.APIPortOpenDelay] = "200ms"

	err = st.UpdateControllerConfig(ctx.Background(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()

	// Check the DNS address.
	row := db.QueryRow("SELECT value FROM controller_config WHERE key = ?", controller.PublicDNSAddress)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var dnsAddress string
	err = row.Scan(&dnsAddress)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(dnsAddress, tc.Equals, "updated-controller.test.com:1234")

	// Check the API port open delay.
	row = db.QueryRow("SELECT value FROM controller_config WHERE key = ?", controller.APIPortOpenDelay)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var apiPortOpenDelay string
	err = row.Scan(&apiPortOpenDelay)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(apiPortOpenDelay, tc.Equals, "200ms")
}

func (s *stateSuite) TestControllerConfigRemove(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	ctrlConfig := map[string]string{
		controller.ControllerUUIDKey:   jujutesting.ControllerTag.Id(),
		controller.CACertKey:           jujutesting.CACert,
		controller.AuditingEnabled:     "1",
		controller.AuditLogCaptureArgs: "0",
		controller.AuditLogMaxBackups:  "10",
		controller.PublicDNSAddress:    "controller.test.com:1234",
		controller.APIPortOpenDelay:    "100ms",
	}

	err := st.UpdateControllerConfig(ctx.Background(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, jc.ErrorIsNil)

	ctrlConfig[controller.AuditLogMaxBackups] = "11"

	// Delete the values that are not in the map.

	delete(ctrlConfig, controller.APIPortOpenDelay)
	delete(ctrlConfig, controller.AuditLogCaptureArgs)

	err = st.UpdateControllerConfig(ctx.Background(), ctrlConfig, []string{
		controller.APIPortOpenDelay,
		controller.AuditLogCaptureArgs,
	}, alwaysValid)
	c.Assert(err, jc.ErrorIsNil)

	controllerConfig, err := st.ControllerConfig(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(controllerConfig, jc.DeepEquals, map[string]string{
		controller.ControllerUUIDKey:  jujutesting.ControllerTag.Id(),
		controller.CACertKey:          jujutesting.CACert,
		controller.AuditingEnabled:    "1",
		controller.AuditLogMaxBackups: "11",
		controller.PublicDNSAddress:   "controller.test.com:1234",
	})
}

func (s *stateSuite) TestControllerConfigRemoveWithAdditionalValues(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	ctrlConfig := map[string]string{
		controller.ControllerUUIDKey:   jujutesting.ControllerTag.Id(),
		controller.CACertKey:           jujutesting.CACert,
		controller.AuditingEnabled:     "1",
		controller.AuditLogCaptureArgs: "0",
		controller.AuditLogMaxBackups:  "10",
		controller.PublicDNSAddress:    "controller.test.com:1234",
		controller.APIPortOpenDelay:    "100ms",
	}

	err := st.UpdateControllerConfig(ctx.Background(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, jc.ErrorIsNil)

	ctrlConfig[controller.AuditLogMaxBackups] = "11"

	// Notice that we've asked for two values to be removed, but they're still
	// in the map. They should still be removed.

	err = st.UpdateControllerConfig(ctx.Background(), ctrlConfig, []string{
		controller.APIPortOpenDelay,
		controller.AuditLogCaptureArgs,
	}, alwaysValid)
	c.Assert(err, jc.ErrorIsNil)

	controllerConfig, err := st.ControllerConfig(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(controllerConfig, jc.DeepEquals, map[string]string{
		controller.ControllerUUIDKey:  jujutesting.ControllerTag.Id(),
		controller.CACertKey:          jujutesting.CACert,
		controller.AuditingEnabled:    "1",
		controller.AuditLogMaxBackups: "11",
		controller.PublicDNSAddress:   "controller.test.com:1234",
	})
}

func (s *stateSuite) TestUpdateControllerWithValidation(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	ctrlConfig := map[string]string{
		controller.PublicDNSAddress: "controller.test.com:1234",
		controller.APIPortOpenDelay: "100ms",
	}

	// Initial values.
	err := st.UpdateControllerConfig(ctx.Background(), ctrlConfig, nil, func(m map[string]string) error {
		return errors.Errorf("boom")
	})
	c.Assert(err, tc.ErrorMatches, `boom`)
}

func alwaysValid(_ map[string]string) error {
	return nil
}
