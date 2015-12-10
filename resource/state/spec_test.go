// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/state"
)

var _ = gc.Suite(&specSuite{})

type specSuite struct {
	testing.IsolationSuite

	stub *testing.Stub
	raw  *stubRawSpecState
}

func (s *specSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.raw = &stubRawSpecState{stub: s.stub}
}

func (s *specSuite) TestListResourceSpecsOkay(c *gc.C) {
	expected, meta := newSpecs(c, "spam", "eggs")
	s.raw.ReturnCharmMetadata = meta
	st := state.NewState(s.raw)

	specs, err := st.ListResourceSpecs("a-service")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(specs, jc.DeepEquals, expected)
	s.stub.CheckCallNames(c, "CharmMetadata")
	s.stub.CheckCall(c, 0, "CharmMetadata", "a-service")
}

func (s *specSuite) TestListResourceSpecsEmpty(c *gc.C) {
	s.raw.ReturnCharmMetadata = &charm.Meta{}
	st := state.NewState(s.raw)

	specs, err := st.ListResourceSpecs("a-service")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(specs, gc.HasLen, 0)
	s.stub.CheckCallNames(c, "CharmMetadata")
}

func (s *specSuite) TestListResourceSpecsCharmMetadataError(c *gc.C) {
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)
	_, meta := newSpecs(c, "spam", "eggs")
	s.raw.ReturnCharmMetadata = meta
	st := state.NewState(s.raw)

	_, err := st.ListResourceSpecs("a-service")

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "CharmMetadata")
}

func (s *specSuite) TestListResourceSpecsInvalidMetadata(c *gc.C) {
	_, meta := newSpecs(c, "spam", "eggs")
	spam := meta.Resources["spam"]
	spam.Info.Name = ""
	meta.Resources["spam"] = spam
	s.raw.ReturnCharmMetadata = meta
	st := state.NewState(s.raw)

	_, err := st.ListResourceSpecs("a-service")

	c.Check(err, gc.ErrorMatches, `.*invalid charm metadata.*`)
	s.stub.CheckCallNames(c, "CharmMetadata")
}

func newSpecs(c *gc.C, names ...string) ([]resource.Spec, *charm.Meta) {
	var specs []resource.Spec
	resources := make(map[string]charmresource.Resource)
	for _, name := range names {
		info := charmresource.Info{
			Name: name,
			Type: charmresource.TypeFile,
			Path: name + ".tgz",
		}

		spec := resource.Spec{
			Definition: info,
			Origin:     resource.OriginKindUpload,
			Revision:   resource.NoRevision,
		}
		specs = append(specs, spec)

		res := charmresource.Resource{
			Info: info,
		}
		resources[info.Name] = res
	}
	return specs, &charm.Meta{
		Name:        "a-charm",
		Summary:     "a charm...",
		Description: "a charm...",
		Resources:   resources,
	}
}

type stubRawSpecState struct {
	stub *testing.Stub

	ReturnCharmMetadata *charm.Meta
}

func (s *stubRawSpecState) CharmMetadata(serviceID string) (*charm.Meta, error) {
	s.stub.AddCall("CharmMetadata", serviceID)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnCharmMetadata, nil
}
