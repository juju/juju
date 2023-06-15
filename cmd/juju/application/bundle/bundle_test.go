// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application/bundle/mocks"
	bundlechanges "github.com/juju/juju/core/bundle/changes"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
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
			"0": {Base: params.Base{Name: "ubuntu", Channel: "18.04"}},
			"1": {Base: params.Base{Name: "ubuntu", Channel: "18.04"}},
			"2": {Base: params.Base{Name: "ubuntu", Channel: "18.04"}},
			"3": {Base: params.Base{Name: "ubuntu", Channel: "18.04"}},
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
			"0": {Base: params.Base{Name: "ubuntu", Channel: "18.04"}},
			"1": {Base: params.Base{Name: "ubuntu", Channel: "18.04"}},
		},
		Applications: map[string]params.ApplicationStatus{
			"wordpress": {
				Charm: "wordpress",
				Base:  params.Base{Name: "ubuntu", Channel: "18.04"},
				Life:  life.Alive,
				Units: map[string]params.UnitStatus{
					"0": {Machine: "0"},
				},
			},
			"sub": {
				Charm:         "sub",
				Base:          params.Base{Name: "ubuntu", Channel: "18.04"},
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

func (s *composeAndVerifyRepSuite) TestComposeAndVerifyBundleEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyParts()
	s.expectBasePath()

	obtained, _, err := ComposeAndVerifyBundle(s.bundleDataSource, nil)
	c.Assert(err, gc.ErrorMatches, ".*bundle is empty not valid")
	c.Assert(obtained, gc.IsNil)
}

func (s *composeAndVerifyRepSuite) TestComposeAndVerifyBundleUnsupportedConstraints(c *gc.C) {
	defer s.setupMocks(c).Finish()
	bundleData, err := charm.ReadBundleData(strings.NewReader(unsupportedConstraintBundle))
	c.Assert(err, jc.ErrorIsNil)
	s.expectParts(&charm.BundleDataPart{Data: bundleData})
	s.expectBasePath()

	obtained, _, err := ComposeAndVerifyBundle(s.bundleDataSource, nil)
	c.Assert(err, gc.ErrorMatches, "*'image-id' constraint in a base bundle not supported")
	c.Assert(obtained, gc.IsNil)
}

func (s *composeAndVerifyRepSuite) TestComposeAndVerifyBundleNoOverlay(c *gc.C) {
	defer s.setupMocks(c).Finish()
	bundleData, err := charm.ReadBundleData(strings.NewReader(wordpressBundle))
	c.Assert(err, jc.ErrorIsNil)
	s.expectParts(&charm.BundleDataPart{Data: bundleData})
	s.expectBasePath()

	obtained, _, err := ComposeAndVerifyBundle(s.bundleDataSource, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, bundleData)
}

func (s *composeAndVerifyRepSuite) TestComposeAndVerifyBundleOverlay(c *gc.C) {
	defer s.setupMocks(c).Finish()
	bundleData, err := charm.ReadBundleData(strings.NewReader(wordpressBundle))
	c.Assert(err, jc.ErrorIsNil)
	s.expectParts(&charm.BundleDataPart{Data: bundleData})
	s.expectBasePath()
	s.setupOverlayFile(c)

	expected := *bundleData
	expected.Applications["wordpress"].Options = map[string]interface{}{
		"blog-title": "magic bundle config",
	}

	obtained, _, err := ComposeAndVerifyBundle(s.bundleDataSource, []string{s.overlayFile})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, &expected)
}

func (s *composeAndVerifyRepSuite) TestComposeAndVerifyBundleOverlayUnsupportedConstraints(c *gc.C) {
	defer s.setupMocks(c).Finish()
	bundleData, err := charm.ReadBundleData(strings.NewReader(unsupportedConstraintBundle))
	c.Assert(err, jc.ErrorIsNil)
	s.expectParts(&charm.BundleDataPart{Data: bundleData})
	s.expectBasePath()
	s.setupOverlayFile(c)

	expected := *bundleData
	expected.Applications["wordpress"].Options = map[string]interface{}{
		"blog-title": "magic bundle config",
	}

	obtained, _, err := ComposeAndVerifyBundle(s.bundleDataSource, []string{s.overlayFile})
	c.Assert(err, gc.ErrorMatches, "*'image-id' constraint in a base bundle not supported")
	c.Assert(obtained, gc.IsNil)
}

func (s *composeAndVerifyRepSuite) TestComposeAndVerifyBundleOverlayUnmarshallErrors(c *gc.C) {
	defer s.setupMocks(c).Finish()
	bundleData, err := charm.ReadBundleData(strings.NewReader(typoBundle))
	c.Assert(err, jc.ErrorIsNil)
	expectedError := errors.New(`document 0:\n  line 1: unrecognized field "sries"\n  line 18: unrecognized field "constrai"`)
	s.expectParts(&charm.BundleDataPart{
		Data:            bundleData,
		UnmarshallError: expectedError,
	})
	s.expectBasePath()
	s.setupOverlayFile(c)

	expected := *bundleData
	expected.Applications["wordpress"].Options = map[string]interface{}{
		"blog-title": "magic bundle config",
	}

	obtained, unmarshallErrors, err := ComposeAndVerifyBundle(s.bundleDataSource, []string{s.overlayFile})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, &expected)
	c.Assert(unmarshallErrors, gc.HasLen, 1)
	c.Assert(unmarshallErrors[0], gc.Equals, expectedError)
}

func (s *composeAndVerifyRepSuite) setupOverlayFile(c *gc.C) {
	s.overlayDir = c.MkDir()
	s.overlayFile = filepath.Join(s.overlayDir, "config.yaml")
	c.Assert(
		os.WriteFile(
			s.overlayFile, []byte(`
applications:
  wordpress:
    constraints: image-id=ubuntu-bf2
    options:
      blog-title: include-file://title
`), 0644),
		jc.ErrorIsNil)
	c.Assert(
		os.WriteFile(
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
			"0": {Base: params.Base{Name: "ubuntu", Channel: "18.04"}},
		},
		Applications: map[string]params.ApplicationStatus{
			"wordpress": {
				Charm: "wordpress",
				Base:  params.Base{Name: "ubuntu", Channel: "18.04"},
				Life:  life.Alive,
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

func (s *composeAndVerifyRepSuite) expectParts(part *charm.BundleDataPart) {
	retVal := []*charm.BundleDataPart{part}
	s.bundleDataSource.EXPECT().Parts().Return(retVal).AnyTimes()
}

func (s *composeAndVerifyRepSuite) expectEmptyParts() {
	retVal := []*charm.BundleDataPart{}
	s.bundleDataSource.EXPECT().Parts().Return(retVal).AnyTimes()
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

const unsupportedConstraintBundle = `
series: bionic
applications:
  mysql:
    charm: ch:mysql
    revision: 42
    channel: stable
    series: xenial
    num_units: 1
    constraints: image-id=ubuntu-bf2
    to:
    - "0"
  wordpress:
    charm: ch:wordpress
    channel: stable
    revision: 47
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

const wordpressBundle = `
series: bionic
applications:
  mysql:
    charm: ch:mysql
    revision: 42
    channel: stable
    series: xenial
    num_units: 1
    to:
    - "0"
  wordpress:
    charm: ch:wordpress
    channel: stable
    revision: 47
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

const typoBundle = `
sries: bionic
applications:
  mysql:
    charm: ch:mysql
    revision: 42
    channel: stable
    series: xenial
    num_units: 1
    to:
    - "0"
  wordpress:
    charm: ch:wordpress
    channel: stable
    revision: 47
    series: xenial
    num_units: 1
    to:
    - "1"
machines:
  "0":
    series: xenial
    constrai: arch=arm64
  "1":
    series: xenial
relations:
- - wordpress:db
  - mysql:db
`
