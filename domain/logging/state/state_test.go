// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/logging"
	loggingerrors "github.com/juju/juju/domain/logging/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestSetLokiConfig(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.SetLokiConfig(c.Context(), "some-uuid-1", logging.LokiConfig{
		Endpoint:      "http://loki:3100/loki/api/v1/push",
		CACertificate: "ca-cert",
	})
	c.Assert(err, tc.ErrorIsNil)

	config, err := st.GetLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config.Endpoint, tc.Equals, "http://loki:3100/loki/api/v1/push")
	c.Check(config.CACertificate, tc.Equals, "ca-cert")
}

func (s *stateSuite) TestSetLokiConfigReplacesExisting(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.SetLokiConfig(c.Context(), "some-uuid-1", logging.LokiConfig{
		Endpoint:      "http://old-loki:3100/loki/api/v1/push",
		CACertificate: "old-ca",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = st.SetLokiConfig(c.Context(), "some-uuid-2", logging.LokiConfig{
		Endpoint:      "http://new-loki:3100/loki/api/v1/push",
		CACertificate: "new-ca",
	})
	c.Assert(err, tc.ErrorIsNil)

	config, err := st.GetLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config.Endpoint, tc.Equals, "http://new-loki:3100/loki/api/v1/push")
	c.Check(config.CACertificate, tc.Equals, "new-ca")
}

func (s *stateSuite) TestGetLokiConfigNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIs, loggingerrors.LokiConfigNotFound)
}

func (s *stateSuite) TestDeleteLokiConfig(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.SetLokiConfig(c.Context(), "some-uuid-1", logging.LokiConfig{
		Endpoint:      "http://loki:3100/loki/api/v1/push",
		CACertificate: "ca-cert",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.GetLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIs, loggingerrors.LokiConfigNotFound)
}

func (s *stateSuite) TestDeleteLokiConfigWhenEmpty(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Delete when nothing is set should be a no-op.
	err := st.DeleteLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}
