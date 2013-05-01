package ec2

import (
	"fmt"
	"io"
	"launchpad.net/juju-core/environs/instances"
	"net/http"
)

// imagesHost holds the address of the images http server.
// It is a variable so that tests can change it to refer to a local
// server when needed.
var baseImagesUrl = "http://cloud-images.ubuntu.com/eightprotons"

// defaultCpuPower is larger the smallest instance's cpuPower, and no larger than
// any other instance type's cpuPower. It is used when no explicit CpuPower
// constraint exists, preventing the smallest instance from being chosen unless
// the user has clearly indicated that they are willing to accept poor performance.
const defaultCpuPower = 100

// findInstanceSpec returns an InstanceSpec satisfying the supplied instanceConstraint.
func findInstanceSpec(ic *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
	if ic.Constraints.CpuPower == nil {
		ic.Constraints.CpuPower = instances.CpuPower(defaultCpuPower)
	}
	imageFileUrl := baseImagesUrl + "/streams/v1/com.ubuntu.cloud:released:aws.js"
	resp, err := http.Get(imageFileUrl)
	var r io.Reader
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			err = fmt.Errorf("%s", resp.Status)
		}
		if err == nil {
			r = resp.Body
		}
	}

	// Make a copy of the known EC2 instance types, filling in the cost for the specified region.
	regionCosts := allRegionCosts[ic.Region]
	if len(regionCosts) == 0 && len(allRegionCosts) > 0 {
		return nil, fmt.Errorf("no instance types found in %s", ic.Region)
	}

	var itypesWithCosts []instances.InstanceType
	for _, itype := range allInstanceTypes {
		cost, ok := regionCosts[itype.Name]
		if !ok {
			continue
		}
		itWithCost := itype
		itWithCost.Cost = cost
		itypesWithCosts = append(itypesWithCosts, itWithCost)
	}
	return instances.FindInstanceSpec(r, ic, itypesWithCosts)
}
