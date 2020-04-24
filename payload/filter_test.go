// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payload_test

import (
	"github.com/juju/charm/v7"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/payload"
)

var _ = gc.Suite(&filterSuite{})

type filterSuite struct {
	testing.IsolationSuite
}

func (s *filterSuite) newPayload(name string) payload.FullPayloadInfo {
	return payload.FullPayloadInfo{
		Payload: payload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: name,
				Type: "docker",
			},
			ID:     "id" + name,
			Status: "running",
			Labels: []string{"a-tag"},
			Unit:   "a-application/0",
		},
		Machine: "1",
	}
}

func (s *filterSuite) TestFilterOkay(c *gc.C) {
	payloads := []payload.FullPayloadInfo{
		s.newPayload("spam"),
	}
	predicate := func(payload.FullPayloadInfo) bool {
		return true
	}
	matched := payload.Filter(payloads, predicate)

	c.Check(matched, jc.DeepEquals, payloads)
}

func (s *filterSuite) TestFilterMatchAll(c *gc.C) {
	payloads := []payload.FullPayloadInfo{
		s.newPayload("spam"),
		s.newPayload("eggs"),
	}
	predicate := func(payload.FullPayloadInfo) bool {
		return true
	}
	matched := payload.Filter(payloads, predicate)

	c.Check(matched, jc.DeepEquals, payloads)
}

func (s *filterSuite) TestFilterMatchNone(c *gc.C) {
	payloads := []payload.FullPayloadInfo{
		s.newPayload("spam"),
	}
	predicate := func(payload.FullPayloadInfo) bool {
		return false
	}
	matched := payload.Filter(payloads, predicate)

	c.Check(matched, gc.HasLen, 0)
}

func (s *filterSuite) TestFilterNoPayloads(c *gc.C) {
	predicate := func(payload.FullPayloadInfo) bool {
		return true
	}
	matched := payload.Filter(nil, predicate)

	c.Check(matched, gc.HasLen, 0)
}

func (s *filterSuite) TestFilterMatchPartial(c *gc.C) {
	payloads := []payload.FullPayloadInfo{
		s.newPayload("spam"),
		s.newPayload("eggs"),
	}
	predicate := func(p payload.FullPayloadInfo) bool {
		return p.Name == "spam"
	}
	matched := payload.Filter(payloads, predicate)

	c.Check(matched, jc.DeepEquals, payloads[:1])
}

func (s *filterSuite) TestFilterMultiMatch(c *gc.C) {
	payloads := []payload.FullPayloadInfo{
		s.newPayload("spam"),
		s.newPayload("eggs"),
	}
	predA := func(p payload.FullPayloadInfo) bool {
		return p.Name == "spam"
	}
	predB := func(p payload.FullPayloadInfo) bool {
		return p.Name == "eggs"
	}
	matched := payload.Filter(payloads, predA, predB)

	c.Check(matched, jc.DeepEquals, payloads)
}

func (s *filterSuite) TestFilterMultiMatchPartial(c *gc.C) {
	payloads := []payload.FullPayloadInfo{
		s.newPayload("spam"),
		s.newPayload("eggs"),
		s.newPayload("ham"),
	}
	predA := func(p payload.FullPayloadInfo) bool {
		return p.Name == "ham"
	}
	predB := func(p payload.FullPayloadInfo) bool {
		return p.Name == "spam"
	}
	matched := payload.Filter(payloads, predA, predB)

	c.Check(matched, jc.DeepEquals, []payload.FullPayloadInfo{
		s.newPayload("spam"),
		s.newPayload("ham"),
	})
}

func (s *filterSuite) TestBuildPredicatesForOkay(c *gc.C) {
	pl := payload.FullPayloadInfo{
		Payload: payload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "spam",
				Type: "docker",
			},
			ID:     "idspam",
			Status: "running",
			Labels: []string{"tagA", "tagB"},
			Unit:   "a-application/0",
		},
		Machine: "1",
	}

	// Check matching patterns.

	patterns := []string{
		"spam",
		"docker",
		"idspam",
		"running",
		"tagA",
		"tagB",
		"a-application/0",
		"1",
	}
	for _, pattern := range patterns {
		predicates, err := payload.BuildPredicatesFor([]string{
			pattern,
		})
		c.Assert(err, jc.ErrorIsNil)

		c.Check(predicates, gc.HasLen, 1)
		matched := predicates[0](pl)
		c.Check(matched, jc.IsTrue)
	}

	// Check a non-matching pattern.

	predicates, err := payload.BuildPredicatesFor([]string{
		"tagC",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(predicates, gc.HasLen, 1)
	matched := predicates[0](pl)
	c.Check(matched, jc.IsFalse)
}

func (s *filterSuite) TestBuildPredicatesForMulti(c *gc.C) {
	predicates, err := payload.BuildPredicatesFor([]string{
		"tagC",
		"spam",
		"1",
		"2",
		"idspam",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(predicates, gc.HasLen, 5)
	pl := s.newPayload("spam")
	var matches []bool
	for _, pred := range predicates {
		matched := pred(pl)
		matches = append(matches, matched)
	}
	c.Check(matches, jc.DeepEquals, []bool{
		false,
		true,
		true,
		false,
		true,
	})
}

func (s *filterSuite) TestMatch(c *gc.C) {
	pl := payload.FullPayloadInfo{
		Payload: payload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "spam",
				Type: "docker",
			},
			ID:     "idspam",
			Status: "running",
			Labels: []string{"tagA", "tagB"},
			Unit:   "a-application/0",
		},
		Machine: "1",
	}

	// match
	for _, pattern := range []string{
		"spam",
		"docker",
		"idspam",
		"running",
		"tagA",
		"tagB",
		"a-application/0",
		"1",
	} {
		c.Logf("check %q", pattern)
		matched := payload.Match(pl, pattern)
		c.Check(matched, jc.IsTrue)
	}

	// no match
	for _, pattern := range []string{
		"tagC",
		"2",
	} {
		c.Logf("check %q", pattern)
		matched := payload.Match(pl, pattern)
		c.Check(matched, jc.IsFalse)
	}
}
