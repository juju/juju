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
