package ec2

import (
	"bufio"
	"fmt"
	"launchpad.net/juju-core/environs"
	"net/http"
)

// imagesHost holds the address of the images http server.
// It is a variable so that tests can change it to refer to a local
// server when needed.
var imagesHost = "http://cloud-images.ubuntu.com"

// findInstanceSpec returns an InstanceSpec satisfying the supplied instanceConstraint.
func findInstanceSpec(ic *environs.InstanceConstraint) (*environs.InstanceSpec, error) {
	// first gather the instance types we are allowed to use.
	availableTypes, err := environs.GetInstanceTypes(ic, allInstanceTypes, allRegionCosts)
	if err != nil {
		return nil, err
	}

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
	return environs.FindInstanceSpec(r, ic, availableTypes)
}
