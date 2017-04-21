// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/series"
	"github.com/juju/utils/set"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
)

var windowsServerMap = map[string]string{
	"Microsoft_Windows_Server_2012_R2": "win2012r2",
	"Microsoft_Windows_Server_2008_R2": "win2018r2",
}

// defaultImages is a list of official Ubuntu images available on Oracle cloud
// TODO (gsamfira): seed this from simplestreams
var defaultImages = []string{
	"Ubuntu.12.04-LTS.amd64.20170417",
	"Ubuntu.14.04-LTS.amd64.20170405",
	"Ubuntu.16.04-LTS.amd64.20170221",
	"Ubuntu.16.10.amd64.20170330",
}

// ensureImageInventory populates the image inventory for the current user
// with official Ubuntu images
func ensureImageInventory(c EnvironAPI) error {
	// TODO (gsamfira): add tests for this
	logger.Debugf("checking image inventory")
	images, err := c.AllImageLists(nil)
	if err != nil {
		return err
	}
	names := set.Strings{}
	for _, val := range images.Result {
		trimmed := strings.Split(val.Name, "/")
		names.Add(trimmed[len(trimmed)-1])
	}
	logger.Debugf("found %d images", names.Size())
	errs := []error{}
	for _, val := range defaultImages {
		if !names.Contains(val) {
			logger.Debugf("adding missing image: %s", val)
			imageName := c.ComposeName(val)
			listDetails, err := c.CreateImageList(1, val, imageName)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			// mirror the default attributes
			entryAttributes := map[string]interface{}{
				"type":            val,
				"defaultShape":    "oc2m",
				"minimumDiskSize": "10",
				"supportedShapes": "oc3,oc4,oc5,oc6,oc7,oc1m,oc2m,oc3m,oc4m,oc5m,ocio1m,ocio2m,ocio3m,ocio4m,ocio5m,ociog1k80,ociog2k80,ociog3k80",
			}
			_, err = c.CreateImageListEntry(
				listDetails.Name,
				entryAttributes,
				1, []string{imageName})
			if err != nil {
				errs = append(errs, err)
				// Cleanup list in case of error
				_ = c.DeleteImageList(listDetails.Name)
			}
		}
	}
	if len(errs) > 0 {
		return errors.Errorf("failed to add images to inventory: %v", errs)
	}
	return nil
}

// instanceTypes returns all oracle cloud shapes and wraps them into instance.InstanceType
// For more information about oracle cloud shapes, please see:
// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/api-Shapes.html
// https://docs.oracle.com/cloud/latest/stcomputecs/STCSG/GUID-1DD0FA71-AC7B-461C-B8C1-14892725AA69.htm#OCSUG210
func instanceTypes(c EnvironAPI) ([]instances.InstanceType, error) {
	if c == nil {
		return nil, errors.Errorf("cannot use nil client")
	}

	// fetch all shapes from the provider
	shapes, err := c.AllShapes(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// convert shapes to InstanceType
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
	c EnvironAPI,
	allImageMetadata []*imagemetadata.ImageMetadata,
	instanceType []instances.InstanceType,
	ic *instances.InstanceConstraint,
) (*instances.InstanceSpec, string, error) {

	logger.Debugf("received %d image(s): %v", len(allImageMetadata), allImageMetadata)
	version, err := series.SeriesVersion(ic.Series)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	filtered := []*imagemetadata.ImageMetadata{}
	for _, val := range allImageMetadata {
		if val.Version != version {
			continue
		}
		filtered = append(filtered, val)
	}

	images := instances.ImageMetadataToImages(filtered)
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

func parseImageName(name string, uri *url.URL) (*imagemetadata.ImageMetadata, error) {
	var id, arch, version string
	if strings.HasPrefix(name, "Ubuntu") {
		meta := strings.Split(name, ".")
		if len(meta) < 4 {
			return nil, errors.Errorf("invalid ubuntu image name: %s", name)
		}
		id = meta[len(meta)-1]
		arch = meta[3]
		version = meta[1] + "." + strings.TrimSuffix(meta[2], "-LTS")
	} else if strings.HasPrefix(name, "Microsoft") {
		if ver, ok := windowsServerMap[name]; ok {
			version = ver
			id = ver
			arch = "amd64"
		} else {
			return nil, errors.Errorf("unknown windows version: %q", name)
		}
	} else {
		return nil, errors.Errorf("could not determine OS from image name: %q", name)
	}

	tmp := strings.Split(uri.Host, ".")
	region := tmp[0]
	if len(tmp) > 1 {
		region = tmp[1]
	}
	return &imagemetadata.ImageMetadata{
		Id:         id,
		Arch:       arch,
		Endpoint:   fmt.Sprintf("%s://%s", uri.Scheme, uri.Host),
		RegionName: region,
		Version:    version,
	}, nil
}

// checkImageList creates image metadata from the oracle image list
func checkImageList(c EnvironAPI) ([]*imagemetadata.ImageMetadata, error) {
	if c == nil {
		return nil, errors.NotFoundf("oracle client")
	}

	err := ensureImageInventory(c)
	if err != nil {
		return nil, errors.Trace(err)
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
		uri, err := url.Parse(val.Uri)
		if err != nil {
			logger.Warningf("image with ID %q had invalid resource URI %q", val.Name, val.Uri)
			continue
		}
		requestUri := strings.Split(uri.RequestURI(), "/")
		if len(requestUri) == 0 {
			continue
		}
		name := requestUri[len(requestUri)-1]
		metadata, err := parseImageName(name, uri)
		if err != nil {
			logger.Warningf("failed to parse image name %s. Error was: %q", name, err)
			continue
		}
		logger.Infof("adding image %v to metadata", metadata.String())
		images = append(images, metadata)
	}
	return images, nil
}

// getImageName gets the name of the image represented by the supplied ID
func getImageName(c EnvironAPI, id string) (string, error) {
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
			"no usable images found in your account. Please add images from the oracle market",
		)
	}

	for _, val := range resp.Result {
		if strings.Contains(val.Name, id) {
			s := strings.Split(val.Name, "/")
			return s[len(s)-1], nil
		}
	}

	return "", errors.NotFoundf("image not found: %q", id)
}
