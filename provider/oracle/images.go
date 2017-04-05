// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"strings"

	"github.com/juju/errors"
	oci "github.com/juju/go-oracle-cloud/api"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
)

// instanceTypes returns all oracle cloud shapes and wraps them into
// instance.InstanceType for juju to understand and use them
// more information about oracle cloud shapes here
// https://docs.oracle.com/cloud/latest/stcomputecs/STCSG/GUID-1DD0FA71-AC7B-461C-B8C1-14892725AA69.htm#OCSUG210
func instanceTypes(c *oci.Client) ([]instances.InstanceType, error) {

	if c == nil {
		return nil, errors.Errorf("cannot use nil client")
	}

	// make api request to oracle cloud to give us a list of
	// all supported shapes
	shapes, err := c.AllShapes(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// so let's transform these into juju complaint
	// instance types
	onlyArch := []string{"amd64"}
	types := make([]instances.InstanceType, len(shapes.Result), len(shapes.Result))
	for key, val := range shapes.Result {
		types[key].Name = val.Name
		types[key].Arches = onlyArch
		types[key].Mem = val.Ram
		types[key].CpuCores = uint64(val.Cpus)
		types[key].RootDisk = val.Root_disk_size
	}

	return types, nil
}

// findInstanceSpec returns an *InstanceSpec, imagelist name
// satisfying the supplied instanceConstraint
func findInstanceSpec(
	c *oci.Client,
	allImageMetadata []*imagemetadata.ImageMetadata,
	instanceType []instances.InstanceType,
	ic *instances.InstanceConstraint,
) (*instances.InstanceSpec, string, error) {

	logger.Debugf("recived %d image(s)", len(allImageMetadata))

	images := instances.ImageMetadataToImages(allImageMetadata)
	spec, err := instances.FindInstanceSpec(images, ic, instanceType)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	imagelist, err := getImageName(c, spec.Image.Id)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	return spec, imagelist, nil
}

// checkImageList creates image metadata from the oracle image list
func checkImageList(
	c *oci.Client,
	cons constraints.Value,
) ([]*imagemetadata.ImageMetadata, error) {

	if c == nil {
		return nil, errors.NotFoundf("oracle client")
	}

	// take a list of all images that are in the oracle cloud account
	resp, err := c.AllImageLists(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// if we don't have any images that are in
	// the oracle cloud account under your username namespace
	// we should let the user know this
	n := len(resp.Result)
	if n == 0 {
		return nil, errors.NotFoundf(
			"images under the current client username are",
		)
	}

	images := make([]*imagemetadata.ImageMetadata, 0, n)
	for _, val := range resp.Result {
		list := strings.Split(val.Uri, "/")
		//TODO(sgiulitti): parse windows images as well
		//TODO: we expect images to be named in the following format:
		// OS.version.ARCH.timestamp
		// This may fail miserably, and may be an assumption that will not hold
		// in the future. Use simplestreams instead?
		// TODO(sgiulitti): (better use regexp)
		meta := strings.Split(list[len(list)-1], ".")
		if len(meta) < 4 {
			continue
		}

		endpoint := strings.Split(list[2], ".")
		images = append(images, &imagemetadata.ImageMetadata{
			Id:         meta[len(meta)-1],
			Arch:       meta[3],
			Endpoint:   "https://" + list[2],
			RegionName: endpoint[2],
			Version:    meta[1] + "." + meta[2],
		})
	}

	return images, nil
}

// getImageName based on the id from the imagemetadata extract
// only the name of the image
func getImageName(c *oci.Client, id string) (string, error) {
	if id == "" {
		return "", errors.NotFoundf("empty id")
	}

	resp, err := c.AllImageLists(nil)
	if err != nil {
		return "", errors.Trace(err)
	}

	// if we don't have any images that are in
	// the oracle cloud account under your username namespace
	// we should let the user know this
	if resp.Result == nil {
		return "", errors.NotFoundf(
			"images under the current client username are",
		)
	}

	for _, val := range resp.Result {
		if strings.Contains(val.Name, id) {
			s := strings.Split(val.Name, "/")
			return s[len(s)-1], nil
		}
	}

	return "", errors.NotFoundf("images found with the id provided are ")
}
