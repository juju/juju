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

func (s *stateSuite) TestSetLokiConfigInsecureSkipVerifyTrue(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	boolVal := true
	err := st.SetLokiConfig(c.Context(), "some-uuid-1", logging.LokiConfig{
		Endpoint:           "http://loki:3100/loki/api/v1/push",
		CACertificate:      "ca-cert",
		InsecureSkipVerify: &boolVal,
	})
	c.Assert(err, tc.ErrorIsNil)

	config, err := st.GetLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config.Endpoint, tc.Equals, "http://loki:3100/loki/api/v1/push")
	c.Check(config.CACertificate, tc.Equals, "ca-cert")
	c.Assert(config.InsecureSkipVerify, tc.NotNil)
	c.Check(*config.InsecureSkipVerify, tc.Equals, true)
}

func (s *stateSuite) TestSetLokiConfigInsecureSkipVerifyFalse(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	boolVal := false
	err := st.SetLokiConfig(c.Context(), "some-uuid-2", logging.LokiConfig{
		Endpoint:           "http://loki:3100/loki/api/v1/push",
		CACertificate:      "ca-cert",
		InsecureSkipVerify: &boolVal,
	})
	c.Assert(err, tc.ErrorIsNil)

	config, err := st.GetLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config.Endpoint, tc.Equals, "http://loki:3100/loki/api/v1/push")
	c.Check(config.CACertificate, tc.Equals, "ca-cert")
	c.Assert(config.InsecureSkipVerify, tc.NotNil)
	c.Check(*config.InsecureSkipVerify, tc.Equals, false)
}

func (s *stateSuite) TestSetLokiConfigInsecureSkipVerifyNil(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.SetLokiConfig(c.Context(), "some-uuid-3", logging.LokiConfig{
		Endpoint:           "http://loki:3100/loki/api/v1/push",
		CACertificate:      "ca-cert",
		InsecureSkipVerify: nil,
	})
	c.Assert(err, tc.ErrorIsNil)

	config, err := st.GetLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config.Endpoint, tc.Equals, "http://loki:3100/loki/api/v1/push")
	c.Check(config.CACertificate, tc.Equals, "ca-cert")
	c.Assert(config.InsecureSkipVerify, tc.IsNil)
}

func (s *stateSuite) TestSetLokiConfigReplacesInsecureSkipVerify(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	boolTrue := true
	err := st.SetLokiConfig(c.Context(), "some-uuid-1", logging.LokiConfig{
		Endpoint:           "http://old-loki:3100/loki/api/v1/push",
		CACertificate:      "old-ca",
		InsecureSkipVerify: &boolTrue,
	})
	c.Assert(err, tc.ErrorIsNil)

	boolFalse := false
	err = st.SetLokiConfig(c.Context(), "some-uuid-2", logging.LokiConfig{
		Endpoint:           "http://new-loki:3100/loki/api/v1/push",
		CACertificate:      "new-ca",
		InsecureSkipVerify: &boolFalse,
	})
	c.Assert(err, tc.ErrorIsNil)

	config, err := st.GetLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config.Endpoint, tc.Equals, "http://new-loki:3100/loki/api/v1/push")
	c.Check(config.CACertificate, tc.Equals, "new-ca")
	c.Check(*config.InsecureSkipVerify, tc.Equals, false)

	err = st.DeleteLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.GetLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIs, loggingerrors.LokiConfigNotFound)
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

func (s *stateSuite) TestIsLokiEnabledTrue(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.SetLokiConfig(c.Context(), "some-uuid-1", logging.LokiConfig{
		Endpoint:      "http://loki:3100/loki/api/v1/push",
		CACertificate: "ca-cert",
	})
	c.Assert(err, tc.ErrorIsNil)

	enabled, err := st.IsLokiEnabled(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(enabled, tc.IsTrue)
}

func (s *stateSuite) TestIsLokiEnabledFalse(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	enabled, err := st.IsLokiEnabled(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(enabled, tc.IsFalse)
}

func (s *stateSuite) TestIsLokiEnabledAfterDelete(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.SetLokiConfig(c.Context(), "some-uuid-1", logging.LokiConfig{
		Endpoint:      "http://loki:3100/loki/api/v1/push",
		CACertificate: "ca-cert",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	enabled, err := st.IsLokiEnabled(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(enabled, tc.IsFalse)
}
