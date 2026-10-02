// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
)

// unitResourceAccessSuite exists to form a set of contract tests for
// [checkUnitResourceAccess].
type unitResourceAccessSuite struct{}

func TestUnitResourceAccessSuite(t *testing.T) {
	tc.Run(t, &unitResourceAccessSuite{})
}

// TestAllowed checks that an agent can fetch the resources of the unit it is
// authenticated for: the unit itself, or the application the unit belongs to.
func (s *unitResourceAccessSuite) TestAllowed(c *tc.C) {
	unitTag := names.NewUnitTag("wordpress/0")

	for _, authTag := range []names.Tag{
		names.NewUnitTag("wordpress/0"),
		names.NewApplicationTag("wordpress"),
	} {
		c.Logf("authenticated as %q", authTag)
		err := checkUnitResourceAccess(authTag, unitTag)
		c.Check(err, tc.ErrorIsNil)
	}
}

// TestRefused checks that the unit named in the request URL is not enough on
// its own. An agent asking for the resources of a unit it is not
// authenticated for is refused, including a peer unit of the same
// application, as is any entity that is not a unit or application agent.
func (s *unitResourceAccessSuite) TestRefused(c *tc.C) {
	unitTag := names.NewUnitTag("wordpress/0")

	for _, authTag := range []names.Tag{
		names.NewUnitTag("wordpress/1"),
		names.NewUnitTag("mysql/0"),
		names.NewApplicationTag("mysql"),
		names.NewApplicationTag("word"),
		names.NewMachineTag("0"),
		names.NewUserTag("admin"),
	} {
		c.Logf("authenticated as %q", authTag)
		err := checkUnitResourceAccess(authTag, unitTag)
		c.Check(err, tc.ErrorIs, apiservererrors.ErrPerm)
	}
}
