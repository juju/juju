// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"time"

	"github.com/juju/tc"
)

func (s *exportControllerStateSuiteV4_1_0) TestExportNullableBooleans(c *tc.C) {
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO flag (name, value, description) VALUES
    ('nullable-null', NULL, ''),
    ('nullable-false', FALSE, ''),
    ('nullable-true', TRUE, '')`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `
INSERT INTO logging_loki_config (uuid, endpoint)
VALUES ('loki-uuid', 'https://loki.example.com')`)
	c.Assert(err, tc.ErrorIsNil)

	for _, test := range []struct {
		name  string
		value any
		want  *bool
	}{
		{name: "null", value: nil},
		{name: "false", value: false, want: new(false)},
		{name: "true", value: true, want: new(true)},
	} {
		_, err := s.DB().ExecContext(c.Context(),
			`UPDATE logging_loki_config SET insecure_skip_verify = ?`, test.value)
		c.Assert(err, tc.ErrorIsNil)

		payload, err := NewState(s.TxnRunnerFactory()).Export(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(payload.LoggingLokiConfig, tc.HasLen, 1)
		if test.want == nil {
			c.Check(payload.LoggingLokiConfig[0].InsecureSkipVerify, tc.IsNil,
				tc.Commentf("%s", test.name))
		} else {
			c.Assert(payload.LoggingLokiConfig[0].InsecureSkipVerify, tc.NotNil,
				tc.Commentf("%s", test.name))
			c.Check(*payload.LoggingLokiConfig[0].InsecureSkipVerify, tc.Equals,
				*test.want, tc.Commentf("%s", test.name))
		}

		flags := make(map[string]*bool)
		for _, flag := range payload.Flag {
			flags[flag.Name] = flag.Value
		}
		_, found := flags["nullable-null"]
		c.Assert(found, tc.IsTrue)
		c.Check(flags["nullable-null"], tc.IsNil)
		c.Assert(flags["nullable-false"], tc.NotNil)
		c.Check(*flags["nullable-false"], tc.IsFalse)
		c.Assert(flags["nullable-true"], tc.NotNil)
		c.Check(*flags["nullable-true"], tc.IsTrue)
	}
}

func (s *exportControllerStateSuiteV4_1_0) TestExportNullableTimestamps(c *tc.C) {
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO lease (uuid, lease_type_id, start, expiry) VALUES
    ('lease-null', 0, NULL, NULL),
    ('lease-time', 0, '2026-01-02 03:04:05', '2026-01-02 04:04:05')`)
	c.Assert(err, tc.ErrorIsNil)

	payload, err := NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(payload.Lease, tc.HasLen, 2)

	for _, lease := range payload.Lease {
		switch lease.UUID {
		case "lease-null":
			c.Check(lease.Start, tc.IsNil)
			c.Check(lease.Expiry, tc.IsNil)
		case "lease-time":
			c.Assert(lease.Start, tc.NotNil)
			c.Assert(lease.Expiry, tc.NotNil)
			c.Check(*lease.Start, tc.Equals, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
			c.Check(*lease.Expiry, tc.Equals, time.Date(2026, 1, 2, 4, 4, 5, 0, time.UTC))
		default:
			c.Errorf("unexpected lease %q", lease.UUID)
		}
	}
}

func (s *exportControllerStateSuiteV4_1_0) TestExportOtherNullableColumns(c *tc.C) {
	for _, query := range []string{
		`INSERT INTO cloud (uuid, name, cloud_type_id, endpoint, skip_tls_verify)
VALUES ('cloud', 'cloud', 1, '', FALSE)`,
		`INSERT INTO user (uuid, name, external, created_by_uuid, created_at)
VALUES ('user', 'user', FALSE, 'user', '2026-01-01 00:00:00')`,
		`INSERT INTO cloud_credential (uuid, cloud_uuid, auth_type_id, owner_uuid, name, revoked, invalid)
VALUES ('null', 'cloud', 1, 'user', 'null', NULL, TRUE),
       ('false', 'cloud', 1, 'user', 'false', FALSE, NULL),
       ('true', 'cloud', 1, 'user', 'true', TRUE, FALSE)`,
		`INSERT INTO controller_api_address (controller_id, address, scope, is_agent)
VALUES ('0', 'null', 'public', NULL), ('0', 'false', 'public', FALSE),
       ('0', 'true', 'public', TRUE)`,
		`INSERT INTO model_migration_redirect (model_uuid, source_migration_uuid, target_controller_uuid,
    target_addresses, target_ca_cert, created_at, completed_at)
VALUES ('null', 'migration', 'target', '', '', '2026-01-01 00:00:00', NULL),
       ('set', 'migration', 'target', '', '', '2026-01-01 00:00:00', '2026-01-02 03:04:05')`,
		`INSERT INTO upgrade_info (uuid, previous_version, target_version, state_type_id)
VALUES ('null', '4.0.0', '4.1.0', 3), ('set', '4.0.0', '4.1.0', 3)`,
		`INSERT INTO upgrade_info_controller_node (uuid, controller_node_id, upgrade_info_uuid, node_upgrade_completed_at)
VALUES ('null', '0', 'null', NULL), ('set', '0', 'set', '2026-01-02 03:04:05')`,
	} {
		_, err := s.DB().ExecContext(c.Context(), query)
		c.Assert(err, tc.ErrorIsNil)
	}

	payload, err := NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	type credentialFlags struct {
		revoked *bool
		invalid *bool
	}
	credentials := make(map[string]credentialFlags)
	for _, credential := range payload.CloudCredential {
		credentials[credential.UUID] = credentialFlags{credential.Revoked, credential.Invalid}
	}
	c.Check(credentials, tc.DeepEquals, map[string]credentialFlags{
		"null":  {nil, new(true)},
		"false": {new(false), nil},
		"true":  {new(true), new(false)},
	})

	addresses := make(map[string]*bool)
	for _, address := range payload.ControllerApiAddress {
		addresses[address.Address] = address.IsAgent
	}
	c.Check(addresses, tc.DeepEquals, map[string]*bool{
		"null": nil, "false": new(false), "true": new(true),
	})

	completedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	redirects := make(map[string]*time.Time)
	for _, redirect := range payload.ModelMigrationRedirect {
		redirects[redirect.ModelUUID] = redirect.CompletedAt
	}
	c.Check(redirects, tc.DeepEquals, map[string]*time.Time{
		"null": nil, "set": &completedAt,
	})
	upgrades := make(map[string]*time.Time)
	for _, upgrade := range payload.UpgradeInfoControllerNode {
		upgrades[upgrade.UUID] = upgrade.NodeUpgradeCompletedAt
	}
	c.Check(upgrades, tc.DeepEquals, map[string]*time.Time{
		"null": nil, "set": &completedAt,
	})
}
