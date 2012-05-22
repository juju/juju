package ec2

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"
)

// ImageConstraint specifies a range of possible machine images.
// TODO allow specification of softer constraints?
type ImageConstraint struct {
	Series            string // Ubuntu release name.
	Arch              string
	PersistentStorage bool
	Region            string
	Daily             bool
	Desktop           bool
}

var DefaultImageConstraint = &ImageConstraint{
	Series:            "oneiric",
	Arch:              "i386",
	PersistentStorage: true,
	Region:            "us-east-1",
	Daily:             false,
	Desktop:           false,
}

type ImageSpec struct {
	ImageId string
	Arch    string // The architecture the image will run on.
	Series  string // The Ubuntu series the image will run on.
}

// imagesHost holds the address of the images http server.
// It is a variable so that tests can change it to refer to a local
// server when needed.
var imagesHost = "http://uec-images.ubuntu.com"

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
	// + more that we don't care about.
	colMax
)

func FindImageSpec(spec *ImageConstraint) (*ImageSpec, error) {
	hclient := new(http.Client)
	uri := fmt.Sprintf(imagesHost+"/query/%s/%s/%s.current.txt",
		spec.Series,
		either(spec.Desktop, "desktop", "server"), // variant.
		either(spec.Daily, "daily", "released"),   // version.
	)
	resp, err := hclient.Get(uri)
	if err == nil && resp.StatusCode != 200 {
		err = fmt.Errorf("%s", resp.Status)
	}
	if err != nil {
		return nil, fmt.Errorf("error getting instance types: %v", err)
	}
	defer resp.Body.Close()
	ebsMatch := either(spec.PersistentStorage, "ebs", "instance-store")

	r := bufio.NewReader(resp.Body)
	for {
		line, _, err := r.ReadLine()
		if err != nil {
			return nil, fmt.Errorf("cannot find matching image: %v", err)
		}
		f := strings.Split(string(line), "\t")
		if len(f) < colMax {
			continue
		}
		if f[colEBS] != ebsMatch {
			continue
		}
		if f[colArch] == spec.Arch && f[colRegion] == spec.Region {
			return &ImageSpec{
				ImageId: f[colImageId],
				Arch:    spec.Arch,
				Series:  spec.Series,
			}, nil
		}
	}
	panic("not reached")
}

func either(yes bool, a, b string) string {
	if yes {
		return a
	}
	return b
}
