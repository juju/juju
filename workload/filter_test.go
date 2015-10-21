// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
)

var _ = gc.Suite(&filterSuite{})

type filterSuite struct{}

func (s *filterSuite) newPayload(name string) workload.FullPayloadInfo {
	return workload.FullPayloadInfo{
		Payload: workload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: name,
				Type: "docker",
			},
			ID:     "id" + name,
			Status: "running",
			Tags:   []string{"a-tag"},
			Unit:   "a-service/0",
		},
		Machine: "1",
	}
}

func (s *filterSuite) TestFilterOkay(c *gc.C) {
	payloads := []workload.FullPayloadInfo{
		s.newPayload("spam"),
	}
	predicate := func(workload.FullPayloadInfo) bool {
		return true
	}
	matched := workload.Filter(payloads, predicate)

	c.Check(matched, jc.DeepEquals, payloads)
}

func (s *filterSuite) TestFilterMatchAll(c *gc.C) {
	payloads := []workload.FullPayloadInfo{
		s.newPayload("spam"),
		s.newPayload("eggs"),
	}
	predicate := func(workload.FullPayloadInfo) bool {
		return true
	}
	matched := workload.Filter(payloads, predicate)

	c.Check(matched, jc.DeepEquals, payloads)
}

func (s *filterSuite) TestFilterMatchNone(c *gc.C) {
	payloads := []workload.FullPayloadInfo{
		s.newPayload("spam"),
	}
	predicate := func(workload.FullPayloadInfo) bool {
		return false
	}
	matched := workload.Filter(payloads, predicate)

	c.Check(matched, gc.HasLen, 0)
}

func (s *filterSuite) TestFilterNoPayloads(c *gc.C) {
	predicate := func(workload.FullPayloadInfo) bool {
		return true
	}
	matched := workload.Filter(nil, predicate)

	c.Check(matched, gc.HasLen, 0)
}

func (s *filterSuite) TestFilterMatchPartial(c *gc.C) {
	payloads := []workload.FullPayloadInfo{
		s.newPayload("spam"),
		s.newPayload("eggs"),
	}
	predicate := func(p workload.FullPayloadInfo) bool {
		return p.Name == "spam"
	}
	matched := workload.Filter(payloads, predicate)

	c.Check(matched, jc.DeepEquals, payloads[:1])
}

func (s *filterSuite) TestFilterMultiMatch(c *gc.C) {
	payloads := []workload.FullPayloadInfo{
		s.newPayload("spam"),
		s.newPayload("eggs"),
	}
	predA := func(p workload.FullPayloadInfo) bool {
		return p.Name == "spam"
	}
	predB := func(p workload.FullPayloadInfo) bool {
		return p.Name == "eggs"
	}
	matched := workload.Filter(payloads, predA, predB)

	c.Check(matched, jc.DeepEquals, payloads)
}

func (s *filterSuite) TestFilterMultiMatchPartial(c *gc.C) {
	payloads := []workload.FullPayloadInfo{
		s.newPayload("spam"),
		s.newPayload("eggs"),
		s.newPayload("ham"),
	}
	predA := func(p workload.FullPayloadInfo) bool {
		return p.Name == "ham"
	}
	predB := func(p workload.FullPayloadInfo) bool {
		return p.Name == "spam"
	}
	matched := workload.Filter(payloads, predA, predB)

	c.Check(matched, jc.DeepEquals, []workload.FullPayloadInfo{
		s.newPayload("spam"),
		s.newPayload("ham"),
	})
}

func (s *filterSuite) TestBuildPredicatesForOkay(c *gc.C) {
	payload := workload.FullPayloadInfo{
		Payload: workload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "spam",
				Type: "docker",
			},
			ID:     "idspam",
			Status: "running",
			Tags:   []string{"tagA", "tagB"},
			Unit:   "a-service/0",
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
		"a-service/0",
		"1",
	}
	for _, pattern := range patterns {
		predicates, err := workload.BuildPredicatesFor([]string{
			pattern,
		})
		c.Assert(err, jc.ErrorIsNil)

		c.Check(predicates, gc.HasLen, 1)
		matched := predicates[0](payload)
		c.Check(matched, jc.IsTrue)
	}

	// Check a non-matching pattern.

	predicates, err := workload.BuildPredicatesFor([]string{
		"tagC",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(predicates, gc.HasLen, 1)
	matched := predicates[0](payload)
	c.Check(matched, jc.IsFalse)
}

func (s *filterSuite) TestBuildPredicatesForMulti(c *gc.C) {
	predicates, err := workload.BuildPredicatesFor([]string{
		"tagC",
		"spam",
		"1",
		"2",
		"idspam",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(predicates, gc.HasLen, 5)
	payload := s.newPayload("spam")
	var matches []bool
	for _, pred := range predicates {
		matched := pred(payload)
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
	payload := workload.FullPayloadInfo{
		Payload: workload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "spam",
				Type: "docker",
			},
			ID:     "idspam",
			Status: "running",
			Tags:   []string{"tagA", "tagB"},
			Unit:   "a-service/0",
		},
		Machine: "1",
	}
	patterns := []string{
		// match
		"spam",
		"docker",
		"idspam",
		"running",
		"tagA",
		"tagB",
		"a-service/0",
		"1",
		// no match
		"tagC",
		"2",
	}

	var matches []bool
	for _, pattern := range patterns {
		matched := workload.Match(payload, pattern)
		matches = append(matches, matched)
	}

	c.Check(matches, jc.DeepEquals, []bool{
		true,
		true,
		true,
		true,
		true,
		true,
		true,
		true,
		false,
		false,
	})
}
