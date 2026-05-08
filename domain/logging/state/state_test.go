// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	loggingerrors "github.com/juju/juju/domain/logging/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestSetLokiEndpoint(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.SetLokiEndpoint(c.Context(), "http://loki:3100/loki/api/v1/push")
	c.Assert(err, tc.ErrorIsNil)

	endpoint, err := st.GetLokiEndpoint(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(endpoint, tc.Equals, "http://loki:3100/loki/api/v1/push")
}

func (s *stateSuite) TestSetLokiEndpointReplacesExisting(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.SetLokiEndpoint(c.Context(), "http://old-loki:3100/loki/api/v1/push")
	c.Assert(err, tc.ErrorIsNil)

	err = st.SetLokiEndpoint(c.Context(), "http://new-loki:3100/loki/api/v1/push")
	c.Assert(err, tc.ErrorIsNil)

	endpoint, err := st.GetLokiEndpoint(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(endpoint, tc.Equals, "http://new-loki:3100/loki/api/v1/push")
}

func (s *stateSuite) TestGetLokiEndpointNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetLokiEndpoint(c.Context())
	c.Assert(err, tc.ErrorIs, loggingerrors.LokiEndpointNotFound)
}

func (s *stateSuite) TestDeleteLokiEndpoint(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.SetLokiEndpoint(c.Context(), "http://loki:3100/loki/api/v1/push")
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteLokiEndpoint(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.GetLokiEndpoint(c.Context())
	c.Assert(err, tc.ErrorIs, loggingerrors.LokiEndpointNotFound)
}

func (s *stateSuite) TestDeleteLokiEndpointWhenEmpty(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Delete when nothing is set should be a no-op.
	err := st.DeleteLokiEndpoint(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}
