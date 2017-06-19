// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type unitPayloadSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&unitPayloadSuite{})

func (s *unitPayloadSuite) createTestUnit(c *gc.C, t string, apiCaller basetesting.APICallerFunc) *uniter.Unit {
	tag := names.NewUnitTag(t)
	st := uniter.NewState(apiCaller, tag)
	return uniter.CreateUnit(st, tag)
}

func (s *unitPayloadSuite) TestTrackPayloads(c *gc.C) {
	args := []params.TrackPayloadParams{
		params.TrackPayloadParams{
			Class:  "class",
			Type:   "type",
			ID:     "id",
			Status: "running",
			Labels: []string{},
		},
	}
	expected := params.TrackPayloadsParams{Payloads: args}

	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		// This functionality is available from v6.
		c.Check(version, jc.GreaterThan, 5)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "TrackPayloads")
		c.Assert(arg, gc.DeepEquals, expected)
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "yoink"},
			}},
		}
		return nil
	})
	u := s.createTestUnit(c, "mysql/0", apiCaller)
	err := u.TrackPayloads(args)
	c.Assert(err, gc.ErrorMatches, "yoink")
}

func (s *unitPayloadSuite) TestTrackPayloadsError(c *gc.C) {
	args := []params.TrackPayloadParams{
		params.TrackPayloadParams{
			Class:  "class",
			Type:   "type",
			ID:     "id",
			Status: "running",
			Labels: []string{},
		},
	}
	expected := params.TrackPayloadsParams{Payloads: args}

	var called bool
	msg := "yoink"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		// This functionality is available from v6.
		c.Check(version, jc.GreaterThan, 5)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "TrackPayloads")
		c.Assert(arg, gc.DeepEquals, expected)
		called = true

		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		return errors.New(msg)
	})

	u := s.createTestUnit(c, "mysql/0", apiCaller)
	err := u.TrackPayloads(args)
	c.Assert(err, gc.ErrorMatches, msg)
	c.Assert(called, jc.IsTrue)
}

func (s *unitPayloadSuite) TestUntrackPayloads(c *gc.C) {
	args := []params.UntrackPayloadParams{
		params.UntrackPayloadParams{
			Class: "class",
			ID:    "id",
		},
	}
	expected := params.UntrackPayloadsParams{Payloads: args}

	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		// This functionality is available from v6.
		c.Check(version, jc.GreaterThan, 5)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "UntrackPayloads")
		c.Assert(arg, gc.DeepEquals, expected)
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "yoink"},
			}},
		}
		return nil
	})
	u := s.createTestUnit(c, "mysql/0", apiCaller)
	err := u.UntrackPayloads(args)
	c.Assert(err, gc.ErrorMatches, "yoink")
}

func (s *unitPayloadSuite) TestUntrackPayloadsError(c *gc.C) {
	args := []params.UntrackPayloadParams{
		params.UntrackPayloadParams{
			Class: "class",
			ID:    "id",
		},
	}
	expected := params.UntrackPayloadsParams{Payloads: args}

	var called bool
	msg := "yoink"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		// This functionality is available from v6.
		c.Check(version, jc.GreaterThan, 5)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "UntrackPayloads")
		c.Assert(arg, gc.DeepEquals, expected)
		called = true

		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		return errors.New(msg)
	})

	u := s.createTestUnit(c, "mysql/0", apiCaller)
	err := u.UntrackPayloads(args)
	c.Assert(err, gc.ErrorMatches, msg)
	c.Assert(called, jc.IsTrue)
}

func (s *unitPayloadSuite) TestSetPayloadsStatus(c *gc.C) {
	args := []params.PayloadStatusParams{
		params.PayloadStatusParams{
			Class:  "class",
			ID:     "id",
			Status: "running",
		},
	}
	expected := params.PayloadsStatusParams{Payloads: args}

	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		// This functionality is available from v6.
		c.Check(version, jc.GreaterThan, 5)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "SetPayloadsStatus")
		c.Assert(arg, gc.DeepEquals, expected)
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "yoink"},
			}},
		}
		return nil
	})
	u := s.createTestUnit(c, "mysql/0", apiCaller)
	err := u.SetPayloadsStatus(args)
	c.Assert(err, gc.ErrorMatches, "yoink")
}

func (s *unitPayloadSuite) TestSetPayloadsStatusError(c *gc.C) {
	args := []params.PayloadStatusParams{
		params.PayloadStatusParams{
			Class:  "class",
			ID:     "id",
			Status: "running",
		},
	}
	expected := params.PayloadsStatusParams{Payloads: args}

	var called bool
	msg := "yoink"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		// This functionality is available from v6.
		c.Check(version, jc.GreaterThan, 5)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "SetPayloadsStatus")
		c.Assert(arg, gc.DeepEquals, expected)
		called = true

		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		return errors.New(msg)
	})

	u := s.createTestUnit(c, "mysql/0", apiCaller)
	err := u.SetPayloadsStatus(args)
	c.Assert(err, gc.ErrorMatches, msg)
	c.Assert(called, jc.IsTrue)
}
