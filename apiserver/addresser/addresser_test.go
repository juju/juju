// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser_test

import (
	"errors"

	gc "gopkg.in/check.v1"

	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/apiserver/addresser"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type AddresserSuite struct {
	coretesting.BaseSuite

	st         *mockState
	api        *addresser.AddresserAPI
	authoriser apiservertesting.FakeAuthorizer
	resources  *common.Resources
}

var _ = gc.Suite(&AddresserSuite{})

func (s *AddresserSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.authoriser = apiservertesting.FakeAuthorizer{
		EnvironManager: true,
	}
	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	s.st = NewMockState()
	addresser.PatchState(s, s.st)

	var err error
	s.api, err = addresser.NewAddresserAPI(nil, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *AddresserSuite) TestEnvironConfigSuccess(c *gc.C) {
	config := coretesting.EnvironConfig(c)
	s.st.SetConfig(c, config)

	result, err := s.api.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.EnvironConfigResult{
		Config: config.AllAttrs(),
	})

	s.st.CheckCallNames(c, "EnvironConfig")
}

func (s *AddresserSuite) TestEnvironConfigFailure(c *gc.C) {
	s.st.SetErrors(errors.New("ouch"))

	result, err := s.api.EnvironConfig()
	c.Assert(err, gc.ErrorMatches, "ouch")
	c.Assert(result, jc.DeepEquals, params.EnvironConfigResult{})

	s.st.CheckCallNames(c, "EnvironConfig")
}

func (s *AddresserSuite) TestLifeSuccess(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: "ipaddress-00000000-1111-2222-3333-0123456789ab"}},
	}
	result, err := s.api.Life(args)
	c.Assert(err, gc.IsNil)
	c.Assert(len(result.Results), gc.Equals, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].Life, gc.Equals, params.Alive)

	args = params.Entities{
		Entities: []params.Entity{
			{Tag: "ipaddress-00000000-1111-2222-3333-0123456789ab"},
			{Tag: "ipaddress-00000000-1111-2222-7777-0123456789ab"},
		},
	}
	result, err = s.api.Life(args)
	c.Assert(err, gc.IsNil)
	c.Assert(len(result.Results), gc.Equals, 2)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].Life, gc.Equals, params.Alive)
	c.Assert(result.Results[1].Error, gc.IsNil)
	c.Assert(result.Results[1].Life, gc.Equals, params.Dead)

	args = params.Entities{}
	result, err = s.api.Life(args)
	c.Assert(err, gc.IsNil)
	c.Assert(len(result.Results), gc.Equals, 0)
}

func (s *AddresserSuite) TestLifeFail(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: "ipaddress-00000000-1111-2222-9999-ffffffffffff"}},
	}
	result, err := s.api.Life(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, `IP address ipaddress-00000000-1111-2222-9999-ffffffffffff not found`)
}

func (s *AddresserSuite) TestRemoveSuccess(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: "ipaddress-00000000-1111-2222-6666-0123456789ab"}},
	}
	result, err := s.api.Remove(args)
	c.Assert(err, gc.IsNil)
	c.Assert(len(result.Results), gc.Equals, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

}

func (s *AddresserSuite) TestRemoveFail(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: "ipaddress-00000000-1111-2222-3333-0123456789ab"}},
	}
	result, err := s.api.Remove(args)
	c.Assert(err, gc.IsNil)
	c.Assert(len(result.Results), gc.Equals, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, `cannot remove entity "ipaddress-00000000-1111-2222-3333-0123456789ab": still alive`)

}

func (s *AddresserSuite) TestWatchIPAddresses(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	result, err := s.api.WatchIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.EntityWatchResult{
		EntityWatcherId: "1",
		Changes: []string{
			"ipaddress-00000000-1111-2222-3333-0123456789ab",
			"ipaddress-00000000-1111-2222-4444-0123456789ab",
			"ipaddress-00000000-1111-2222-5555-0123456789ab",
			"ipaddress-00000000-1111-2222-6666-0123456789ab",
			"ipaddress-00000000-1111-2222-7777-0123456789ab",
		},
		Error: nil,
	})

	// Verify the resource was registered and stop when done.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.st, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}
