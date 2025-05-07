// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/rpc/params"
)

type charmOriginSuite struct{}

var _ = tc.Suite(&charmOriginSuite{})

func (s *charmOriginSuite) TestValidateCharmOriginSuccessCharmHub(c *tc.C) {
	err := ValidateCharmOrigin(&params.CharmOrigin{
		Hash:   "myHash",
		ID:     "myID",
		Source: "charm-hub",
	})
	c.Assert(err, tc.Not(jc.ErrorIs), errors.BadRequest)
}

func (s *charmOriginSuite) TestValidateCharmOriginSuccessLocal(c *tc.C) {
	err := ValidateCharmOrigin(&params.CharmOrigin{Source: "local"})
	c.Assert(err, tc.Not(jc.ErrorIs), errors.BadRequest)
}

func (s *charmOriginSuite) TestValidateCharmOriginNil(c *tc.C) {
	err := ValidateCharmOrigin(nil)
	c.Assert(err, jc.ErrorIs, errors.BadRequest)
}

func (s *charmOriginSuite) TestValidateCharmOriginNilSource(c *tc.C) {
	err := ValidateCharmOrigin(&params.CharmOrigin{Source: ""})
	c.Assert(err, jc.ErrorIs, errors.BadRequest)
}

func (s *charmOriginSuite) TestValidateCharmOriginBadSource(c *tc.C) {
	err := ValidateCharmOrigin(&params.CharmOrigin{Source: "charm-store"})
	c.Assert(err, jc.ErrorIs, errors.BadRequest)
}

func (s *charmOriginSuite) TestValidateCharmOriginCharmHubIDNoHash(c *tc.C) {
	err := ValidateCharmOrigin(&params.CharmOrigin{
		ID:     "myID",
		Source: "charm-hub",
	})
	c.Assert(err, jc.ErrorIs, errors.BadRequest)
}

func (s *charmOriginSuite) TestValidateCharmOriginCharmHubHashNoID(c *tc.C) {
	err := ValidateCharmOrigin(&params.CharmOrigin{
		Hash:   "myHash",
		Source: "charm-hub",
	})
	c.Assert(err, jc.ErrorIs, errors.BadRequest)
}
