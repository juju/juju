// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	ctx "context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	schematesting "github.com/juju/juju/domain/schema/testing"
	jujutesting "github.com/juju/juju/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestControllerConfigRead(c *gc.C) {
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

func (s *stateSuite) TestUpdateControllerConfigNewData(c *gc.C) {
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
	c.Check(auditLogMaxBackups, gc.Equals, "10")

}

func (s *stateSuite) TestUpdateControllerUpsertAndReplace(c *gc.C) {
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
	c.Check(dnsAddress, gc.Equals, "updated-controller.test.com:1234")

	// Check the API port open delay.
	row = db.QueryRow("SELECT value FROM controller_config WHERE key = ?", controller.APIPortOpenDelay)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var apiPortOpenDelay string
	err = row.Scan(&apiPortOpenDelay)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(apiPortOpenDelay, gc.Equals, "200ms")
}

func (s *stateSuite) TestUpdateControllerWithValidation(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	ctrlConfig := map[string]string{
		controller.PublicDNSAddress: "controller.test.com:1234",
		controller.APIPortOpenDelay: "100ms",
	}

	// Initial values.
	err := st.UpdateControllerConfig(ctx.Background(), ctrlConfig, nil, func(m map[string]string) error {
		return errors.Errorf("boom")
	})
	c.Assert(err, gc.ErrorMatches, `boom`)
}

func alwaysValid(_ map[string]string) error {
	return nil
}
