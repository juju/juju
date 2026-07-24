// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/internal/uuid"
)

// debugLogAuthorizerSuite exists to form a set of contract tests for the
// authorizer composition used by the /log endpoint. It exists to prevent
// a regression where controller machines were denied access to /log,
// breaking log transfer during model migration, while ensuring workload
// machines stay denied.
type debugLogAuthorizerSuite struct{}

// TestDebugLogAuthorizerSuite runs all of the tests that are apart of
// the [debugLogAuthorizerSuite].
func TestDebugLogAuthorizerSuite(t *testing.T) {
	tc.Run(t, &debugLogAuthorizerSuite{})
}

func (s *debugLogAuthorizerSuite) authorizer(c *tc.C) authentication.Authorizer {
	controllerUUID := tc.Must(c, uuid.NewUUID)
	return debugLogAuthorizer(controllerAdminAuthorizer{
		controllerTag: names.NewControllerTag(controllerUUID.String()),
	})
}

// TestControllerAgentAllowed tests that a controller agent is authorized
// for the /log endpoint.
func (s *debugLogAuthorizerSuite) TestControllerAgentAllowed(c *tc.C) {
	err := s.authorizer(c).Authorize(c.Context(), authentication.AuthInfo{
		Tag:        names.NewControllerAgentTag("0"),
		Controller: true,
	})
	c.Assert(err, tc.ErrorIsNil)
}

// TestControllerMachineAllowed tests that a machine agent for a controller
// machine is authorized for the /log endpoint. This is the regression guard
// for the migration-master's LOGTRANSFER phase, which streams the source
// model's logs through /log while authenticated as the controller machine
// agent.
func (s *debugLogAuthorizerSuite) TestControllerMachineAllowed(c *tc.C) {
	err := s.authorizer(c).Authorize(c.Context(), authentication.AuthInfo{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	})
	c.Assert(err, tc.ErrorIsNil)
}

// TestWorkloadMachineDenied tests that a machine agent for a workload
// (non-controller) machine is denied access to the /log endpoint. This
// guards the security boundary fixed in #22387: admitting all machine tags
// would reopen the cross-model log disclosure.
func (s *debugLogAuthorizerSuite) TestWorkloadMachineDenied(c *tc.C) {
	err := s.authorizer(c).Authorize(c.Context(), authentication.AuthInfo{
		Tag:        names.NewMachineTag("1"),
		Controller: false,
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

// TestOtherAgentTagsDenied tests that agent tags that are neither
// controller agents nor machines are denied access to the /log endpoint.
func (s *debugLogAuthorizerSuite) TestOtherAgentTagsDenied(c *tc.C) {
	authorizer := s.authorizer(c)
	for _, tag := range []names.Tag{
		names.NewApplicationTag("foo"),
		names.NewUnitTag("foo/0"),
	} {
		err := authorizer.Authorize(c.Context(), authentication.AuthInfo{
			Tag: tag,
		})
		c.Check(err, tc.ErrorIs, apiservererrors.ErrPerm)
	}
}
