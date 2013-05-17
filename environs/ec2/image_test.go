// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/testing"
)

type imageSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&imageSuite{})

func (s *imageSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	UseTestImageData(TestImagesData)
}

func (s *imageSuite) TearDownSuite(c *C) {
	UseTestImageData(nil)
	s.LoggingSuite.TearDownTest(c)
}

type specSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&specSuite{})

func (s *specSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	UseTestImageData(TestImagesData)
	UseTestInstanceTypeData(TestInstanceTypeCosts)
	UseTestRegionData(TestRegions)
}

func (s *specSuite) TearDownSuite(c *C) {
	UseTestInstanceTypeData(nil)
	UseTestImageData(nil)
	UseTestRegionData(nil)
	s.LoggingSuite.TearDownSuite(c)
}

var findInstanceSpecTests = []struct {
	series       string
	arches       []string
	cons         string
	defaultImage string
	itype        string
	image        string
}{
	{
		series: "precise",
		arches: both,
		itype:  "m1.small",
		image:  "ami-00000033",
	}, {
		series: "quantal",
		arches: both,
		itype:  "m1.small",
		image:  "ami-01000034",
	}, {
		series: "precise",
		arches: both,
		cons:   "cpu-cores=4",
		itype:  "m1.xlarge",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: both,
		cons:   "cpu-cores=2 arch=i386",
		itype:  "c1.medium",
		image:  "ami-00000034",
	}, {
		series: "precise",
		arches: both,
		cons:   "mem=10G",
		itype:  "m1.xlarge",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: both,
		cons:   "mem=",
		itype:  "m1.small",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: both,
		cons:   "cpu-power=",
		itype:  "t1.micro",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: both,
		cons:   "cpu-power=800",
		itype:  "m1.xlarge",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: both,
		cons:   "cpu-power=500 arch=i386",
		itype:  "c1.medium",
		image:  "ami-00000034",
	}, {
		series: "precise",
		arches: []string{"i386"},
		cons:   "cpu-power=400",
		itype:  "c1.medium",
		image:  "ami-00000034",
	}, {
		series: "quantal",
		arches: both,
		cons:   "arch=amd64",
		itype:  "cc1.4xlarge",
		image:  "ami-01000035",
	}, {
		series:       "raring",
		arches:       both,
		itype:        "m1.small",
		defaultImage: "ami-02000035",
		image:        "ami-02000035",
	},
}

func (s *specSuite) TestFindInstanceSpec(c *C) {
	for i, t := range findInstanceSpecTests {
		c.Logf("test %d", i)
		storage := ebsStorage
		spec, err := findInstanceSpec([]string{"test:"}, &instances.InstanceConstraint{
			Region:         "test",
			Series:         t.series,
			Arches:         t.arches,
			Constraints:    constraints.MustParse(t.cons),
			DefaultImageId: t.defaultImage,
			Storage:        &storage,
		})
		c.Assert(err, IsNil)
		c.Check(spec.InstanceTypeName, Equals, t.itype)
		c.Check(spec.Image.Id, Equals, t.image)
	}
}

var findInstanceSpecErrorTests = []struct {
	series         string
	arches         []string
	cons           string
	defaultImageId string
	err            string
}{
	{
		series: "bad",
		arches: both,
		err:    `invalid series "bad"`,
	}, {
		series: "precise",
		arches: []string{"arm"},
		err:    `no "precise" images in test with arches \[arm\], and no default specified`,
	}, {
		series:         "precise",
		arches:         both,
		defaultImageId: "bad",
		err:            `invalid default image id "bad"`,
	}, {
		series: "raring",
		arches: both,
		cons:   "mem=4G",
		err:    `no "raring" images in test matching instance types \[m1.large m1.xlarge c1.xlarge cc1.4xlarge cc2.8xlarge\]`,
	},
}

func (s *specSuite) TestFindInstanceSpecErrors(c *C) {
	for i, t := range findInstanceSpecErrorTests {
		c.Logf("test %d", i)
		_, err := findInstanceSpec([]string{"test:"}, &instances.InstanceConstraint{
			Region:         "test",
			Series:         t.series,
			Arches:         t.arches,
			Constraints:    constraints.MustParse(t.cons),
			DefaultImageId: t.defaultImageId,
		})
		c.Check(err, ErrorMatches, t.err)
	}
}
