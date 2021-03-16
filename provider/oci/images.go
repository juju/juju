// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/core/series"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/oci/common"

	ociCore "github.com/oracle/oci-go-sdk/core"
)

const (
	BareMetal      InstanceType = "metal"
	VirtualMachine InstanceType = "vm"
	GPUMachine     InstanceType = "gpu"

	// ImageTypeVM should be run on a virtual instance
	ImageTypeVM ImageType = "vm"
	// ImageTypeBM should be run on bare metal
	ImageTypeBM ImageType = "metal"
	// ImageTypeGPU should be run on an instance with attached GPUs
	ImageTypeGPU ImageType = "gpu"
	// ImageTypeGeneric should work on any type of instance (bare metal or virtual)
	ImageTypeGeneric ImageType = "generic"

	windowsOS = "Windows"
	centOS    = "CentOS"
	ubuntuOS  = "Canonical Ubuntu"

	staleImageCacheTimeoutInMinutes = 30
)

var globalImageCache = &ImageCache{}
var cacheMutex = &sync.Mutex{}

type InstanceType string
type ImageType string

type ImageVersion struct {
	TimeStamp time.Time
	Revision  int
}

func setImageCache(cache *ImageCache) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	globalImageCache = cache
}

func NewImageVersion(img ociCore.Image) (ImageVersion, error) {
	var imgVersion ImageVersion
	if img.DisplayName == nil {
		return imgVersion, errors.Errorf("image does not have a display name")
	}
	fields := strings.Split(*img.DisplayName, "-")
	if len(fields) < 2 {
		return imgVersion, errors.Errorf("invalid image display name %q", *img.DisplayName)
	}
	timeStamp, err := time.Parse("2006.01.02", fields[len(fields)-2])
	if err != nil {
		return imgVersion, errors.Annotatef(err, "parsing time for %q", *img.DisplayName)
	}

	revision, err := strconv.Atoi(fields[len(fields)-1])

	if err != nil {
		return imgVersion, errors.Annotatef(err, "parsing revision for %q", *img.DisplayName)
	}

	imgVersion.TimeStamp = timeStamp
	imgVersion.Revision = revision
	return imgVersion, nil
}

// InstanceImage aggregates information pertinent to provider supplied
// images (eg: shapes it ca run on, type of instance it can run on, etc)
type InstanceImage struct {
	// ImageType determines which type of image this is. Valid values are:
	// vm, baremetal and generic
	ImageType ImageType
	// Id is the provider ID of the image
	Id string
	// Series is the series as known by juju
	Series string
	// Version is the version of the image
	Version ImageVersion
	// Raw stores the core.Image object
	Raw ociCore.Image

	// CompartmentId is the compartment Id where this image is available
	CompartmentId *string

	// InstanceTypes holds a list of shapes compatible with this image
	InstanceTypes []instances.InstanceType
}

func (i *InstanceImage) SetInstanceTypes(types []instances.InstanceType) {
	i.InstanceTypes = types
}

// byVersion sorts shapes by version number
type byVersion []InstanceImage

func (t byVersion) Len() int {
	return len(t)
}

func (t byVersion) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}

func (t byVersion) Less(i, j int) bool {
	// Sort in reverse order. Newer versions first in array
	if t[i].Version.TimeStamp.Before(t[j].Version.TimeStamp) {
		return false
	}
	if t[i].Version.TimeStamp.Equal(t[j].Version.TimeStamp) {
		if t[i].Version.Revision < t[j].Version.Revision {
			return false
		}
	}
	return true
}

// ImageCache holds a cache of all provider images for a fixed
// amount of time before it becomes stale
type ImageCache struct {
	// images []InstanceImage
	images map[string][]InstanceImage

	// shapeToInstanceImageMap map[string][]InstanceImage

	lastRefresh time.Time
}

func (i *ImageCache) ImageMap() map[string][]InstanceImage {
	return i.images
}

// SetLastRefresh sets the lastRefresh attribute of ImageCache
// This is used mostly for testing purposes
func (i *ImageCache) SetLastRefresh(t time.Time) {
	i.lastRefresh = t
}

func (i *ImageCache) SetImages(images map[string][]InstanceImage) {
	i.images = images
}

func (i *ImageCache) isStale() bool {
	threshold := i.lastRefresh.Add(staleImageCacheTimeoutInMinutes * time.Minute)
	now := time.Now()
	if now.After(threshold) {
		return true
	}
	return false
}

// ImageMetadata returns an array of imagemetadata.ImageMetadata for
// all images that are currently in cache, matching the provided series
// If defaultVirtType is specified, all generic images will inherit the
// value of defaultVirtType.
func (i ImageCache) ImageMetadata(series string, defaultVirtType string) []*imagemetadata.ImageMetadata {
	var metadata []*imagemetadata.ImageMetadata

	images, ok := i.images[series]
	if !ok {
		return metadata
	}
	for _, val := range images {
		if val.ImageType == ImageTypeGeneric {
			if defaultVirtType != "" {
				val.ImageType = ImageType(defaultVirtType)
			} else {
				val.ImageType = ImageTypeVM
			}
		}
		imgMeta := &imagemetadata.ImageMetadata{
			Id:   val.Id,
			Arch: "amd64",
			// TODO(gsamfira): add region name?
			// RegionName: region,
			Version:  val.Series,
			VirtType: string(val.ImageType),
		}
		metadata = append(metadata, imgMeta)
	}

	return metadata
}

// SupportedShapes returns the InstanceTypes available for images matching
// the supplied series
func (i ImageCache) SupportedShapes(series string) []instances.InstanceType {
	matches := map[string]int{}
	ret := []instances.InstanceType{}
	// TODO(gsamfira): Find a better way for this.
	images, ok := i.images[series]
	if !ok {
		return ret
	}
	for _, img := range images {
		for _, instType := range img.InstanceTypes {
			if _, ok := matches[instType.Name]; !ok {
				matches[instType.Name] = 1
				ret = append(ret, instType)
			}
		}
	}
	return ret
}

func getImageType(img ociCore.Image) ImageType {
	if img.DisplayName == nil {
		return ImageTypeGeneric
	}
	name := strings.ToLower(*img.DisplayName)
	if strings.Contains(name, "-vm-") {
		return ImageTypeVM
	}
	if strings.Contains(name, "-bm-") {
		return ImageTypeBM
	}
	if strings.Contains(name, "-gpu-") {
		return ImageTypeGPU
	}
	return ImageTypeGeneric
}

func getCentOSSeries(img ociCore.Image) (string, error) {
	if img.OperatingSystemVersion == nil || *img.OperatingSystem != centOS {
		return "", errors.NotSupportedf("invalid Operating system")
	}
	splitVersion := strings.Split(*img.OperatingSystemVersion, ".")
	if len(splitVersion) < 1 {
		return "", errors.NotSupportedf("invalid centOS version: %v", *img.OperatingSystemVersion)
	}
	tmpVersion := fmt.Sprintf("%s%s", strings.ToLower(*img.OperatingSystem), splitVersion[0])

	// call series.CentOSVersionSeries to validate that the version
	// of CentOS is supported by juju
	logger.Tracef("Determining CentOS series for: %s", tmpVersion)
	return series.CentOSVersionSeries(tmpVersion)
}

func NewInstanceImage(img ociCore.Image, compartmentID *string) (imgType InstanceImage, err error) {
	var imgSeries string
	switch osVersion := *img.OperatingSystem; osVersion {
	case windowsOS:
		tmp := fmt.Sprintf("%s %s", *img.OperatingSystem, *img.OperatingSystemVersion)
		logger.Tracef("Determining Windows series for: %s", tmp)
		imgSeries, err = series.WindowsVersionSeries(tmp)
	case centOS:
		imgSeries, err = getCentOSSeries(img)
	case ubuntuOS:
		logger.Tracef("Determining Ubuntu series for: %s", *img.OperatingSystemVersion)
		imgSeries, err = series.VersionSeries(*img.OperatingSystemVersion)
	default:
		return imgType, errors.NotSupportedf("os %s", osVersion)
	}

	if err != nil {
		return imgType, err
	}

	imgType.ImageType = getImageType(img)
	imgType.Id = *img.Id
	imgType.Series = imgSeries
	imgType.Raw = img
	imgType.CompartmentId = compartmentID

	version, err := NewImageVersion(img)
	if err != nil {
		return imgType, err
	}
	imgType.Version = version

	return imgType, nil
}

func instanceTypes(cli common.OCIComputeClient, compartmentID, imageID *string) ([]instances.InstanceType, error) {
	if cli == nil {
		return nil, errors.Errorf("cannot use nil client")
	}

	request := ociCore.ListShapesRequest{
		CompartmentId: compartmentID,
		ImageId:       imageID,
	}
	// fetch all shapes from the provider
	shapes, err := cli.ListShapes(context.Background(), request)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// convert shapes to InstanceType
	arch := []string{"amd64"}
	types := []instances.InstanceType{}
	for _, val := range shapes.Items {
		spec, ok := shapeSpecs[*val.Shape]
		if !ok {
			logger.Debugf("shape %s does not have a mapping", *val.Shape)
			continue
		}
		instanceType := string(spec.Type)
		newType := instances.InstanceType{
			Name:     *val.Shape,
			Arches:   arch,
			Mem:      uint64(spec.Memory),
			CpuCores: uint64(spec.Cpus),
			// its not really virtualization type. We have just 3 types of images:
			// bare metal, virtual and generic (works on metal and VM).
			VirtType: &instanceType,
		}
		types = append(types, newType)
	}
	return types, nil
}

func refreshImageCache(cli common.OCIComputeClient, compartmentID *string) (*ImageCache, error) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	if globalImageCache.isStale() == false {
		return globalImageCache, nil
	}

	request := ociCore.ListImagesRequest{
		CompartmentId: compartmentID,
	}
	response, err := cli.ListImages(context.Background(), request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing provider images")
	}

	images := map[string][]InstanceImage{}

	for _, val := range response.Items {
		instTypes, err := instanceTypes(cli, compartmentID, val.Id)
		if err != nil {
			return nil, err
		}
		img, err := NewInstanceImage(val, compartmentID)
		if err != nil {
			if val.Id != nil {
				logger.Debugf("error parsing image %q: %q", *val.Id, err)
			} else {
				logger.Debugf("error parsing image %q", err)
			}
			continue
		}
		img.SetInstanceTypes(instTypes)
		images[img.Series] = append(images[img.Series], img)
	}
	for v := range images {
		sort.Sort(byVersion(images[v]))
	}
	globalImageCache = &ImageCache{
		images:      images,
		lastRefresh: time.Now(),
	}
	return globalImageCache, nil
}

// findInstanceSpec returns an *InstanceSpec, imagelist name
// satisfying the supplied instanceConstraint
func findInstanceSpec(
	allImageMetadata []*imagemetadata.ImageMetadata,
	instanceType []instances.InstanceType,
	ic *instances.InstanceConstraint,
) (*instances.InstanceSpec, string, error) {

	logger.Debugf("received %d image(s): %v", len(allImageMetadata), allImageMetadata)
	filtered := []*imagemetadata.ImageMetadata{}
	// Filter by series. imgCache.supportedShapes() and
	// imgCache.imageMetadata() will return filtered values
	// by series already. This additional filtering is done
	// in case someone wants to use this function with values
	// not returned by the above two functions
	for _, val := range allImageMetadata {
		if val.Version != ic.Series {
			continue
		}
		filtered = append(filtered, val)
	}

	images := instances.ImageMetadataToImages(filtered)
	spec, err := instances.FindInstanceSpec(images, ic, instanceType)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	return spec, spec.Image.Id, nil
}
