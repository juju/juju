// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"github.com/juju/charm/v12"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type BaseSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&BaseSuite{})

func (s *BaseSuite) TestParseBase(c *gc.C) {
	base, err := ParseBase("ubuntu", "22.04")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(base, jc.DeepEquals, Base{OS: "ubuntu", Channel: Channel{Track: "22.04", Risk: "stable"}})
	base, err = ParseBase("ubuntu", "22.04/edge")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(base, jc.DeepEquals, Base{OS: "ubuntu", Channel: Channel{Track: "22.04", Risk: "edge"}})
}

func (s *BaseSuite) TestGetBaseFromSeries(c *gc.C) {
	base, err := GetBaseFromSeries("jammy")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(base, jc.DeepEquals, Base{OS: "ubuntu", Channel: Channel{Track: "22.04", Risk: "stable"}})
}

func (s *BaseSuite) TestGetSeriesFromChannel(c *gc.C) {
	series, err := GetSeriesFromChannel("ubuntu", "22.04")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(series, gc.Equals, "jammy")
	series, err = GetSeriesFromChannel("ubuntu", "22.04/edge")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(series, gc.Equals, "jammy")
}

func (s *BaseSuite) TestGetSeriesFromBase(c *gc.C) {
	series, err := GetSeriesFromBase(MakeDefaultBase("ubuntu", "22.04"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(series, gc.Equals, "jammy")
}

func (s *BaseSuite) TestParseBaseFromString(c *gc.C) {
	base, err := ParseBaseFromString("ubuntu@22.04")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(base.String(), gc.Equals, "ubuntu@22.04/stable")
	base, err = ParseBaseFromString("ubuntu@22.04/edge")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(base.String(), gc.Equals, "ubuntu@22.04/edge")
	base, err = ParseBaseFromString("foo")
	c.Assert(err, gc.ErrorMatches, `expected base string to contain os and channel separated by '@'`)
}

func (s *BaseSuite) TestDisplayString(c *gc.C) {
	b := Base{OS: "ubuntu", Channel: Channel{Track: "18.04"}}
	c.Check(b.DisplayString(), gc.Equals, "ubuntu@18.04")
	b = Base{OS: "kubuntu", Channel: Channel{Track: "20.04", Risk: "stable"}}
	c.Check(b.DisplayString(), gc.Equals, "kubuntu@20.04")
	b = Base{OS: "qubuntu", Channel: Channel{Track: "22.04", Risk: "edge"}}
	c.Check(b.DisplayString(), gc.Equals, "qubuntu@22.04/edge")
}

func (s *BaseSuite) TestParseManifestBases(c *gc.C) {
	manifestBases := []charm.Base{{
		Name: "ubuntu", Channel: charm.Channel{
			Track: "18.04",
			Risk:  "stable",
		},
		Architectures: []string{"amd64"},
	}, {
		Name: "ubuntu", Channel: charm.Channel{
			Track: "20.04",
			Risk:  "edge",
		},
	}, {
		Name: "ubuntu", Channel: charm.Channel{
			Track: "18.04",
			Risk:  "stable",
		},
		Architectures: []string{"arm64"},
	}, {
		Name: "centos", Channel: charm.Channel{
			Track: "9",
			Risk:  "candidate",
		},
	}}
	obtained, err := ParseManifestBases(manifestBases)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.HasLen, 3)
	expected := []Base{
		{OS: "ubuntu", Channel: Channel{Track: "18.04", Risk: "stable"}},
		{OS: "ubuntu", Channel: Channel{Track: "20.04", Risk: "edge"}},
		{OS: "centos", Channel: Channel{Track: "9", Risk: "candidate"}},
	}
	c.Assert(obtained, jc.DeepEquals, expected)
}

var ubuntuLTS = []Base{
	MustParseBaseFromString("ubuntu@20.04"),
	MustParseBaseFromString("ubuntu@22.04"),
	MustParseBaseFromString("ubuntu@24.04"),
	MustParseBaseFromString("ubuntu@24.04/stable"),
	MustParseBaseFromString("ubuntu@24.04/edge"),
}

func (s *BaseSuite) TestIsUbuntuLTSForLTSes(c *gc.C) {
	for i, lts := range ubuntuLTS {
		c.Logf("Checking index %d base %v", i, lts)
		c.Check(lts.IsUbuntuLTS(), jc.IsTrue)
	}
}

var nonUbuntuLTS = []Base{
	MustParseBaseFromString("ubuntu@17.04"),
	MustParseBaseFromString("ubuntu@19.04"),
	MustParseBaseFromString("ubuntu@21.04"),

	MustParseBaseFromString("ubuntu@18.10"),
	MustParseBaseFromString("ubuntu@20.10"),
	MustParseBaseFromString("ubuntu@22.10"),

	MustParseBaseFromString("ubuntu@22.04-blah"),
	MustParseBaseFromString("ubuntu@22.04.1234"),

	MustParseBaseFromString("centos@7"),
	MustParseBaseFromString("centos@20.04"),
}

func (s *BaseSuite) TestIsUbuntuLTSForNonLTSes(c *gc.C) {
	for i, lts := range nonUbuntuLTS {
		c.Logf("Checking index %d base %v", i, lts)
		c.Check(lts.IsUbuntuLTS(), jc.IsFalse)
	}
}
