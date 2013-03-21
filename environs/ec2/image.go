package ec2

import (
	"bufio"
	"fmt"
	"io"
	"launchpad.net/juju-core/constraints"
	"net/http"
	"sort"
	"strings"
)

// instanceConstraint constrains the possible instances that may be
// chosen by the ec2 provider.
type instanceConstraint struct {
	region      string
	series      string
	arches      []string
	constraints constraints.Value
}

// instanceSpec holds an instance type name and the chosen image info.
type instanceSpec struct {
	instanceType string
	image        image
}

// findInstanceSpec returns an instanceSpec satisfying the supplied instanceConstraint.
func findInstanceSpec(ic *instanceConstraint) (*instanceSpec, error) {
	images, err := getImages(ic.region, ic.series, ic.arches)
	if err != nil {
		return nil, err
	}
	itypes, err := getInstanceTypes(ic.region, ic.constraints)
	if err != nil {
		return nil, err
	}
	for _, itype := range itypes {
		for _, image := range images {
			if image.match(itype) {
				return &instanceSpec{itype.name, image}, nil
			}
		}
	}
	names := make([]string, len(itypes))
	for i, itype := range itypes {
		names[i] = itype.name
	}
	return nil, fmt.Errorf("no %q images in %s matching instance types %v", ic.series, ic.region, names)
}

// image holds the attributes that vary amongst relevant images for
// a given series in a given region.
type image struct {
	id   string
	arch string
	// hvm is true when the image is built for an ec2 cluster instance type.
	hvm bool
}

// match returns true if the image can run on the supplied instance type.
func (image image) match(itype instanceType) bool {
	if image.hvm != itype.hvm {
		return false
	}
	for _, arch := range itype.arches {
		if arch == image.arch {
			return true
		}
	}
	return false
}

// imagesHost holds the address of the images http server.
// It is a variable so that tests can change it to refer to a local
// server when needed.
var imagesHost = "http://cloud-images.ubuntu.com"

// Columns in the file returned from the images server.
const (
	colSeries = iota
	colServer
	colDaily
	colDate
	colEBS
	colArch
	colRegion
	colImageId
	_
	_
	colVtype
	colMax
	// + more that we don't care about.
)

// getImages returns the latest released ubuntu server images for the
// supplied series in the supplied region.
func getImages(region, series string, arches []string) ([]image, error) {
	path := fmt.Sprintf("/query/%s/server/released.current.txt", series)
	hclient := new(http.Client)
	resp, err := hclient.Get(imagesHost + path)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			err = fmt.Errorf("%s", resp.Status)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get image data for %q: %v", series, err)
	}

	var images []image
	r := bufio.NewReader(resp.Body)
	for {
		line, _, err := r.ReadLine()
		if err == io.EOF {
			if len(images) == 0 {
				return nil, fmt.Errorf("no %q images in %s with arches %v", series, region, arches)
			}
			sort.Sort(byArch(images))
			return images, nil
		} else if err != nil {
			return nil, err
		}
		f := strings.Split(string(line), "\t")
		if len(f) < colMax {
			continue
		}
		if f[colRegion] != region {
			continue
		}
		if f[colEBS] != "ebs" {
			continue
		}
		if len(filterArches([]string{f[colArch]}, arches)) != 0 {
			images = append(images, image{
				id:   f[colImageId],
				arch: f[colArch],
				hvm:  f[colVtype] == "hvm",
			})
		}
	}
	panic("unreachable")
}

// byArch is used to sort a slice of images by architecture preference, such
// that amd64 images come ealier than i386 ones.
type byArch []image

func (ba byArch) Len() int      { return len(ba) }
func (ba byArch) Swap(i, j int) { ba[i], ba[j] = ba[j], ba[i] }
func (ba byArch) Less(i, j int) bool {
	return ba[i].arch == "amd64" && ba[j].arch != "amd64"
}
