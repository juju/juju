// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"

	"github.com/juju/os/series"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&specSuite{})

type specSuite struct {
	testing.BaseSuite
}

var paravirtual = "pv"

var testInstanceTypes = []instances.InstanceType{{
	Name:     "t3a.xlarge",
	CpuCores: 4,
	CpuPower: instances.CpuPower(1400),
	Mem:      16384,
	Arches:   []string{"amd64"},
	Cost:     172,
}, {
	Name:     "t3a.nano",
	CpuCores: 2,
	CpuPower: instances.CpuPower(700),
	Mem:      512,
	Arches:   []string{"amd64"},
	Cost:     5,
}, {
	Name:     "t3a.micro",
	CpuCores: 2,
	CpuPower: instances.CpuPower(700),
	Mem:      1024,
	Arches:   []string{"amd64"},
	Cost:     10,
}, {
	Name:     "t3a.small",
	CpuCores: 2,
	CpuPower: instances.CpuPower(700),
	Mem:      2048,
	Arches:   []string{"amd64"},
	Cost:     21,
}, {
	Name:     "t3a.medium",
	CpuPower: instances.CpuPower(700),
	Mem:      4096,
	Arches:   []string{"amd64"},
	Cost:     43,
}, {
	Name:     "m1.medium",
	CpuCores: 1,
	CpuPower: instances.CpuPower(100),
	Mem:      3840,
	Arches:   []string{"amd64", "i386"},
	VirtType: &paravirtual,
	Cost:     117,
}, {
	Name:     "a1.xlarge",
	CpuCores: 4,
	CpuPower: instances.CpuPower(1288),
	Mem:      8192,
	Arches:   []string{"arm64"},
}, {
	Name:     "c4.large",
	CpuCores: 2,
	CpuPower: instances.CpuPower(811),
	Mem:      3840,
	Arches:   []string{"amd64"},
	Cost:     114,
}, {
	Name:     "t2.medium",
	CpuCores: 2,
	CpuPower: instances.CpuPower(40),
	Mem:      4096,
	Arches:   []string{"amd64"},
	Cost:     46,
}, {
	Name:     "c1.medium",
	CpuCores: 2,
	CpuPower: instances.CpuPower(200),
	Mem:      1741,
	Arches:   []string{"amd64", "i386"},
	Cost:     164,
}, {
	Name:     "cc2.8xlarge",
	CpuCores: 32,
	CpuPower: instances.CpuPower(11647),
	Mem:      61952,
	Arches:   []string{"amd64"},
	Cost:     2250,
}, {
	Name:     "r5a.large",
	CpuCores: 2,
	CpuPower: instances.CpuPower(700),
	Mem:      16384,
	Arches:   []string{"amd64"},
	Cost:     137,
}}

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
		itype:  "t3a.micro",
		image:  "ami-00000133",
	}, {
		series: "quantal",
		arches: []string{"amd64"},
		itype:  "t3a.micro",
		image:  "ami-01000035",
	}, {
		series: "xenial",
		arches: []string{"amd64"},
		cons:   "cores=4",
		itype:  "t3a.xlarge",
		image:  "ami-00000133",
	}, {
		series: "bionic",
		arches: []string{"arm64"},
		cons:   "cores=4",
		itype:  "a1.xlarge",
		image:  "ami-00002133",
	}, {
		series: "xenial",
		arches: []string{"amd64"},
		cons:   "mem=10G",
		itype:  "r5a.large",
		image:  "ami-00000133",
	}, {
		series: "xenial",
		arches: []string{"amd64"},
		cons:   "mem=",
		itype:  "t3a.nano",
		image:  "ami-00000133",
	}, {
		series: "xenial",
		arches: []string{"amd64"},
		cons:   "cpu-power=",
		itype:  "t3a.micro",
		image:  "ami-00000133",
	}, {
		series: "xenial",
		arches: []string{"amd64"},
		cons:   "cpu-power=800",
		itype:  "c4.large",
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
		itype:  "t3a.small",
		image:  "ami-00000133",
	}, {
		series:  "xenial",
		arches:  []string{"amd64"},
		cons:    "mem=4G root-disk=16384M",
		itype:   "t3a.medium",
		storage: []string{"ssd", "ebs"},
		image:   "ami-00000133",
	}, {
		series:  "xenial",
		arches:  []string{"amd64"},
		cons:    "mem=4G root-disk=16384M",
		itype:   "t3a.medium",
		storage: []string{"ebs", "ssd"},
		image:   "ami-00000139",
	}, {
		series:  "xenial",
		arches:  []string{"amd64"},
		cons:    "mem=4G root-disk=16384M",
		itype:   "t3a.medium",
		storage: []string{"ebs"},
		image:   "ami-00000139",
	}, {
		series: "trusty",
		arches: []string{"amd64"},
		itype:  "t3a.micro",
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
		itype:  "t3a.micro",
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
	size := len(findInstanceSpecTests)
	for i, test := range findInstanceSpecTests {
		c.Logf("\ntest %d of %d: %q; %q; %q; %q; %q; %v", i+1, size,
			test.series, test.arches, test.cons, test.itype, test.image,
			test.storage)
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
			testInstanceTypes,
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
		Series:      series.DefaultSupportedLTS(),
		Constraints: constraints.MustParse("instance-type=t2.medium"),
	}

	c.Check(instanceConstraint.Constraints.CpuPower, gc.IsNil)
	_, err := findInstanceSpec(
		false, // non-controller
		TestImageMetadata,
		testInstanceTypes,
		instanceConstraint,
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(instanceConstraint.Constraints.CpuPower, gc.IsNil)
}

var findInstanceSpecErrorTests = []struct {
	series string
	arches []string
	cons   string
	err    string
}{
	{
		series: series.DefaultSupportedLTS(),
		arches: []string{"arm"},
		err:    fmt.Sprintf(`no metadata for "%s" images in test with arches \[arm\]`, series.DefaultSupportedLTS()),
	}, {
		series: "raring",
		arches: []string{"amd64", "i386"},
		cons:   "mem=4G",
		err:    `no "raring" images in test matching instance types \[.*\]`,
	}, {
		series: series.DefaultSupportedLTS(),
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
			testInstanceTypes,
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
