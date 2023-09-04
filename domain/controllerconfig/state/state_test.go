// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	ctx "context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucontroller "github.com/juju/juju/controller"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestControllerConfigRead(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	cc := map[string]interface{}{
		jujucontroller.AuditingEnabled:     "1",
		jujucontroller.AuditLogCaptureArgs: "0",
		jujucontroller.AuditLogMaxBackups:  "10",
		jujucontroller.PublicDNSAddress:    "controller.test.com:1234",
		jujucontroller.APIPortOpenDelay:    "100ms",
	}

	err := st.UpdateControllerConfig(ctx.Background(), cc, nil)
	c.Assert(err, jc.ErrorIsNil)

	controllerConfig, err := st.ControllerConfig(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(controllerConfig, jc.DeepEquals, cc)

}

func (s *stateSuite) TestUpdateControllerConfigNewData(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.UpdateControllerConfig(ctx.Background(), jujucontroller.Config{
		jujucontroller.PublicDNSAddress: "controller.test.com:1234",
		jujucontroller.APIPortOpenDelay: "100ms",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateControllerConfig(ctx.Background(), jujucontroller.Config{
		jujucontroller.AuditLogMaxBackups: "10",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()

	// Check the controller record.
	row := db.QueryRow("SELECT value FROM controller_config WHERE key = ?", jujucontroller.AuditLogMaxBackups)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var auditLogMaxBackups string
	err = row.Scan(&auditLogMaxBackups)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(auditLogMaxBackups, gc.Equals, "10")

}

func (s *stateSuite) TestUpdateExternalControllerUpsertAndReplace(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	cc := jujucontroller.Config{
		jujucontroller.PublicDNSAddress: "controller.test.com:1234",
		jujucontroller.APIPortOpenDelay: "100ms",
	}

	// Initial values.
	err := st.UpdateControllerConfig(ctx.Background(), cc, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Now with different DNS address and API port open delay.
	cc[jujucontroller.PublicDNSAddress] = "updated-controller.test.com:1234"
	cc[jujucontroller.APIPortOpenDelay] = "200ms"

	err = st.UpdateControllerConfig(ctx.Background(), cc, nil)
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()

	// Check the DNS address.
	row := db.QueryRow("SELECT value FROM controller_config WHERE key = ?", jujucontroller.PublicDNSAddress)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var dnsAddress string
	err = row.Scan(&dnsAddress)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(dnsAddress, gc.Equals, "updated-controller.test.com:1234")

	// Check the API port open delay.
	row = db.QueryRow("SELECT value FROM controller_config WHERE key = ?", jujucontroller.APIPortOpenDelay)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var apiPortOpenDelay string
	err = row.Scan(&apiPortOpenDelay)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(apiPortOpenDelay, gc.Equals, "200ms")
}
