package ec2

import (
	"bufio"
	"fmt"
	"launchpad.net/juju/go/version"
	"net/http"
	"strings"
)

// TODO implement constraints properly.

// InstanceConstraint specifies a range of possible instances
// and the images that can run on them.
type InstanceConstraint struct {
	UbuntuRelease     string
	Arch              string
	PersistentStorage bool
	Region            string
	Daily             bool
	Desktop           bool
}

var DefaultInstanceConstraint = &InstanceConstraint{
	UbuntuRelease:     "precise",
	Arch:              version.CurrentArch,
	PersistentStorage: true,
	Region:            "us-east-1",
	Daily:             false,
	Desktop:           false,
}

// InstanceSpec specifies a particular machine type and the OS that
// will run on it.
type InstanceSpec struct {
	ImageId string
	Arch    string // The architecture the image will run on.
	OS      string // The OS the image will run on.
}

// imagesHost holds the address of the images http server.
// It is a variable so that tests can change it to refer to a local
// server when needed.
var imagesHost = "http://uec-images.ubuntu.com"

// FindInstanceSpec finds a suitable instance specification given
// the specified constraints.
func FindInstanceSpec(spec *InstanceConstraint) (*InstanceSpec, error) {
	hclient := new(http.Client)
	uri := fmt.Sprintf(imagesHost+"/query/%s/%s/%s.current.txt",
		spec.UbuntuRelease,
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
		if len(f) < 11 {
			continue
		}
		// TODO hvm heuristics (see python code)
		if f[10] != "paravirtual" {
			continue
		}
		if f[4] != ebsMatch {
			continue
		}
		if f[5] == spec.Arch && f[6] == spec.Region {
			return &InstanceSpec{
				ImageId: f[7],
				Arch:    spec.Arch,
				OS:      "linux",
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
