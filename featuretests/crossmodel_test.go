// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
)

type cmdCrossModelSuite struct {
	jujutesting.RepoSuite
}

func runOffer(c *gc.C, expectedError string, args ...string) {
	cmdArgs := append([]string{"offer"}, args...)
	context, err := runJujuCommand(c, cmdArgs...)
	if expectedError == "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, gc.NotNil)
		c.Assert(testing.Stderr(context), jc.Contains, expectedError)
	}
}

func (s *cmdCrossModelSuite) TestOfferEmpty(c *gc.C) {
	runOffer(c, "error: an offer must at least specify service endpoint\n")
}

func (s *cmdCrossModelSuite) TestOfferInvalidEndpoints(c *gc.C) {
	runOffer(c, `error: endpoints must conform to format "<service-name>:<endpoint-name>[,...]" `, "fluff")
}

func (s *cmdCrossModelSuite) TestOffer(c *gc.C) {
	_, err := runJujuCommand(c, "offer", "test:db")
	c.Assert(err, jc.ErrorIsNil)

	// TODO (anastasiamac 2015-11-2) test that the offer is persisted.
	// For now, this test only checks that no errors wre thrown...
}
