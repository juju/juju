// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"

	"github.com/juju/os/series"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/juju/version"
	"github.com/juju/juju/provider/ec2/internal/ec2instancetypes"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&specSuite{})

type specSuite struct {
	testing.BaseSuite
}

var findInstanceSpecTests = []struct {
	// LTS-dependent requires new or updated entries upon a new LTS release.
	series  string
	arches  []string
	cons    string
	itype   string
	image   string
	storage []string
}{
	{
		series: "xenial",
		arches: []string{"amd64"},
		itype:  "t3.micro",
		image:  "ami-00000133",
	}, {
		series: "quantal",
		arches: []string{"amd64"},
		itype:  "t3.micro",
		image:  "ami-01000035",
	}, {
		series: "xenial",
		arches: []string{"amd64"},
		cons:   "cores=4",
		itype:  "t3.xlarge",
		image:  "ami-00000133",
	}, {
		series: "xenial",
		arches: []string{"amd64"},
		cons:   "mem=10G",
		itype:  "r5.large",
		image:  "ami-00000133",
	}, {
		series: "xenial",
		arches: []string{"amd64"},
		cons:   "mem=",
		itype:  "t3.nano",
		image:  "ami-00000133",
	}, {
		series: "xenial",
		arches: []string{"amd64"},
		cons:   "cpu-power=",
		itype:  "t3.micro",
		image:  "ami-00000133",
	}, {
		series: "xenial",
		arches: []string{"amd64"},
		cons:   "cpu-power=800",
		itype:  "c5.large",
		image:  "ami-00000133",
	}, {
		series: "xenial",
		arches: []string{"amd64"},
		cons:   "instance-type=m1.medium cpu-power=100",
		itype:  "m1.medium",
		image:  "ami-00000135",
	}, {
		series: "xenial",
		arches: []string{"amd64"},
		cons:   "mem=2G root-disk=16384M",
		itype:  "t3.small",
		image:  "ami-00000133",
	}, {
		series:  "xenial",
		arches:  []string{"amd64"},
		cons:    "mem=4G root-disk=16384M",
		itype:   "t3.medium",
		storage: []string{"ssd", "ebs"},
		image:   "ami-00000133",
	}, {
		series:  "xenial",
		arches:  []string{"amd64"},
		cons:    "mem=4G root-disk=16384M",
		itype:   "t3.medium",
		storage: []string{"ebs", "ssd"},
		image:   "ami-00000139",
	}, {
		series:  "xenial",
		arches:  []string{"amd64"},
		cons:    "mem=4G root-disk=16384M",
		itype:   "t3.medium",
		storage: []string{"ebs"},
		image:   "ami-00000139",
	}, {
		series: "trusty",
		arches: []string{"amd64"},
		itype:  "t3.micro",
		image:  "ami-00000033",
	}, {
		series: "trusty",
		arches: []string{"i386"},
		cons:   "instance-type=c1.medium",
		itype:  "c1.medium",
		image:  "ami-00000034",
	}, {
		series: "bionic",
		arches: []string{"amd64", "i386"},
		cons:   "arch=amd64",
		itype:  "t3.micro",
		image:  "ami-00001133",
	}, {
		series: "bionic",
		arches: []string{"amd64", "i386"},
		cons:   "instance-type=cc2.8xlarge",
		itype:  "cc2.8xlarge",
		image:  "ami-00001133",
	},
}

func (s *specSuite) TestFindInstanceSpec(c *gc.C) {
	for i, test := range findInstanceSpecTests {
		c.Logf("\ntest %d: %q; %q; %q; %v", i, test.series, test.arches, test.cons, test.storage)
		stor := test.storage
		if len(stor) == 0 {
			stor = []string{ssdStorage, ebsStorage}
		}
		// We need to filter the image metadata to the test's
		// arches and series; the provisioner and bootstrap
		// code will do this.
		imageMetadata := filterImageMetadata(
			c, TestImageMetadata, test.series, test.arches,
		)
		spec, err := findInstanceSpec(
			false, // non-controller
			imageMetadata,
			ec2instancetypes.RegionInstanceTypes("test"),
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

	instanceConstraint := &instances.InstanceConstraint{
		Region:      "test",
		Series:      version.SupportedLTS(),
		Constraints: constraints.MustParse("instance-type=t2.medium"),
	}

	c.Check(instanceConstraint.Constraints.CpuPower, gc.IsNil)
	findInstanceSpec(
		false, // non-controller
		TestImageMetadata,
		ec2instancetypes.RegionInstanceTypes("test"),
		instanceConstraint,
	)

	c.Check(instanceConstraint.Constraints.CpuPower, gc.IsNil)
}

var findInstanceSpecErrorTests = []struct {
	series string
	arches []string
	cons   string
	err    string
}{
	{
		series: version.SupportedLTS(),
		arches: []string{"arm"},
		err:    fmt.Sprintf(`no "%s" images in test with arches \[arm\]`, version.SupportedLTS()),
	}, {
		series: "raring",
		arches: []string{"amd64", "i386"},
		cons:   "mem=4G",
		err:    `no "raring" images in test matching instance types \[.*\]`,
	}, {
		series: version.SupportedLTS(),
		arches: []string{"amd64"},
		cons:   "instance-type=m1.small mem=4G",
		err:    `no instance types in test matching constraints "instance-type=m1.small mem=4096M"`,
	},
}

func (s *specSuite) TestFindInstanceSpecErrors(c *gc.C) {
	for i, t := range findInstanceSpecErrorTests {
		c.Logf("test %d", i)
		// We need to filter the image metadata to the test's
		// arches and series; the provisioner and bootstrap
		// code will do this.
		imageMetadata := filterImageMetadata(
			c, TestImageMetadata, t.series, t.arches,
		)
		_, err := findInstanceSpec(
			false, // non-controller
			imageMetadata,
			ec2instancetypes.RegionInstanceTypes("test"),
			&instances.InstanceConstraint{
				Region:      "test",
				Series:      t.series,
				Arches:      t.arches,
				Constraints: constraints.MustParse(t.cons),
			})
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

func filterImageMetadata(
	c *gc.C,
	in []*imagemetadata.ImageMetadata,
	filterSeries string, filterArches []string,
) []*imagemetadata.ImageMetadata {
	var imageMetadata []*imagemetadata.ImageMetadata
	for _, im := range in {
		version, err := series.SeriesVersion(filterSeries)
		c.Assert(err, jc.ErrorIsNil)
		if im.Version != version {
			continue
		}
		match := false
		for _, arch := range filterArches {
			match = match || im.Arch == arch
		}
		if match {
			imageMetadata = append(imageMetadata, im)
		}
	}
	return imageMetadata
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
