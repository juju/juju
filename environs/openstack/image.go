package openstack

import (
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/instances"
)

// findInstanceSpec returns an image and instance type satisfying the constraint.
// The instance type comes from querying the flavors supported by the deployment.
func findInstanceSpec(e *environ, ic *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
	// first construct all available instance types from the supported flavors.
	nova := e.nova()
	flavors, err := nova.ListFlavorsDetail()
	if err != nil {
		return nil, err
	}
	allInstanceTypes := []instances.InstanceType{}
	for _, flavor := range flavors {
		instanceType := instances.InstanceType{
			Id:       flavor.Id,
			Name:     flavor.Name,
			Arches:   ic.Arches,
			Mem:      uint64(flavor.RAM),
			CpuCores: uint64(flavor.VCPUs),
			Cost:     uint64(flavor.RAM),
		}
		allInstanceTypes = append(allInstanceTypes, instanceType)
	}

	cloudSpec := imagemetadata.CloudSpec{ic.Region, e.ecfg().authURL()}
	prodSpec := imagemetadata.NewProductSpec(ic.Series, ic.Arches, "")
	baseURLs, err := e.getImageBaseURLs()
	if err != nil {
		return nil, err
	}
	matchingImages, err := imagemetadata.GetImageIdMetadata(baseURLs, imagemetadata.DefaultIndexPath, &cloudSpec, &prodSpec)
	if err != nil {
		return nil, err
	}
	var images []instances.Image
	for _, imageMetadata := range matchingImages {
		im := *imageMetadata
		images = append(images, instances.Image{
			Id:    im.Id,
			VType: im.VType,
			Arch:  im.Arch,
		})
	}
	spec, err := instances.FindInstanceSpec(images, ic, allInstanceTypes)
	if err != nil {
		return nil, err
	}
	return spec, nil
}
