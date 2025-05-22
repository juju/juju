// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloadshookcontext_test

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	unitfacade "github.com/juju/juju/apiserver/facades/agent/payloadshookcontext"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type suite struct {
	testhelpers.IsolationSuite
}

func TestSuite(t *testing.T) {
	tc.Run(t, &suite{})
}

func (s *suite) TestTrack(c *tc.C) {
	a := unitfacade.NewUnitFacadeV1()
	args := params.TrackPayloadArgs{
		Payloads: []params.Payload{{
			Class: "idfoo",
			Type:  "type",
			ID:    "bar",
		}},
	}

	res, err := a.Track(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, params.PayloadResults{
		Results: []params.PayloadResult{{
			NotFound: true,
		}},
	})
}

func (s *suite) TestListOne(c *tc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	a := unitfacade.NewUnitFacadeV1()
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewPayloadTag(id).String(),
		}},
	}
	results, err := a.List(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, params.PayloadResults{
		Results: []params.PayloadResult{{
			Entity: params.Entity{
				Tag: names.NewPayloadTag(id).String(),
			},
			NotFound: true,
		}},
	})
}

func (s *suite) TestListAll(c *tc.C) {
	a := unitfacade.NewUnitFacadeV1()
	args := params.Entities{}
	results, err := a.List(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, params.PayloadResults{})
}

func (s *suite) TestLookUp(c *tc.C) {
	a := unitfacade.NewUnitFacadeV1()
	args := params.LookUpPayloadArgs{
		Args: []params.LookUpPayloadArg{{
			Name: "fooID",
			ID:   "bar",
		}},
	}
	res, err := a.LookUp(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, params.PayloadResults{
		Results: []params.PayloadResult{{
			NotFound: true,
		}},
	})
}

func (s *suite) TestSetStatus(c *tc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	a := unitfacade.NewUnitFacadeV1()
	args := params.SetPayloadStatusArgs{
		Args: []params.SetPayloadStatusArg{{
			Entity: params.Entity{
				Tag: names.NewPayloadTag(id).String(),
			},
		}},
	}
	res, err := a.SetStatus(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, params.PayloadResults{
		Results: []params.PayloadResult{{
			Entity: params.Entity{
				Tag: names.NewPayloadTag(id).String(),
			},
			NotFound: true,
		}},
	})
}

func (s *suite) TestUntrack(c *tc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"

	a := unitfacade.NewUnitFacadeV1()
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewPayloadTag(id).String(),
		}},
	}
	res, err := a.Untrack(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, params.PayloadResults{
		Results: []params.PayloadResult{{
			Entity: params.Entity{
				Tag: names.NewPayloadTag(id).String(),
			},
			NotFound: true,
		}},
	})
}

func (s *suite) TestUntrackEmptyID(c *tc.C) {
	a := unitfacade.NewUnitFacadeV1()
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: "",
		}},
	}
	res, err := a.Untrack(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, params.PayloadResults{
		Results: []params.PayloadResult{{
			Entity: params.Entity{
				Tag: "",
			},
			Error: nil,
		}},
	})
}

func (s *suite) TestUntrackNoIDs(c *tc.C) {
	a := unitfacade.NewUnitFacadeV1()
	args := params.Entities{
		Entities: []params.Entity{},
	}
	res, err := a.Untrack(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, params.PayloadResults{})
}
