// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/state/utils"
)

var _ = gc.Suite(&CharmSuite{})

type CharmSuite struct {
	testing.IsolationSuite

	stub *testing.Stub
}

func (s *CharmSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)

	s.stub = &testing.Stub{}
}

func (s *CharmSuite) TestCharmMetadata(c *gc.C) {
	st := &stubCharmState{stub: s.stub}
	expected := &charm.Meta{
		Name:        "a-charm",
		Summary:     "a charm...",
		Description: "a charm...",
	}
	st.ReturnMeta = expected

	meta, err := utils.TestingCharmMetadata(st, "a-service")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(meta, jc.DeepEquals, expected)
	s.stub.CheckCallNames(c, "Service", "Charm", "Meta")
	s.stub.CheckCall(c, 0, "Service", "a-service")
}

type stubCharmState struct {
	stub *testing.Stub

	ReturnMeta *charm.Meta
}

func (s *stubCharmState) Service(id string) (utils.CharmService, error) {
	s.stub.AddCall("Service", id)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s, nil
}

func (s *stubCharmState) Charm() (utils.Charm, error) {
	s.stub.AddCall("Charm")
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s, nil
}

func (s *stubCharmState) Meta() *charm.Meta {
	s.stub.AddCall("Meta")
	s.stub.PopNoErr()

	return s.ReturnMeta
}
