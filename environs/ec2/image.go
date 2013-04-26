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

// defaultCpuPower is larger the smallest instance's cpuPower, and no larger than
// any other instance type's cpuPower. It is used when no explicit CpuPower
// constraint exists, preventing the smallest instance from being chosen unless
// the user has clearly indicated that they are willing to accept poor performance.
var defaultCpuPower uint64 = 100

// findInstanceSpec returns an InstanceSpec satisfying the supplied instanceConstraint.
func findInstanceSpec(ic *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
	if ic.Constraints.CpuPower == nil {
		v := defaultCpuPower
		ic.Constraints.CpuPower = &v
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
	return instances.FindInstanceSpec(r, ic, allInstanceTypes, allRegionCosts)
}
