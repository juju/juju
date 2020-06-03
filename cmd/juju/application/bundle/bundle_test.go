// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/golang/mock/gomock"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application/bundle/mocks"
)

type buildModelRepSuite struct {
	modelExtractor *mocks.MockModelExtractor
}

var _ = gc.Suite(&buildModelRepSuite{})

func (s *buildModelRepSuite) TestBuildModelRepresentationEmptyModel(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyGetAnnotations()
	s.expectEmptyGetConfig()
	s.expectEmptyGetConstraints()
	s.expectEmptySequences()

	status := &params.FullStatus{
		Model: params.ModelStatusInfo{
			Name: "default",
		},
	}
	machines := map[string]string{}

	obtainedModel, err := BuildModelRepresentation(status, s.modelExtractor, false, machines)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("%s", spew.Sdump(obtainedModel))
	c.Assert(obtainedModel.Applications, gc.HasLen, 0)
	c.Assert(obtainedModel.Machines, gc.HasLen, 0)
	c.Assert(obtainedModel.Relations, gc.HasLen, 0)
	c.Assert(obtainedModel.Sequence, gc.HasLen, 0)
	c.Assert(obtainedModel.MachineMap, gc.HasLen, 0)
}

func (s *buildModelRepSuite) TestBuildModelRepresentationUseExistingMachines(c *gc.C) {
	s.testBuildModelRepresentationUseExistingMachines(c, true)
}

func (s *buildModelRepSuite) TestBuildModelRepresentationDoNotUseExistingMachines(c *gc.C) {
	s.testBuildModelRepresentationUseExistingMachines(c, false)
}

func (s *buildModelRepSuite) testBuildModelRepresentationUseExistingMachines(c *gc.C, use bool) {
	defer s.setupMocks(c).Finish()
	s.expectGetAnnotations(c, []string{"machine-0", "machine-1", "machine-2", "machine-3"})
	s.expectEmptyGetConfig()
	s.expectEmptyGetConstraints()
	s.expectEmptySequences()

	status := &params.FullStatus{
		Model: params.ModelStatusInfo{
			Name: "default",
		},
		Machines: map[string]params.MachineStatus{
			"0": {Series: "bionic"},
			"1": {Series: "bionic"},
			"2": {Series: "bionic"},
			"3": {Series: "bionic"},
		},
	}
	machines := map[string]string{
		"0": "1",
		"1": "3",
	}

	obtainedModel, err := BuildModelRepresentation(status, s.modelExtractor, use, machines)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("%s", spew.Sdump(obtainedModel))
	c.Assert(obtainedModel.Applications, gc.HasLen, 0)
	c.Assert(obtainedModel.Machines, gc.HasLen, 4)
	c.Assert(obtainedModel.Relations, gc.HasLen, 0)
	c.Assert(obtainedModel.Sequence, gc.HasLen, 0)
	if use {
		c.Assert(obtainedModel.MachineMap, gc.DeepEquals, map[string]string{"0": "1", "1": "3", "2": "2", "3": "3"})

	} else {
		c.Assert(obtainedModel.MachineMap, gc.DeepEquals, map[string]string{"0": "1", "1": "3"})
	}
}

func (s *buildModelRepSuite) TestBuildModelRepresentationApplicationsWithSubordinate(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectGetAnnotations(c, []string{"machine-0", "machine-1", "application-wordpress", "application-sub"})
	s.expectGetConfigWordpress()
	s.expectGetConstraintsWordpress()
	s.expectEmptySequences()

	status := &params.FullStatus{
		Model: params.ModelStatusInfo{
			Name: "default",
		},
		Machines: map[string]params.MachineStatus{
			"0": {Series: "bionic"},
			"1": {Series: "bionic"},
		},
		Applications: map[string]params.ApplicationStatus{
			"wordpress": {
				Series: "bionic",
				Life:   life.Alive,
				Units: map[string]params.UnitStatus{
					"0": {Machine: "0"},
				},
			},
			"sub": {
				Series:        "bionic",
				Life:          life.Alive,
				SubordinateTo: []string{"wordpress"},
			},
		},
	}
	machines := map[string]string{}

	obtainedModel, err := BuildModelRepresentation(status, s.modelExtractor, false, machines)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("%s", spew.Sdump(obtainedModel))
	c.Assert(obtainedModel.Applications, gc.HasLen, 2)
	_, ok := obtainedModel.Applications["wordpress"]
	c.Assert(ok, jc.IsTrue)
	_, ok = obtainedModel.Applications["sub"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(obtainedModel.Machines, gc.HasLen, 2)
	c.Assert(obtainedModel.Relations, gc.HasLen, 0)
	c.Assert(obtainedModel.Sequence, gc.HasLen, 0)
	c.Assert(obtainedModel.MachineMap, gc.HasLen, 0)
}

func (s *buildModelRepSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelExtractor = mocks.NewMockModelExtractor(ctrl)
	return ctrl
}

func (s *buildModelRepSuite) expectEmptyGetAnnotations() {
	s.modelExtractor.EXPECT().GetAnnotations(gomock.Nil()).Return(nil, nil)
}

func (s *buildModelRepSuite) expectGetAnnotations(c *gc.C, tags []string) {
	matcher := stringSliceMatcher{c: c, expected: tags}
	result := make([]params.AnnotationsGetResult, len(tags))
	for i, tag := range tags {
		result[i] = params.AnnotationsGetResult{EntityTag: tag}
	}
	s.modelExtractor.EXPECT().GetAnnotations(matcher).Return(result, nil)
}

func (s *buildModelRepSuite) expectEmptyGetConstraints() {
	s.modelExtractor.EXPECT().GetConstraints([]string{}).Return(nil, nil)
}

func (s *buildModelRepSuite) expectGetConstraintsWordpress() {
	s.modelExtractor.EXPECT().GetConstraints([]string{"wordpress"}).Return(nil, nil)
}

func (s *buildModelRepSuite) expectEmptyGetConfig() {
	s.modelExtractor.EXPECT().GetConfig(model.GenerationMaster, []string{}).Return(nil, nil)
}

func (s *buildModelRepSuite) expectGetConfigWordpress() {
	cfg := map[string]interface{}{
		"outlook": map[string]interface{}{
			"description": "No default outlook.",
			"source":      "unset",
			"type":        "string",
		},
		"skill-level": map[string]interface{}{
			"description": "A number indicating skill.",
			"source":      "user",
			"type":        "int",
			"value":       42,
		}}
	s.modelExtractor.EXPECT().GetConfig(model.GenerationMaster, "wordpress").Return([]map[string]interface{}{cfg}, nil)
}

func (s *buildModelRepSuite) expectEmptySequences() {
	s.modelExtractor.EXPECT().Sequences().Return(map[string]int{}, nil)
}

type composeAndVerifyRepSuite struct {
	bundleDataSource *mocks.MockBundleDataSource
}

var _ = gc.Suite(&composeAndVerifyRepSuite{})

func (s *composeAndVerifyRepSuite) TestComposeAndVerifyBundleNoOverlay(c *gc.C) {
	defer s.setupMocks(c).Finish()
	obtained, err := ComposeAndVerifyBundle(s.bundleDataSource, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *composeAndVerifyRepSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.bundleDataSource = mocks.NewMockBundleDataSource(ctrl)
	return ctrl
}

func (s *composeAndVerifyRepSuite) expectParts() {}

//Parts() []*charm.BundleDataPart
func (s *composeAndVerifyRepSuite) expectBasePath() {}

//BasePath() string
func (s *composeAndVerifyRepSuite) expectResolveInclude() {}

//ResolveInclude(path string) ([]byte, error)

type stringSliceMatcher struct {
	c        *gc.C
	expected []string
}

func (m stringSliceMatcher) Matches(x interface{}) bool {
	obtained, ok := x.([]string)
	m.c.Assert(ok, jc.IsTrue)
	if !ok {
		return false
	}
	m.c.Assert(obtained, jc.SameContents, m.expected)
	return true
}

func (m stringSliceMatcher) String() string {
	return "match a slice of strings, no matter the order"
}
