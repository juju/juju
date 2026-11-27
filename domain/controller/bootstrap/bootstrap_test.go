// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	coremodel "github.com/juju/juju/core/model"
	controllerconfigbootstrap "github.com/juju/juju/domain/controllerconfig/bootstrap"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/testing"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite
}

func TestBootstrapSuite(t *stdtesting.T) {
	tc.Run(t, &bootstrapSuite{})
}

func (s *bootstrapSuite) TestInsertInitialController(c *tc.C) {
	cfg := controller.Config{
		controller.CACertKey:         testing.CACert,
		controller.ControllerUUIDKey: testing.ControllerTag.Id(),
	}
	modelUUID, err := coremodel.NewUUID()
	c.Assert(err, tc.IsNil)
	err = controllerconfigbootstrap.InsertInitialControllerConfig(cfg, modelUUID)(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	err = InsertInitialController("a", "b", "c", "d")(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	var cert, pk, caPK, si string
	row := s.DB().QueryRow("SELECT cert, private_key, ca_private_key, system_identity FROM controller")
	c.Assert(row.Scan(&cert, &pk, &caPK, &si), tc.ErrorIsNil)

	c.Check(cert, tc.Equals, "a")
	c.Check(pk, tc.Equals, "b")
	c.Check(caPK, tc.Equals, "c")
	c.Check(si, tc.Equals, "d")
}

// TestInsertInitialControllerService checks that the Kubernetes controller
// Service bootstrap configuration is stored with comma-joined external IPs.
func (s *bootstrapSuite) TestInsertInitialControllerService(c *tc.C) {
	modelUUID := tc.Must0(c, coremodel.NewUUID)
	controllerUUID := s.SeedControllerTable(c, modelUUID)

	err := InsertInitialControllerService(
		controllerUUID, "external", "controller.example.com",
		[]string{"1.2.3.4", "5.6.7.8"},
	)(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	var (
		gotUUID, gotType, gotName, gotIPs string
		count                             int
	)
	row := s.DB().QueryRow("SELECT COUNT(*) FROM k8s_controller_service")
	c.Assert(row.Scan(&count), tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)

	row = s.DB().QueryRow(`
SELECT controller_uuid, service_type, external_name, external_ips
FROM k8s_controller_service`)
	c.Assert(row.Scan(&gotUUID, &gotType, &gotName, &gotIPs), tc.ErrorIsNil)

	c.Check(gotUUID, tc.Equals, controllerUUID)
	c.Check(gotType, tc.Equals, "external")
	c.Check(gotName, tc.Equals, "controller.example.com")
	c.Check(gotIPs, tc.Equals, "1.2.3.4,5.6.7.8")
}

// TestInsertInitialControllerServiceEmptyValues checks that unset bootstrap
// service options are stored as empty strings (meaning provider defaults).
func (s *bootstrapSuite) TestInsertInitialControllerServiceEmptyValues(c *tc.C) {
	modelUUID := tc.Must0(c, coremodel.NewUUID)
	controllerUUID := s.SeedControllerTable(c, modelUUID)

	err := InsertInitialControllerService(controllerUUID, "", "", nil)(
		c.Context(), s.TxnRunner(), s.NoopTxnRunner(),
	)
	c.Assert(err, tc.ErrorIsNil)

	var gotType, gotName, gotIPs string
	row := s.DB().QueryRow(`
SELECT service_type, external_name, external_ips
FROM k8s_controller_service`)
	c.Assert(row.Scan(&gotType, &gotName, &gotIPs), tc.ErrorIsNil)

	c.Check(gotType, tc.Equals, "")
	c.Check(gotName, tc.Equals, "")
	c.Check(gotIPs, tc.Equals, "")
}

// TestInsertInitialControllerServiceDuplicate checks that the controller_uuid
// primary key prevents inserting more than one row for the controller.
func (s *bootstrapSuite) TestInsertInitialControllerServiceDuplicate(c *tc.C) {
	modelUUID := tc.Must0(c, coremodel.NewUUID)
	controllerUUID := s.SeedControllerTable(c, modelUUID)

	opt := InsertInitialControllerService(controllerUUID, "cluster", "", nil)
	err := opt(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	err = opt(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Check(err, tc.ErrorMatches, `.*UNIQUE constraint failed: k8s_controller_service.controller_uuid.*`)
}

// TestInsertInitialControllerServiceWithoutController checks that the foreign
// key rejects inserting a service row before the parent controller row exists.
func (s *bootstrapSuite) TestInsertInitialControllerServiceWithoutController(c *tc.C) {
	err := InsertInitialControllerService(
		testing.ControllerTag.Id(), "cluster", "", nil,
	)(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Check(err, tc.ErrorMatches, `.*FOREIGN KEY constraint failed.*`)
}
