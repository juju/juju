package ec2

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"
	"launchpad.net/juju/go/log"
	"launchpad.net/juju/go/environs"
)

// instanceConstraint constrains the possible instances that may be
// chosen by the ec2 provider.
type instanceConstraint struct {
	series            string // Ubuntu release name.
	arch              string
	persistentStorage bool
	region            string
	daily             bool
	desktop           bool
}

var defaultInstanceConstraint = &instanceConstraint{
	series:            environs.CurrentSeries,
	arch:              environs.CurrentArch,
	persistentStorage: true,
	region:            "us-east-1",
	daily:             false,
	desktop:           false,
}

// instanceSpec specifies a particular kind of instance.
type instanceSpec struct {
	imageId string
	arch    string
	series  string
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
	_
	_
	colVtype
	colMax
	// + more that we don't care about.
)

// fndInstanceSpec finds a suitable instance specification given
// the provided constraints.
func findInstanceSpec(spec *instanceConstraint) (*instanceSpec, error) {
	hclient := new(http.Client)
	uri := fmt.Sprintf(imagesHost+"/query/%s/%s/%s.current.txt",
		spec.series,
		either(spec.desktop, "desktop", "server"), // variant.
		either(spec.daily, "daily", "released"),   // version.
	)
	resp, err := hclient.Get(uri)
	if err == nil && resp.StatusCode != 200 {
		err = fmt.Errorf("%s", resp.Status)
	}
	if err != nil {
		return nil, fmt.Errorf("error getting instance types: %v", err)
	}
	defer resp.Body.Close()
	ebsMatch := either(spec.persistentStorage, "ebs", "instance-store")

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
		if f[colVtype] == "hvm" {
			continue
		}
		if f[colEBS] != ebsMatch {
			continue
		}
		if f[colArch] == spec.arch && f[colRegion] == spec.region {
			log.Printf("choosing image from fields %q", f)
			return &instanceSpec{
				imageId: f[colImageId],
				arch:    spec.arch,
				series:  spec.series,
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
