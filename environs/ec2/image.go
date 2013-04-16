package ec2

import (
	"bufio"
	"fmt"
	"net/http"
	"launchpad.net/juju-core/environs"
)

// imagesHost holds the address of the images http server.
// It is a variable so that tests can change it to refer to a local
// server when needed.
var imagesHost = "http://cloud-images.ubuntu.com"

// findInstanceSpec returns an InstanceSpec satisfying the supplied instanceConstraint.
func findInstanceSpec(ic *environs.InstanceConstraint) (*environs.InstanceSpec, error) {
	path := fmt.Sprintf("/query/%s/server/released.current.txt", ic.Series)
	resp, err := http.Get(imagesHost + path)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			err = fmt.Errorf("%s", resp.Status)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get image data for %q: %v", ic.Series, err)
	}
	availableTypes, err := environs.GetInstanceTypes(ic.Region, ic.Constraints, allInstanceTypes, allRegionCosts)
	if err != nil {
		return nil, fmt.Errorf("cannot get instance types for %q: %v", ic.Series, err)
	}
	r := bufio.NewReader(resp.Body)
	return environs.FindInstanceSpec(r, ic, availableTypes)
}
