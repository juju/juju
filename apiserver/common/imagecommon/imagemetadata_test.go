// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagecommon_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/imagecommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state/cloudimagemetadata"
	coretesting "github.com/juju/juju/testing"
)

type imageMetadataSuite struct {
	st *mockState
}

var _ = gc.Suite(&imageMetadataSuite{})

func (s *imageMetadataSuite) SetUpTest(c *gc.C) {
	mCfg := testConfig(c)
	s.st = &mockState{
		Stub:     &testing.Stub{},
		modelCfg: mCfg,
	}
}

func (s *imageMetadataSuite) TestSaveEmpty(c *gc.C) {
	errs, err := imagecommon.Save(s.st, params.MetadataSaveParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.HasLen, 0)
	s.st.CheckCallNames(c, []string{}...) // Nothing was called
}

func (s *imageMetadataSuite) TestSaveModelCfgFailed(c *gc.C) {
	m := params.CloudImageMetadata{
		Source: "custom",
	}
	ms := params.MetadataSaveParams{
		Metadata: []params.CloudImageMetadataList{{
			Metadata: []params.CloudImageMetadata{m},
		}},
	}

	msg := "save error"
	s.st.SetErrors(
		errors.New(msg), // ModelConfig
	)

	errs, err := imagecommon.Save(s.st, ms)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(errs, gc.IsNil)
	s.st.CheckCallNames(c, "ModelConfig")
}

func (s *imageMetadataSuite) TestSave(c *gc.C) {
	m := params.CloudImageMetadata{
		Source: "custom",
	}
	ms := params.MetadataSaveParams{
		Metadata: []params.CloudImageMetadataList{{
			Metadata: []params.CloudImageMetadata{m},
		}, {
			Metadata: []params.CloudImageMetadata{m, m},
		}},
	}

	msg := "save error"
	s.st.SetErrors(
		nil,             // ModelConfig
		nil,             // Save (1st call)
		errors.New(msg), // Save (2nd call)
	)

	errs, err := imagecommon.Save(s.st, ms)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.HasLen, 2)
	c.Assert(errs[0].Error, gc.IsNil)
	c.Assert(errs[1].Error, gc.ErrorMatches, msg)

	// TODO (anastasiamac 2016-08-24) This is a check for a band-aid solution.
	// Once correct value is read from simplestreams, this "adjustment" needs to go.
	// Bug# 1616295
	m.Stream = "released"

	expectedMetadata1 := imagecommon.ParseMetadataListFromParams(params.CloudImageMetadataList{
		Metadata: []params.CloudImageMetadata{m},
	}, nil)
	c.Assert(expectedMetadata1[0].Priority, gc.Equals, 50)
	expectedMetadata2 := imagecommon.ParseMetadataListFromParams(params.CloudImageMetadataList{
		Metadata: []params.CloudImageMetadata{m, m},
	}, nil)

	s.st.CheckCalls(c, []testing.StubCall{
		{"ModelConfig", nil},
		{"SaveMetadata", []interface{}{expectedMetadata1}},
		{"SaveMetadata", []interface{}{expectedMetadata2}},
	})
}

type mockState struct {
	*testing.Stub
	modelCfg *config.Config
}

func (s *mockState) SaveMetadata(m []cloudimagemetadata.Metadata) error {
	s.MethodCall(s, "SaveMetadata", m)
	return s.NextErr()
}

func (s *mockState) ModelConfig() (*config.Config, error) {
	s.MethodCall(s, "ModelConfig")
	return s.modelCfg, s.NextErr()
}

func testConfig(c *gc.C) *config.Config {
	cfg, err := config.New(config.UseDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		"type": "mock",
	}))
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}
