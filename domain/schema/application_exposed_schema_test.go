// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
)

type applicationExposedSchemaSuite struct {
	schemaBaseSuite
}

func TestApplicationExposedSchemaSuite(t *testing.T) {
	tc.Run(t, &applicationExposedSchemaSuite{})
}

func (s *applicationExposedSchemaSuite) SetUpTest(c *tc.C) {
	s.schemaBaseSuite.SetUpTest(c)
	s.applyDDL(c, ModelDDL())

	s.assertExecSQL(c, `INSERT INTO space (uuid, name) VALUES ('space-uuid', 'space0')`)
	s.assertExecSQL(c, `
INSERT INTO charm (uuid, reference_name, source_id, architecture_id)
VALUES ('charm-uuid', 'foo', 0, 0)`)
	s.assertExecSQL(c, `
INSERT INTO charm_relation (uuid, charm_uuid, scope_id, role_id, name)
VALUES ('relation-uuid', 'charm-uuid', 0, 0, 'ep0')`)
	s.assertExecSQL(c, `
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES ('app-uuid', 'charm-uuid', 'foo', 0, ?)`, network.AlphaSpaceId)
	s.assertExecSQL(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid)
VALUES ('endpoint-uuid', 'app-uuid', 'relation-uuid')`)
}

// TestExposedEndpointRequiresEndpoint verifies that per-endpoint exposure must
// name its endpoint. Exposure on all endpoints has its own tables.
func (s *applicationExposedSchemaSuite) TestExposedEndpointRequiresEndpoint(c *tc.C) {
	s.assertExecSQLError(c, `
INSERT INTO application_exposed_endpoint_space (application_uuid, application_endpoint_uuid, space_uuid)
VALUES ('app-uuid', NULL, 'space-uuid')`,
		`NOT NULL constraint failed: application_exposed_endpoint_space.application_endpoint_uuid`)
	s.assertExecSQLError(c, `
INSERT INTO application_exposed_endpoint_cidr (application_uuid, application_endpoint_uuid, cidr)
VALUES ('app-uuid', NULL, '10.0.0.0/24')`,
		`NOT NULL constraint failed: application_exposed_endpoint_cidr.application_endpoint_uuid`)
}

// TestApplicationExposedRejectsDuplicates verifies that an application is
// exposed to a space or CIDR on all endpoints at most once. A NULL endpoint in
// the primary key previously allowed duplicate rows.
func (s *applicationExposedSchemaSuite) TestApplicationExposedRejectsDuplicates(c *tc.C) {
	s.assertExecSQL(c, `
INSERT INTO application_exposed_space (application_uuid, space_uuid)
VALUES ('app-uuid', 'space-uuid')`)
	s.assertExecSQLError(c, `
INSERT INTO application_exposed_space (application_uuid, space_uuid)
VALUES ('app-uuid', 'space-uuid')`,
		`UNIQUE constraint failed: application_exposed_space.application_uuid, application_exposed_space.space_uuid`)

	s.assertExecSQL(c, `
INSERT INTO application_exposed_cidr (application_uuid, cidr)
VALUES ('app-uuid', '10.0.0.0/24')`)
	s.assertExecSQLError(c, `
INSERT INTO application_exposed_cidr (application_uuid, cidr)
VALUES ('app-uuid', '10.0.0.0/24')`,
		`UNIQUE constraint failed: application_exposed_cidr.application_uuid, application_exposed_cidr.cidr`)
}

// TestExposedEndpointView verifies that the view combines application-wide and
// per-endpoint exposure, reporting application-wide rows with a NULL endpoint.
func (s *applicationExposedSchemaSuite) TestExposedEndpointView(c *tc.C) {
	s.assertExecSQL(c, `
INSERT INTO application_exposed_space (application_uuid, space_uuid)
VALUES ('app-uuid', 'space-uuid')`)
	s.assertExecSQL(c, `
INSERT INTO application_exposed_cidr (application_uuid, cidr)
VALUES ('app-uuid', '10.0.0.0/24')`)
	s.assertExecSQL(c, `
INSERT INTO application_exposed_endpoint_space (application_uuid, application_endpoint_uuid, space_uuid)
VALUES ('app-uuid', 'endpoint-uuid', 'space-uuid')`)
	s.assertExecSQL(c, `
INSERT INTO application_exposed_endpoint_cidr (application_uuid, application_endpoint_uuid, cidr)
VALUES ('app-uuid', 'endpoint-uuid', '10.0.1.0/24')`)

	rows, err := s.DB().QueryContext(c.Context(), `
SELECT application_uuid,
       COALESCE(application_endpoint_uuid, '*'),
       COALESCE(space_uuid, ''),
       COALESCE(cidr, '')
FROM   v_application_exposed_endpoint`)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var got [][4]string
	for rows.Next() {
		var row [4]string
		c.Assert(rows.Scan(&row[0], &row[1], &row[2], &row[3]), tc.ErrorIsNil)
		got = append(got, row)
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)
	c.Check(got, tc.SameContents, [][4]string{
		{"app-uuid", "*", "space-uuid", ""},
		{"app-uuid", "*", "", "10.0.0.0/24"},
		{"app-uuid", "endpoint-uuid", "space-uuid", ""},
		{"app-uuid", "endpoint-uuid", "", "10.0.1.0/24"},
	})
}
