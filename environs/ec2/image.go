package ec2

import (
	"bufio"
	"fmt"
	"launchpad.net/juju-core/environs/instances"
	"net/http"
)

// imagesHost holds the address of the images http server.
// It is a variable so that tests can change it to refer to a local
// server when needed.
var imagesHost = "http://cloud-images.ubuntu.com"

// findInstanceSpec returns an InstanceSpec satisfying the supplied instanceConstraint.
func findInstanceSpec(ic *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
	path := fmt.Sprintf("/query/%s/server/released.current.txt", ic.Series)
	resp, err := http.Get(imagesHost + path)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			err = fmt.Errorf("%s", resp.Status)
		}
	}
	var r *bufio.Reader
	if err == nil {
		r = bufio.NewReader(resp.Body)
	}
	return instances.FindInstanceSpec(r, ic, allInstanceTypes, allRegionCosts)
}
