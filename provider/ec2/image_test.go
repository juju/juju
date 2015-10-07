// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/testing"
	"github.com/juju/utils/series"
)

var _ = gc.Suite(&specSuite{})

type specSuite struct {
	testing.FakeJujuHomeSuite
}

func (s *specSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpSuite(c)
	UseTestImageData(TestImagesData)
	UseTestInstanceTypeData(TestInstanceTypeCosts)
	UseTestRegionData(TestRegions)
}

func (s *specSuite) TearDownSuite(c *gc.C) {
	UseTestInstanceTypeData(nil)
	UseTestImageData(nil)
	UseTestRegionData(nil)
	s.FakeJujuHomeSuite.TearDownSuite(c)
}

var findInstanceSpecTests = []struct {
	series  string
	arches  []string
	cons    string
	itype   string
	image   string
	storage []string
}{
	{
		series: testing.FakeDefaultSeries,
		arches: both,
		itype:  "m1.small",
		image:  "ami-00000033",
	}, {
		series: "quantal",
		arches: []string{"i386"},
		itype:  "m1.small",
		image:  "ami-01000034",
	}, {
		series: testing.FakeDefaultSeries,
		arches: both,
		cons:   "cpu-cores=4",
		itype:  "m1.xlarge",
		image:  "ami-00000033",
	}, {
		series: testing.FakeDefaultSeries,
		arches: both,
		cons:   "cpu-cores=2 arch=i386",
		itype:  "c1.medium",
		image:  "ami-00000034",
	}, {
		series: testing.FakeDefaultSeries,
		arches: both,
		cons:   "mem=10G",
		itype:  "m1.xlarge",
		image:  "ami-00000033",
	}, {
		series: testing.FakeDefaultSeries,
		arches: both,
		cons:   "mem=",
		itype:  "m1.small",
		image:  "ami-00000033",
	}, {
		series: testing.FakeDefaultSeries,
		arches: both,
		cons:   "cpu-power=",
		itype:  "m1.small",
		image:  "ami-00000033",
	}, {
		series: testing.FakeDefaultSeries,
		arches: both,
		cons:   "cpu-power=800",
		itype:  "m1.xlarge",
		image:  "ami-00000033",
	}, {
		series: testing.FakeDefaultSeries,
		arches: both,
		cons:   "cpu-power=500 arch=i386",
		itype:  "c1.medium",
		image:  "ami-00000034",
	}, {
		series: testing.FakeDefaultSeries,
		arches: []string{"i386"},
		cons:   "cpu-power=400",
		itype:  "c1.medium",
		image:  "ami-00000034",
	}, {
		series: "quantal",
		arches: both,
		cons:   "arch=amd64",
		itype:  "cc2.8xlarge",
		image:  "ami-01000035",
	}, {
		series: "quantal",
		arches: both,
		cons:   "instance-type=cc2.8xlarge",
		itype:  "cc2.8xlarge",
		image:  "ami-01000035",
	}, {
		series: testing.FakeDefaultSeries,
		arches: []string{"i386"},
		cons:   "instance-type=c1.medium",
		itype:  "c1.medium",
		image:  "ami-00000034",
	}, {
		series: testing.FakeDefaultSeries,
		arches: both,
		cons:   "mem=4G root-disk=16384M",
		itype:  "m1.large",
		image:  "ami-00000033",
	}, {
		series:  testing.FakeDefaultSeries,
		arches:  both,
		cons:    "mem=4G root-disk=16384M",
		itype:   "m1.large",
		storage: []string{"ssd", "ebs"},
		image:   "ami-00000033",
	}, {
		series:  testing.FakeDefaultSeries,
		arches:  both,
		cons:    "mem=4G root-disk=16384M",
		itype:   "m1.large",
		storage: []string{"ebs", "ssd"},
		image:   "ami-00000039",
	}, {
		series:  testing.FakeDefaultSeries,
		arches:  both,
		cons:    "mem=4G root-disk=16384M",
		itype:   "m1.large",
		storage: []string{"ebs"},
		image:   "ami-00000039",
	},
}

func (s *specSuite) TestFindInstanceSpec(c *gc.C) {
	env := s.registerTestDataSource(c)
	for i, test := range findInstanceSpecTests {
		c.Logf("\ntest %d: %q; %q; %q; %v", i, test.series, test.arches, test.cons, test.storage)
		stor := test.storage
		if len(stor) == 0 {
			stor = []string{ssdStorage, ebsStorage}
		}
		spec, err := findInstanceSpec(
			env,
			"released",
			&instances.InstanceConstraint{
				Region:      "test",
				Series:      test.series,
				Arches:      test.arches,
				Constraints: constraints.MustParse(test.cons),
				Storage:     stor,
			})
		c.Assert(err, jc.ErrorIsNil)
		c.Check(spec.InstanceType.Name, gc.Equals, test.itype)
		c.Check(spec.Image.Id, gc.Equals, test.image)
	}
}

func (s *specSuite) TestFindInstanceSpecNotSetCpuPowerWhenInstanceTypeSet(c *gc.C) {
	env := s.registerTestDataSource(c)
	instanceConstraint := &instances.InstanceConstraint{
		Region:      "test",
		Series:      testing.FakeDefaultSeries,
		Constraints: constraints.MustParse("instance-type=t2.medium"),
	}

	c.Check(instanceConstraint.Constraints.CpuPower, gc.IsNil)
	findInstanceSpec(env, "released", instanceConstraint)

	c.Check(instanceConstraint.Constraints.CpuPower, gc.IsNil)
}

var findInstanceSpecErrorTests = []struct {
	series string
	arches []string
	cons   string
	err    string
}{
	{
		series: "bad",
		arches: both,
		err:    `unknown version for series: "bad"`,
	}, {
		series: testing.FakeDefaultSeries,
		arches: []string{"arm"},
		err:    `image metadata for series \[trusty\], architectures \[arm\] not found`,
	}, {
		series: "raring",
		arches: both,
		cons:   "mem=4G",
		err:    `no "raring" images in test matching instance types \[m1.large m1.xlarge c1.xlarge cc2.8xlarge\]`,
	},
}

func (s *specSuite) TestFindInstanceSpecErrors(c *gc.C) {
	env := s.registerTestDataSource(c)
	for i, t := range findInstanceSpecErrorTests {
		c.Logf("test %d", i)
		_, err := findInstanceSpec(
			env,
			"released",
			&instances.InstanceConstraint{
				Region:      "test",
				Series:      t.series,
				Arches:      t.arches,
				Constraints: constraints.MustParse(t.cons),
			})
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

func (*specSuite) TestFilterImagesAcceptsNil(c *gc.C) {
	c.Check(filterImages(nil, nil), gc.HasLen, 0)
}

func (*specSuite) TestFilterImagesReturnsSelectively(c *gc.C) {
	good := imagemetadata.ImageMetadata{Id: "good", Storage: "ebs"}
	bad := imagemetadata.ImageMetadata{Id: "bad", Storage: "ftp"}
	input := []*imagemetadata.ImageMetadata{&good, &bad}
	expectation := []*imagemetadata.ImageMetadata{&good}

	ic := &instances.InstanceConstraint{Storage: []string{"ebs"}}
	c.Check(filterImages(input, ic), gc.DeepEquals, expectation)
}

func (*specSuite) TestFilterImagesMaintainsOrdering(c *gc.C) {
	input := []*imagemetadata.ImageMetadata{
		{Id: "one", Storage: "ebs"},
		{Id: "two", Storage: "ebs"},
		{Id: "three", Storage: "ebs"},
	}
	ic := &instances.InstanceConstraint{Storage: []string{"ebs"}}
	c.Check(filterImages(input, ic), gc.DeepEquals, input)
}

type mockEnviron struct {
	environ
	config func() *config.Config
}

func (env *mockEnviron) Config() *config.Config {
	return env.config()
}

func minimalConfig(c *gc.C) *config.Config {
	attrs := map[string]interface{}{
		"name":            "whatever",
		"type":            "anything, really",
		"uuid":            testing.EnvironmentTag.Id(),
		"ca-cert":         testing.CACert,
		"ca-private-key":  testing.CAKey,
		"authorized-keys": testing.FakeAuthKeys,
		"default-series":  series.HostSeries(),
	}
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func configGetter(c *gc.C) func() *config.Config {
	cfg := minimalConfig(c)
	return func() *config.Config { return cfg }
}

func (s *specSuite) registerTestDataSource(c *gc.C) environs.Environ {
	s.PatchValue(&imagemetadata.DefaultBaseURL, "")
	env := &mockEnviron{
		config: configGetter(c),
	}

	environs.RegisterImageDataSourceFunc("test", func(environs.Environ) (simplestreams.DataSource, error) {
		return simplestreams.NewURLDataSource("test", "test:", utils.VerifySSLHostnames), nil
	})
	s.AddCleanup(func(*gc.C) {
		environs.UnregisterImageDataSourceFunc("test")
	})

	return env
}
