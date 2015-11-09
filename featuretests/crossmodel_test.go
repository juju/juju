// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/crossmodel"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
)

type cmdCrossModelSuite struct {
	jujutesting.RepoSuite
}

func (s *cmdCrossModelSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	crossmodel.TempPlaceholder = make(map[names.ServiceTag]crossmodel.Offer)
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

func (s *cmdCrossModelSuite) TestOfferAndShow(c *gc.C) {
	_, err := runJujuCommand(c, "offer", "test:db", "local:/u/fred/prod/hosted-db2")
	c.Assert(err, jc.ErrorIsNil)

	context, err := runJujuCommand(c, "show-endpoints", "local:/u/fred/prod/hosted-db2")
	c.Assert(err, jc.ErrorIsNil)

	expected := `
SERVICE  INTERFACES  DESCRIPTION
test     db          

`[1:]
	c.Assert(testing.Stdout(context), gc.Matches, expected)
}
