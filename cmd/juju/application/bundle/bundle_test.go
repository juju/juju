// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

import (
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/bundlechanges/v4"
	"github.com/juju/charm/v8"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/application/bundle/mocks"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
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
	s.expectGetConfigSubWordpress()
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
				Charm:  "wordpress",
				Series: "bionic",
				Life:   life.Alive,
				Units: map[string]params.UnitStatus{
					"0": {Machine: "0"},
				},
			},
			"sub": {
				Charm:         "sub",
				Series:        "bionic",
				Life:          life.Alive,
				SubordinateTo: []string{"wordpress"},
			},
		},
	}
	machines := map[string]string{}

	obtainedModel, err := BuildModelRepresentation(status, s.modelExtractor, false, machines)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedModel.Applications, gc.HasLen, 2)
	obtainedWordpress, ok := obtainedModel.Applications["wordpress"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(obtainedWordpress.Options, gc.HasLen, 1)
	_, ok = obtainedWordpress.Options["skill-level"]
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

func (s *buildModelRepSuite) expectGetConfigSubWordpress() {
	wordpressCfg := map[string]interface{}{
		"outlook": map[string]interface{}{ // Uses default value, will not be in model representation.
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
	retval := []map[string]interface{}{
		{},           // sub
		wordpressCfg, // wordpress
	}
	s.modelExtractor.EXPECT().GetConfig(model.GenerationMaster, "sub", "wordpress").Return(retval, nil)
}

func (s *buildModelRepSuite) expectEmptySequences() {
	s.modelExtractor.EXPECT().Sequences().Return(map[string]int{}, nil)
}

type composeAndVerifyRepSuite struct {
	bundleDataSource *mocks.MockBundleDataSource
	overlayDir       string
	overlayFile      string
}

var _ = gc.Suite(&composeAndVerifyRepSuite{})

func (s *composeAndVerifyRepSuite) TestComposeAndVerifyBundleNoOverlay(c *gc.C) {
	defer s.setupMocks(c).Finish()
	bundleData, err := charm.ReadBundleData(strings.NewReader(wordpressBundle))
	c.Assert(err, jc.ErrorIsNil)
	s.expectParts(bundleData)
	s.expectBasePath()

	obtained, err := ComposeAndVerifyBundle(s.bundleDataSource, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, bundleData)
}

func (s *composeAndVerifyRepSuite) TestComposeAndVerifyBundleOverlay(c *gc.C) {
	defer s.setupMocks(c).Finish()
	bundleData, err := charm.ReadBundleData(strings.NewReader(wordpressBundle))
	c.Assert(err, jc.ErrorIsNil)
	s.expectParts(bundleData)
	s.expectBasePath()
	s.setupOverlayFile(c)

	expected := *bundleData
	expected.Applications["wordpress"].Options = map[string]interface{}{
		"blog-title": "magic bundle config",
	}

	obtained, err := ComposeAndVerifyBundle(s.bundleDataSource, []string{s.overlayFile})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, &expected)
}

func (s *composeAndVerifyRepSuite) setupOverlayFile(c *gc.C) {
	s.overlayDir = c.MkDir()
	s.overlayFile = filepath.Join(s.overlayDir, "config.yaml")
	c.Assert(
		ioutil.WriteFile(
			s.overlayFile, []byte(`
                applications:
                    wordpress:
                        options:
                            blog-title: include-file://title
            `), 0644),
		jc.ErrorIsNil)
	c.Assert(
		ioutil.WriteFile(
			filepath.Join(s.overlayDir, "title"), []byte("magic bundle config"), 0644),
		jc.ErrorIsNil)
}

func (s *buildModelRepSuite) TestBuildModelRepresentationApplicationsWithExposedEndpoints(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectGetAnnotations(c, []string{"machine-0", "application-wordpress"})
	s.expectGetConstraintsWordpress()
	s.expectEmptySequences()

	s.modelExtractor.EXPECT().GetConfig(model.GenerationMaster, "wordpress").Return(nil, nil)

	status := &params.FullStatus{
		Model: params.ModelStatusInfo{
			Name: "default",
		},
		Machines: map[string]params.MachineStatus{
			"0": {Series: "bionic"},
		},
		Applications: map[string]params.ApplicationStatus{
			"wordpress": {
				Charm:  "wordpress",
				Series: "bionic",
				Life:   life.Alive,
				Units: map[string]params.UnitStatus{
					"0": {Machine: "0"},
				},
				ExposedEndpoints: map[string]params.ExposedEndpoint{
					"": {
						ExposeToCIDRs: []string{"10.0.0.0/24"},
					},
					"website": {
						ExposeToSpaces: []string{"inner", "outer"},
						ExposeToCIDRs:  []string{"192.168.0.0/24"},
					},
				},
			},
		},
	}
	machines := map[string]string{}

	obtainedModel, err := BuildModelRepresentation(status, s.modelExtractor, false, machines)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedModel.Applications, gc.HasLen, 1)
	obtainedWordpress, ok := obtainedModel.Applications["wordpress"]
	c.Assert(ok, jc.IsTrue)

	c.Assert(obtainedWordpress.ExposedEndpoints, gc.DeepEquals, map[string]bundlechanges.ExposedEndpoint{
		"": {
			ExposeToCIDRs: []string{"10.0.0.0/24"},
		},
		"website": {
			ExposeToSpaces: []string{"inner", "outer"},
			ExposeToCIDRs:  []string{"192.168.0.0/24"},
		},
	})

	c.Assert(obtainedModel.Machines, gc.HasLen, 1)
	c.Assert(obtainedModel.Relations, gc.HasLen, 0)
	c.Assert(obtainedModel.Sequence, gc.HasLen, 0)
	c.Assert(obtainedModel.MachineMap, gc.HasLen, 0)
}

func (s *composeAndVerifyRepSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.bundleDataSource = mocks.NewMockBundleDataSource(ctrl)
	return ctrl
}

func (s *composeAndVerifyRepSuite) expectParts(bundleData *charm.BundleData) {
	retVal := []*charm.BundleDataPart{{Data: bundleData}}
	s.bundleDataSource.EXPECT().Parts().Return(retVal)
}

func (s *composeAndVerifyRepSuite) expectBasePath() {
	s.bundleDataSource.EXPECT().BasePath().Return(s.overlayDir).AnyTimes()
}

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

const wordpressBundle = `
series: bionic
applications:
  mysql:
    charm: cs:mysql-42
    series: xenial
    num_units: 1
    to:
    - "0"
  wordpress:
    charm: cs:wordpress-47
    series: xenial
    num_units: 1
    to:
    - "1"
machines:
  "0":
    series: xenial
  "1":
    series: xenial
relations:
- - wordpress:db
  - mysql:db
`
