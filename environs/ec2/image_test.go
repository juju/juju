package ec2_test

import (
	"fmt"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs/ec2"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type imageSuite struct{}

var _ = Suite(imageSuite{})

// N.B. the image IDs in this test will need updating
// if the image directory is regenerated.
var imageTests = []struct {
	constraint ec2.ImageConstraint
	imageId    string
	err        string
}{
	{*ec2.DefaultImageConstraint, "ami-a7f539ce", ""},
	{ec2.ImageConstraint{
		UbuntuRelease:     "natty",
		Architecture:      "amd64",
		PersistentStorage: false,
		Region:            "eu-west-1",
		Daily:             true,
		Desktop:           true,
	}, "ami-19fdc16d", ""},
	{ec2.ImageConstraint{
		UbuntuRelease:     "natty",
		Architecture:      "i386",
		PersistentStorage: true,
		Region:            "ap-northeast-1",
		Daily:             true,
		Desktop:           true,
	}, "ami-cc9621cd", ""},
	{ec2.ImageConstraint{
		UbuntuRelease:     "natty",
		Architecture:      "i386",
		PersistentStorage: false,
		Region:            "ap-northeast-1",
		Daily:             true,
		Desktop:           true,
	}, "ami-62962163", ""},
	{ec2.ImageConstraint{
		UbuntuRelease:     "natty",
		Architecture:      "amd64",
		PersistentStorage: false,
		Region:            "ap-northeast-1",
		Daily:             true,
		Desktop:           true,
	}, "ami-a69621a7", ""},
	{ec2.ImageConstraint{
		UbuntuRelease:     "zingy",
		Architecture:      "amd64",
		PersistentStorage: false,
		Region:            "eu-west-1",
		Daily:             true,
		Desktop:           true,
	}, "", "error getting instance types:.*"},
}

func (imageSuite) TestFindImageSpec(c *C) {
	// set up http so that all requests will be satisfied from the images directory.
	defer setTransport(setTransport(http.NewFileTransport(http.Dir("images"))))

	for i, t := range imageTests {
		id, err := ec2.FindImageSpec(&t.constraint)
		if t.err != "" {
			c.Check(err, ErrorMatches, t.err, Commentf("test %d", i))
			c.Check(id, IsNil, Commentf("test %d", i))
			continue
		}
		if !c.Check(err, IsNil, Commentf("test %d", i)) {
			continue
		}
		if !c.Check(id, NotNil, Commentf("test %d", i)) {
			continue
		}
		c.Check(id.ImageId, Equals, t.imageId)
	}
}

func setTransport(t http.RoundTripper) (old http.RoundTripper) {
	old = http.DefaultTransport
	http.DefaultTransport = t
	return
}

// regenerate all data inside the images directory.
// N.B. this second-guesses the logic inside images.go
func regenerateImages(t *testing.T) {
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
