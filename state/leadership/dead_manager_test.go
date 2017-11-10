// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"time"

	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	"github.com/juju/juju/state/leadership"
)

type deadManagerSuite struct{}

var _ = gc.Suite(&deadManagerSuite{})

func (s *deadManagerSuite) TestDeadManager(c *gc.C) {
	deadManagerErr := errors.New("DeadManagerError")
	deadManager := leadership.NewDeadManager(deadManagerErr)

	err := deadManager.BlockUntilLeadershipReleased("foo")
	c.Assert(err, gc.ErrorMatches, "leadership manager stopped")

	err = deadManager.ClaimLeadership("foo", "foo/0", time.Minute)
	c.Assert(err, gc.ErrorMatches, "leadership manager stopped")

	token := deadManager.LeadershipCheck("foo", "foo/0")
	err = token.Check(nil)
	c.Assert(err, gc.ErrorMatches, "leadership manager stopped")

	err = deadManager.Wait()
	c.Assert(err, gc.Equals, deadManagerErr)
}
