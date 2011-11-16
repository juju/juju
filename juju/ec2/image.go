package ec2

import (
	"bufio"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ImageConstraint specifies a range of possible machine images.
// TODO allow specification of softer constraints.
type ImageConstraint struct {
	UbuntuRelease     string
	Architecture      string
	PersistentStorage bool
	Region            string
	Daily             bool
	Desktop           bool
}

var DefaultImageConstraint = &ImageConstraint{
	UbuntuRelease:     "oneiric",
	Architecture:      "i386",
	PersistentStorage: true,
	Region:            "us-east-1",
	Daily:             false,
	Desktop:           false,
}

type ImageSpec struct {
	ImageId string
}

func (*conn) FindImageSpec(spec *ImageConstraint) (*ImageSpec, error) {
	// note: original get_image_id added three optional args:
	// DefaultImageId		if found, returns that immediately
	// Region				overrides spec.Region
	// DefaultSeries		used if spec.UbuntuRelease is ""

	hclient := new(http.Client)
	uri := fmt.Sprintf("http://uec-images.ubuntu.com/query/%s/%s/%s.current.txt",
		spec.UbuntuRelease,
		either(spec.Desktop, "desktop", "server"), // variant.
		either(spec.Daily, "daily", "released"),   // version.
	)
	resp, err := hclient.Get(uri)
	if err != nil {
		return nil, fmt.Errorf("error getting instance types: %v", err)
	}
	defer resp.Body.Close()
	ebsMatch := either(spec.PersistentStorage, "ebs", "instance-store")
	r := bufio.NewReader(resp.Body)
	for {
		line, _, err := r.ReadLine()
		if err != nil {
			return nil, errors.New("cannot find matching instance")
		}
		f := strings.Split(string(line), "\t")
		if len(f) < 8 {
			continue
		}
		if f[4] != ebsMatch {
			continue
		}
		if f[5] == spec.Architecture && f[6] == spec.Region {
			return &ImageSpec{f[7]}, nil
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
