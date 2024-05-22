// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"
	"crypto/sha512"
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

var _ = gc.Suite(&StagedResourceSuite{})

type StagedResourceSuite struct {
	ConnSuite
}

func (s *StagedResourceSuite) assertActivate(c *gc.C, inc state.IncrementCharmModifiedVersionType) {
	ch := s.ConnSuite.AddTestingCharm(c, "starsay")
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "starsay",
		Charm: ch,
	})

	res := s.State.Resources(state.NewObjectStore(c, s.State.ModelUUID()))
	spam := newResourceFromCharm(ch, "store-resource")

	data := "spamspamspam"
	spam.Size = int64(len(data))
	sha384hash := sha512.New384()
	sha384hash.Write([]byte(data))
	fp := fmt.Sprintf("%x", sha384hash.Sum(nil))
	var err error
	spam.Fingerprint, err = charmresource.ParseFingerprint(fp)
	c.Assert(err, jc.ErrorIsNil)

	_, err = res.SetResource("starsay", spam.Username, spam.Resource, bytes.NewBufferString(data), inc)
	c.Assert(err, jc.ErrorIsNil)

	staged := state.StagedResourceForTest(c, s.State, spam)
	err = staged.Activate(inc)
	c.Assert(err, jc.ErrorIsNil)

	_, err = res.GetResource("starsay", "store-resource")
	c.Assert(err, jc.ErrorIsNil)

	err = app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	if inc {
		c.Assert(app.CharmModifiedVersion(), gc.Equals, 2)
	} else {
		c.Assert(app.CharmModifiedVersion(), gc.Equals, 0)
	}
}

func (s *StagedResourceSuite) TestActivateIncrement(c *gc.C) {
	s.assertActivate(c, state.IncrementCharmModifiedVersion)
}

func (s *StagedResourceSuite) TestActivateNoIncrement(c *gc.C) {
	s.assertActivate(c, state.DoNotIncrementCharmModifiedVersion)
}
