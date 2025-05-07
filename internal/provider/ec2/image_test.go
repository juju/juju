// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/testing"
)

var _ = tc.Suite(&specSuite{})

type specSuite struct {
	testing.BaseSuite
}

var paravirtual = "pv"

var testInstanceTypes = []instances.InstanceType{{
	Name:     "t3a.xlarge",
	CpuCores: 4,
	CpuPower: instances.CpuPower(1400),
	Mem:      16384,
	Arch:     "amd64",
	Cost:     172,
}, {
	Name:     "t3a.nano",
	CpuCores: 2,
	CpuPower: instances.CpuPower(700),
	Mem:      512,
	Arch:     "amd64",
	Cost:     5,
}, {
	Name:     "t3a.micro",
	CpuCores: 2,
	CpuPower: instances.CpuPower(700),
	Mem:      1024,
	Arch:     "amd64",
	Cost:     10,
}, {
	Name:     "t3a.small",
	CpuCores: 2,
	CpuPower: instances.CpuPower(700),
	Mem:      2048,
	Arch:     "amd64",
	Cost:     21,
}, {
	Name:     "t3a.medium",
	CpuCores: 2,
	CpuPower: instances.CpuPower(700),
	Mem:      4096,
	Arch:     "amd64",
	Cost:     43,
}, {
	Name:     "m1.medium",
	CpuCores: 1,
	CpuPower: instances.CpuPower(100),
	Mem:      3840,
	Arch:     "amd64",
	VirtType: &paravirtual,
	Cost:     117,
}, {
	Name:     "a1.xlarge",
	CpuCores: 4,
	CpuPower: instances.CpuPower(1288),
	Mem:      8192,
	Arch:     "arm64",
}, {
	Name:     "c4.large",
	CpuCores: 2,
	CpuPower: instances.CpuPower(811),
	Mem:      3840,
	Arch:     "amd64",
	Cost:     114,
}, {
	Name:     "t2.medium",
	CpuCores: 2,
	CpuPower: instances.CpuPower(40),
	Mem:      4096,
	Arch:     "amd64",
	Cost:     46,
}, {
	Name:     "c1.medium",
	CpuCores: 2,
	CpuPower: instances.CpuPower(200),
	Mem:      1741,
	Arch:     "amd64",
	Cost:     164,
}, {
	Name:     "cc2.8xlarge",
	CpuCores: 32,
	CpuPower: instances.CpuPower(11647),
	Mem:      61952,
	Arch:     "amd64",
	Cost:     2250,
}, {
	Name:     "r5a.large",
	CpuCores: 2,
	CpuPower: instances.CpuPower(700),
	Mem:      16384,
	Arch:     "amd64",
	Cost:     137,
}}

var findInstanceSpecTests = []struct {
	// LTS-dependent requires new or updated entries upon a new LTS release.
	version string
	arch    string
	cons    string
	itype   string
	image   string
	storage []string
}{
	{
		version: "18.04",
		arch:    "amd64",
		itype:   "t3a.small",
		image:   "ami-00001133",
	}, {
		version: "18.04",
		arch:    "amd64",
		cons:    "cores=4",
		itype:   "t3a.xlarge",
		image:   "ami-00001133",
	}, {
		version: "20.04",
		arch:    "arm64",
		cons:    "cores=4",
		itype:   "a1.xlarge",
		image:   "ami-02004133",
	}, {
		version: "18.04",
		arch:    "amd64",
		cons:    "mem=10G",
		itype:   "r5a.large",
		image:   "ami-00001133",
	}, {
		version: "18.04",
		arch:    "amd64",
		cons:    "mem=",
		itype:   "t3a.small",
		image:   "ami-00001133",
	}, {
		version: "18.04",
		arch:    "amd64",
		cons:    "cpu-power=",
		itype:   "t3a.small",
		image:   "ami-00001133",
	}, {
		version: "18.04",
		arch:    "amd64",
		cons:    "cpu-power=800",
		itype:   "c4.large",
		image:   "ami-00001133",
	}, {
		version: "18.04",
		arch:    "amd64",
		cons:    "instance-type=m1.medium cpu-power=100",
		itype:   "m1.medium",
		image:   "ami-00001135",
	}, {
		version: "18.04",
		arch:    "amd64",
		cons:    "mem=2G root-disk=16384M",
		itype:   "t3a.small",
		image:   "ami-00001133",
	}, {
		version: "18.04",
		arch:    "amd64",
		cons:    "mem=4G root-disk=16384M",
		itype:   "t3a.medium",
		storage: []string{"ssd", "ebs"},
		image:   "ami-00001133",
	}, {
		version: "18.04",
		arch:    "amd64",
		cons:    "mem=4G root-disk=16384M",
		itype:   "t3a.medium",
		storage: []string{"ebs", "ssd"},
		image:   "ami-00001139",
	}, {
		version: "18.04",
		arch:    "amd64",
		cons:    "mem=4G root-disk=16384M",
		itype:   "t3a.medium",
		storage: []string{"ebs"},
		image:   "ami-00001139",
	}, {
		version: "24.04",
		arch:    "amd64",
		itype:   "t3a.small",
		image:   "ami-02404133",
	}, {
		version: "22.04",
		arch:    "amd64",
		itype:   "t3a.small",
		image:   "ami-02204133",
	}, {
		version: "20.04",
		arch:    "amd64",
		cons:    "arch=amd64",
		itype:   "t3a.small",
		image:   "ami-02004133",
	}, {
		version: "20.04",
		arch:    "amd64",
		cons:    "instance-type=cc2.8xlarge",
		itype:   "cc2.8xlarge",
		image:   "ami-02004133",
	},
}

func (s *specSuite) TestFindInstanceSpec(c *tc.C) {
	size := len(findInstanceSpecTests)
	for i, test := range findInstanceSpecTests {
		c.Logf("\ntest %d of %d: %q; %q; %q; %q; %q; %v", i+1, size,
			test.version, test.arch, test.cons, test.itype, test.image,
			test.storage)
		stor := test.storage
		if len(stor) == 0 {
			stor = []string{ssdStorage, ebsStorage, ssdGP3Storage}
		}
		// We need to filter the image metadata to the test's
		// arches and series; the provisioner and bootstrap
		// code will do this.
		imageMetadata := filterImageMetadata(
			c, TestImageMetadata, test.version, test.arch,
		)
		base := corebase.MakeDefaultBase("ubuntu", test.version)
		spec, err := findInstanceSpec(
			context.Background(),
			false, // non-controller
			imageMetadata,
			testInstanceTypes,
			&instances.InstanceConstraint{
				Region:      "test",
				Base:        base,
				Arch:        test.arch,
				Constraints: constraints.MustParse(test.cons),
				Storage:     stor,
			})
		c.Assert(err, jc.ErrorIsNil)
		c.Check(spec.InstanceType.Name, tc.Equals, test.itype)
		c.Check(spec.Image.Id, tc.Equals, test.image)
	}
}

func (s *specSuite) TestFindInstanceSpecNotSetCpuPowerWhenInstanceTypeSet(c *tc.C) {
	instanceConstraint := &instances.InstanceConstraint{
		Region:      "test",
		Base:        version.DefaultSupportedLTSBase(),
		Constraints: constraints.MustParse("instance-type=t2.medium"),
	}

	c.Check(instanceConstraint.Constraints.CpuPower, tc.IsNil)
	_, err := findInstanceSpec(
		context.Background(),
		false, // non-controller
		TestImageMetadata,
		testInstanceTypes,
		instanceConstraint,
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(instanceConstraint.Constraints.CpuPower, tc.IsNil)
}

var findInstanceSpecErrorTests = []struct {
	base corebase.Base
	arch string
	cons string
	err  string
}{
	{
		base: version.DefaultSupportedLTSBase(),
		arch: "arm",
		err:  `no metadata for "ubuntu@24.04" images in test with arch arm`,
	}, {
		base: corebase.MakeDefaultBase("ubuntu", "15.04"),
		arch: "amd64",
		cons: "mem=4G",
		err:  `no metadata for \"ubuntu@15.04\" images in test with arch amd64`,
	}, {
		base: version.DefaultSupportedLTSBase(),
		arch: "amd64",
		cons: "instance-type=m1.small mem=4G",
		err:  `no instance types in test matching constraints "arch=amd64 instance-type=m1.small mem=4096M"`,
	},
}

func (s *specSuite) TestFindInstanceSpecErrors(c *tc.C) {
	for i, t := range findInstanceSpecErrorTests {
		c.Logf("test %d", i)
		// We need to filter the image metadata to the test's
		// arches and series; the provisioner and bootstrap
		// code will do this.
		imageMetadata := filterImageMetadata(
			c, TestImageMetadata, t.base.Channel.Track, t.arch,
		)
		_, err := findInstanceSpec(
			context.Background(),
			false, // non-controller
			imageMetadata,
			testInstanceTypes,
			&instances.InstanceConstraint{
				Region:      "test",
				Base:        t.base,
				Arch:        t.arch,
				Constraints: constraints.MustParse(t.cons),
			})
		c.Check(err, tc.ErrorMatches, t.err)
	}
}

func filterImageMetadata(
	c *tc.C,
	in []*imagemetadata.ImageMetadata,
	filterVersion string, filterArch string,
) []*imagemetadata.ImageMetadata {
	var imageMetadata []*imagemetadata.ImageMetadata
	for _, im := range in {
		if im.Version != filterVersion {
			continue
		}
		if im.Arch == filterArch {
			imageMetadata = append(imageMetadata, im)
		}
	}
	return imageMetadata
}

func (*specSuite) TestFilterImagesAcceptsNil(c *tc.C) {
	c.Check(filterImages(context.Background(), nil, nil), tc.HasLen, 0)
}

func (*specSuite) TestFilterImagesReturnsSelectively(c *tc.C) {
	good := imagemetadata.ImageMetadata{Id: "good", Storage: "ebs"}
	bad := imagemetadata.ImageMetadata{Id: "bad", Storage: "ftp"}
	input := []*imagemetadata.ImageMetadata{&good, &bad}
	expectation := []*imagemetadata.ImageMetadata{&good}

	ic := &instances.InstanceConstraint{Storage: []string{"ebs"}}
	c.Check(filterImages(context.Background(), input, ic), tc.DeepEquals, expectation)
}

func (*specSuite) TestFilterImagesMaintainsOrdering(c *tc.C) {
	input := []*imagemetadata.ImageMetadata{
		{Id: "one", Storage: "ebs"},
		{Id: "two", Storage: "ebs"},
		{Id: "three", Storage: "ebs"},
	}
	ic := &instances.InstanceConstraint{Storage: []string{"ebs"}}
	c.Check(filterImages(context.Background(), input, ic), tc.DeepEquals, input)
}
