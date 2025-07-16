// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strconv"
	"testing"

	"github.com/juju/tc"

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

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)

	cfg := controller.Config{
		controller.ControllerUUIDKey: jujutesting.ControllerTag.Id(),
		controller.CACertKey:         jujutesting.CACert,
	}
	controllerModelUUID := coremodel.UUID(jujutesting.ModelTag.Id())
	err := bootstrap.InsertInitialControllerConfig(cfg, controllerModelUUID)(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)
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
	}

	err := st.UpdateControllerConfig(c.Context(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	controllerConfig, err := st.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerConfig, tc.DeepEquals, ctrlConfig)
}

func (s *stateSuite) TestControllerConfigReadWithoutData(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	controllerConfig, err := st.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// This is set at bootstrap time.
	c.Check(controllerConfig, tc.DeepEquals, map[string]string{
		controller.APIPort:           strconv.Itoa(controller.DefaultAPIPort),
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
	}

	err := st.UpdateControllerConfig(c.Context(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	err = st.UpdateControllerConfig(c.Context(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	controllerConfig, err := st.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerConfig, tc.DeepEquals, ctrlConfig)
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
	}

	err := st.UpdateControllerConfig(c.Context(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	ctrlConfig[controller.AuditLogMaxBackups] = "11"

	err = st.UpdateControllerConfig(c.Context(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	controllerConfig, err := st.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerConfig, tc.DeepEquals, ctrlConfig)
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
	}

	err := st.UpdateControllerConfig(c.Context(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	// This is just ignored, the service layer will not allow this.

	ctrlConfig[controller.ControllerUUIDKey] = "new-controller-uuid"

	err = st.UpdateControllerConfig(c.Context(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestUpdateControllerConfigNewData(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.UpdateControllerConfig(c.Context(), map[string]string{
		controller.PublicDNSAddress: "controller.test.com:1234",
	}, nil, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	err = st.UpdateControllerConfig(c.Context(), map[string]string{
		controller.AuditLogMaxBackups: "10",
	}, nil, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	db := s.DB()

	// Check the controller record.
	row := db.QueryRow("SELECT value FROM controller_config WHERE key = ?", controller.AuditLogMaxBackups)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var auditLogMaxBackups string
	err = row.Scan(&auditLogMaxBackups)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(auditLogMaxBackups, tc.Equals, "10")

}

func (s *stateSuite) TestUpdateControllerUpsertAndReplace(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	ctrlConfig := map[string]string{
		controller.PublicDNSAddress: "controller.test.com:1234",
	}

	// Initial values.
	err := st.UpdateControllerConfig(c.Context(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	// Now with different DNS address and API port open delay.
	ctrlConfig[controller.PublicDNSAddress] = "updated-controller.test.com:1234"

	err = st.UpdateControllerConfig(c.Context(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	db := s.DB()

	// Check the DNS address.
	row := db.QueryRow("SELECT value FROM controller_config WHERE key = ?", controller.PublicDNSAddress)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var dnsAddress string
	err = row.Scan(&dnsAddress)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(dnsAddress, tc.Equals, "updated-controller.test.com:1234")
}

func (s *stateSuite) TestUpdateControllerUpsertAndReplaceAPIPort(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	ctrlConfig := map[string]string{
		controller.APIPort: "1234",
	}

	// Initial values.
	err := st.UpdateControllerConfig(c.Context(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	cfg, err := st.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg[controller.APIPort], tc.Equals, "1234")

	ctrlConfig[controller.APIPort] = "5678"

	err = st.UpdateControllerConfig(c.Context(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	cfg, err = st.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg[controller.APIPort], tc.Equals, "5678")

	// Ensure that the API port is *not* in the controller config table.
	row := s.DB().QueryRow("SELECT COUNT(*) FROM controller_config WHERE key = ?", controller.APIPort)
	var count int
	err = row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
	c.Assert(row.Err(), tc.ErrorIsNil)
}

func (s *stateSuite) TestUpdateControllerRemoveAPIPort(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	ctrlConfig := map[string]string{
		controller.APIPort: "1234",
	}

	// Initial values.
	err := st.UpdateControllerConfig(c.Context(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	cfg, err := st.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg[controller.APIPort], tc.Equals, "1234")

	err = st.UpdateControllerConfig(c.Context(), ctrlConfig, []string{controller.APIPort}, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	cfg, err = st.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg[controller.APIPort], tc.Equals, "")

	// Ensure that the API port is *not* in the controller config table.
	row := s.DB().QueryRow("SELECT api_port FROM controller")
	var apiPort *string
	err = row.Scan(&apiPort)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(apiPort, tc.IsNil)
	c.Assert(row.Err(), tc.ErrorIsNil)
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
	}

	err := st.UpdateControllerConfig(c.Context(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	ctrlConfig[controller.AuditLogMaxBackups] = "11"

	// Delete the values that are not in the map.

	delete(ctrlConfig, controller.AuditLogCaptureArgs)

	err = st.UpdateControllerConfig(c.Context(), ctrlConfig, []string{
		controller.AuditLogCaptureArgs,
	}, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	controllerConfig, err := st.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerConfig, tc.DeepEquals, map[string]string{
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
	}

	err := st.UpdateControllerConfig(c.Context(), ctrlConfig, nil, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	ctrlConfig[controller.AuditLogMaxBackups] = "11"

	// Notice that we've asked for two values to be removed, but they're still
	// in the map. They should still be removed.

	err = st.UpdateControllerConfig(c.Context(), ctrlConfig, []string{
		controller.AuditLogCaptureArgs,
	}, alwaysValid)
	c.Assert(err, tc.ErrorIsNil)

	controllerConfig, err := st.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerConfig, tc.DeepEquals, map[string]string{
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
	}

	// Initial values.
	err := st.UpdateControllerConfig(c.Context(), ctrlConfig, nil, func(m map[string]string) error {
		return errors.Errorf("boom")
	})
	c.Assert(err, tc.ErrorMatches, `boom`)
}

func alwaysValid(_ map[string]string) error {
	return nil
}
