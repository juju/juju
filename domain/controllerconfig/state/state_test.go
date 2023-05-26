// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	ctx "context"
	"github.com/juju/juju/controller"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/database/testing"
)

type stateSuite struct {
	testing.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestUpdateControllerConfigNewData(c *gc.C) {
	st := NewState(testing.TrackedDBFactory(s.TrackedDB()))

	err := st.UpdateControllerConfig(ctx.Background(), map[string]interface{}{
		controller.AuditingEnabled:     true,
		controller.AuditLogCaptureArgs: false,
		controller.AuditLogMaxBackups:  "10",
		controller.PublicDNSAddress:    "controller.test.com:1234",
		controller.APIPortOpenDelay:    "100ms",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()

	// Check the controller record.
	row := db.QueryRow("SELECT value FROM controller_config WHERE key = ?", controller.PublicDNSAddress)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var dnsAddress string
	err = row.Scan(&dnsAddress)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(dnsAddress, gc.Equals, "controller.test.com:1234")

}

//func (s *stateSuite) TestUpdateExternalControllerUpsertAndReplace(c *gc.C) {
//	st := NewState(testing.TrackedDBFactory(s.TrackedDB()))
//
//	ecUUID := utils.MustNewUUID().String()
//	ec := crossmodel.ControllerInfo{
//		ControllerTag: names.NewControllerTag(ecUUID),
//		Alias:         "new-external-controller",
//		Addrs:         []string{"10.10.10.10", "192.168.0.9"},
//		CACert:        "random-cert-string",
//	}
//
//	// Initial values.
//	err := st.UpdateExternalController(ctx.Background(), ec, nil)
//	c.Assert(err, jc.ErrorIsNil)
//
//	// Now with different alias and addresses.
//	ec.Alias = "updated-external-controller"
//	ec.Addrs = []string{"10.10.10.10", "192.168.0.10"}
//
//	err = st.UpdateExternalController(ctx.Background(), ec, nil)
//	c.Assert(err, jc.ErrorIsNil)
//
//	db := s.DB()
//
//	// Check the controller record.
//	row := db.QueryRow("SELECT alias FROM external_controller WHERE uuid = ?", ecUUID)
//	c.Assert(row.Err(), jc.ErrorIsNil)
//
//	var alias string
//	err = row.Scan(&alias)
//	c.Assert(err, jc.ErrorIsNil)
//	c.Check(alias, gc.Equals, "updated-external-controller")
//
//	// Addresses should have one preserved and one replaced.
//	rows, err := db.Query("SELECT address FROM external_controller_address WHERE controller_uuid = ?", ecUUID)
//	c.Assert(err, jc.ErrorIsNil)
//
//	addrs := set.NewStrings()
//	for rows.Next() {
//		var addr string
//		err := rows.Scan(&addr)
//		c.Assert(err, jc.ErrorIsNil)
//		addrs.Add(addr)
//	}
//	c.Check(addrs.Values(), gc.HasLen, 2)
//	c.Check(addrs.Contains("10.10.10.10"), jc.IsTrue)
//	c.Check(addrs.Contains("192.168.0.10"), jc.IsTrue)
//}
