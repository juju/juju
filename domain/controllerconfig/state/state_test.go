// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	ctx "context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/database/testing"
)

type stateSuite struct {
	testing.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestUpdateControllerConfigNewData(c *gc.C) {
	st := NewState(testing.TrackedDBFactory(s.TrackedDB()))

	err := st.UpdateControllerConfig(ctx.Background(), jujucontroller.Config{
		jujucontroller.AuditingEnabled:     true,
		jujucontroller.AuditLogCaptureArgs: false,
		jujucontroller.AuditLogMaxBackups:  "10",
		jujucontroller.PublicDNSAddress:    "controller.test.com:1234",
		jujucontroller.APIPortOpenDelay:    "100ms",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()

	// Check the controller record.
	row := db.QueryRow("SELECT value FROM controller_config WHERE key = ?", jujucontroller.PublicDNSAddress)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var dnsAddress string
	err = row.Scan(&dnsAddress)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(dnsAddress, gc.Equals, "controller.test.com:1234")

}

func (s *stateSuite) TestUpdateExternalControllerUpsertAndReplace(c *gc.C) {
	st := NewState(testing.TrackedDBFactory(s.TrackedDB()))

	cc := jujucontroller.Config{
		jujucontroller.AuditingEnabled:     true,
		jujucontroller.AuditLogCaptureArgs: false,
		jujucontroller.AuditLogMaxBackups:  "10",
		jujucontroller.PublicDNSAddress:    "controller.test.com:1234",
		jujucontroller.APIPortOpenDelay:    "100ms",
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
