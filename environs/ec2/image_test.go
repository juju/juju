package ec2

import (
	"fmt"
	"io"
	. "launchpad.net/gocheck"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type imageSuite struct{}

var _ = Suite(imageSuite{})

func (imageSuite) SetUpSuite(c *C) {
	UseTestImageData(true)
}

func (imageSuite) TearDownSuite(c *C) {
	UseTestImageData(false)
}

// N.B. the image IDs in this test will need updating
// if the image directory is regenerated.
var imageTests = []struct {
	constraint instanceConstraint
	imageId    string
	err        string
}{
	{*defaultInstanceConstraint, "ami-a7f539ce", ""},
	{instanceConstraint{
		series:            "natty",
		arch:              "amd64",
		persistentStorage: false,
		region:            "eu-west-1",
		daily:             true,
		desktop:           true,
	}, "ami-19fdc16d", ""},
	{instanceConstraint{
		series:            "natty",
		arch:              "i386",
		persistentStorage: true,
		region:            "ap-northeast-1",
		daily:             true,
		desktop:           true,
	}, "ami-cc9621cd", ""},
	{instanceConstraint{
		series:            "natty",
		arch:              "i386",
		persistentStorage: false,
		region:            "ap-northeast-1",
		daily:             true,
		desktop:           true,
	}, "ami-62962163", ""},
	{instanceConstraint{
		series:            "natty",
		arch:              "amd64",
		persistentStorage: false,
		region:            "ap-northeast-1",
		daily:             true,
		desktop:           true,
	}, "ami-a69621a7", ""},
	{instanceConstraint{
		series:            "zingy",
		arch:              "amd64",
		persistentStorage: false,
		region:            "eu-west-1",
		daily:             true,
		desktop:           true,
	}, "", "error getting instance types:.*"},
}

func (imageSuite) TestFindInstanceSpec(c *C) {
	for i, t := range imageTests {
		c.Logf("test %d", i)
		id, err := findInstanceSpec(&t.constraint)
		if t.err != "" {
			c.Check(err, ErrorMatches, t.err)
			c.Check(id, IsNil)
			continue
		}
		if !c.Check(err, IsNil) {
			continue
		}
		if !c.Check(id, NotNil) {
			continue
		}
		c.Check(id.imageId, Equals, t.imageId)
		c.Check(id.arch, Equals, t.constraint.arch)
		c.Check(id.series, Equals, t.constraint.series)
	}
}

// regenerate all data inside the images directory.
// N.B. this second-guesses the logic inside images.go
func RegenerateImages(t *testing.T) {
	if err := os.RemoveAll(imagesRoot); err != nil {
		t.Errorf("cannot remove old images: %v", err)
		return
	}
	for _, variant := range []string{"desktop", "server"} {
		for _, version := range []string{"daily", "released"} {
			for _, release := range []string{"natty", "oneiric"} {
				s := fmt.Sprintf("query/%s/%s/%s.current.txt", release, variant, version)
				t.Logf("regenerating images from %q", s)
				err := copylocal(s)
				if err != nil {
					t.Logf("regenerate: %v", err)
				}
			}
		}
	}
}

var imagesRoot = "images"

func copylocal(s string) error {
	r, err := http.Get("http://uec-images.ubuntu.com/" + s)
	if err != nil {
		return fmt.Errorf("get %q: %v", s, err)
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("status on %q: %s", s, r.Status)
	}
	path := filepath.Join(filepath.FromSlash(imagesRoot), filepath.FromSlash(s))
	d, _ := filepath.Split(path)
	if err := os.MkdirAll(d, 0777); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, r.Body)
	if err != nil {
		return fmt.Errorf("error copying image file: %v", err)
	}
	return nil
}
