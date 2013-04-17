package environs

import (
	"bufio"
	"bytes"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
	"strings"
)

type imageSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&imageSuite{})

func (s *imageSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
}

func (s *imageSuite) TearDownSuite(c *C) {
	s.LoggingSuite.TearDownTest(c)
}

var imagesData = imagesFields(
	"instance-store amd64 us-east-1 ami-00000011 paravirtual",
	"ebs amd64 eu-west-1 ami-00000016 paravirtual",
	"ebs i386 ap-northeast-1 ami-00000023 paravirtual",
	"ebs amd64 ap-northeast-1 ami-00000026 paravirtual",
	"ebs amd64 ap-northeast-1 ami-00000087 hvm",
	"ebs amd64 test ami-00000033 paravirtual",
	"ebs i386 test ami-00000034 paravirtual",
	"ebs amd64 test ami-00000035 hvm",
)

func imagesFields(srcs ...string) string {
	strs := make([]string, len(srcs))
	for i, src := range srcs {
		parts := strings.Split(src, " ")
		if len(parts) != 5 {
			panic("bad clouddata field input")
		}
		args := make([]interface{}, len(parts))
		for i, part := range parts {
			args[i] = part
		}
		// Ignored fields are left empty for clarity's sake, and two additional
		// tabs are tacked on to the end to verify extra columns are ignored.
		strs[i] = fmt.Sprintf("\t\t\t\t%s\t%s\t%s\t%s\t\t\t%s\t\t\n", args...)
	}
	return strings.Join(strs, "")
}

var getImagesTests = []struct {
	region string
	series string
	arches []string
	images []Image
	err    string
}{
	{
		region: "us-east-1",
		series: "precise",
		arches: Both,
		err:    `no "precise" images in us-east-1 with arches \[amd64 i386\]`,
	}, {
		region: "eu-west-1",
		series: "precise",
		arches: []string{"i386"},
		err:    `no "precise" images in eu-west-1 with arches \[i386\]`,
	}, {
		region: "ap-northeast-1",
		series: "precise",
		arches: Both,
		images: []Image{
			{"ami-00000026", "amd64", false},
			{"ami-00000087", "amd64", true},
			{"ami-00000023", "i386", false},
		},
	}, {
		region: "ap-northeast-1",
		series: "precise",
		arches: []string{"amd64"},
		images: []Image{
			{"ami-00000026", "amd64", false},
			{"ami-00000087", "amd64", true},
		},
	}, {
		region: "ap-northeast-1",
		series: "precise",
		arches: []string{"i386"},
		images: []Image{
			{"ami-00000023", "i386", false},
		},
	},
}

func (s *imageSuite) TestGetImages(c *C) {
	for i, t := range getImagesTests {
		c.Logf("test %d", i)
		r := bufio.NewReader(bytes.NewBufferString(imagesData))
		images, err := getImages(r, t.region, t.series, "ebs", t.arches)
		if t.err != "" {
			c.Check(err, ErrorMatches, t.err)
			continue
		}
		if !c.Check(err, IsNil) {
			continue
		}
		c.Check(images, DeepEquals, t.images)
	}
}

var imageMatchtests = []struct {
	image Image
	itype InstanceType
	match bool
}{
	{
		image: Image{Arch: "amd64"},
		itype: InstanceType{Arches: []string{"amd64"}},
		match: true,
	}, {
		image: Image{Arch: "amd64"},
		itype: InstanceType{Arches: []string{"i386", "amd64"}},
		match: true,
	}, {
		image: Image{Arch: "amd64", Hvm: true},
		itype: InstanceType{Arches: []string{"amd64"}, Hvm: true},
		match: true,
	}, {
		image: Image{Arch: "i386"},
		itype: InstanceType{Arches: []string{"amd64"}},
	}, {
		image: Image{Arch: "amd64", Hvm: true},
		itype: InstanceType{Arches: []string{"amd64"}},
	}, {
		image: Image{Arch: "amd64"},
		itype: InstanceType{Arches: []string{"amd64"}, Hvm: true},
	},
}

func (s *imageSuite) TestImageMatch(c *C) {
	for i, t := range imageMatchtests {
		c.Logf("test %d", i)
		c.Check(t.image.match(t.itype), Equals, t.match)
	}
}
