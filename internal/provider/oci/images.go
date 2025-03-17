// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	ociCore "github.com/oracle/oci-go-sdk/v65/core"

	corearch "github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
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

	ubuntuOS = "Canonical Ubuntu"

	staleImageCacheTimeoutInMinutes = 30
)

var globalImageCache = &ImageCache{}
var cacheMutex = &sync.Mutex{}

type InstanceType string

func (i InstanceType) String() string {
	return string(i)
}

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
// images (eg: shapes it can run on, type of instance it can run on, etc)
type InstanceImage struct {
	// ImageType determines which type of image this is. Valid values are:
	// vm, baremetal and generic
	ImageType ImageType
	// Id is the provider ID of the image
	Id string
	// Base is the os base.
	Base corebase.Base
	// Version is the version of the image
	Version ImageVersion
	// Raw stores the core.Image object
	Raw ociCore.Image
	// CompartmentId is the compartment Id where this image is available
	CompartmentId *string
	// InstanceTypes holds a list of shapes compatible with this image
	InstanceTypes []instances.InstanceType
	// IsMinimal is true when the image is a Minimal image. Can only be
	// true for ubuntu OS.
	IsMinimal bool
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

// Alias type representing list of InstanceImage separated by buckets of Base
// and architecture for each Base.
type imageMap map[corebase.Base]map[string][]InstanceImage

// ImageCache holds a cache of all provider images for a fixed
// amount of time before it becomes stale
type ImageCache struct {
	images      imageMap
	lastRefresh time.Time
}

func (i *ImageCache) ImageMap() imageMap {
	return i.images
}

// SetLastRefresh sets the lastRefresh attribute of ImageCache
// This is used mostly for testing purposes
func (i *ImageCache) SetLastRefresh(t time.Time) {
	i.lastRefresh = t
}

func (i *ImageCache) SetImages(images imageMap) {
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
// all images that are currently in cache, matching the provided base
// If defaultVirtType is specified, all generic images will inherit the
// value of defaultVirtType.
func (i ImageCache) ImageMetadata(base corebase.Base, arch string, defaultVirtType string) []*imagemetadata.ImageMetadata {
	var metadata []*imagemetadata.ImageMetadata

	images, ok := i.images[base][arch]
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
			Arch: arch,
			// TODO(gsamfira): add region name?
			// RegionName: region,
			Version:  val.Base.Channel.Track,
			VirtType: string(val.ImageType),
		}
		metadata = append(metadata, imgMeta)
	}

	return metadata
}

// SupportedShapes returns the InstanceTypes available for images matching
// the supplied base
func (i ImageCache) SupportedShapes(base corebase.Base, arch string) []instances.InstanceType {
	matches := map[string]int{}
	ret := []instances.InstanceType{}
	// TODO(gsamfira): Find a better way for this.
	images, ok := i.images[base][arch]
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

// TODO - display names for Images no longer contain vm, bm.
// Find a better way to determine image type. One bit of useful info
// is "-aarch64-" indicates arm64 images today.
//
// DisplayName:   &"Canonical-Ubuntu-22.04-aarch64-2023.03.18-0",
// DisplayName:   &"CentOS-7-2023.01.31-0",
// DisplayName:   &"Canonical-Ubuntu-22.04-2023.03.18-0",
// DisplayName:   &"Canonical-Ubuntu-22.04-Minimal-2023.01.30-0",
// DisplayName:   &"Oracle-Linux-7.9-Gen2-GPU-2022.12.16-0",
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

// NewInstanceImage returns a populated InstanceImage from the ociCore.Image
// struct returned by oci's API, the image's architecture or an error.
func NewInstanceImage(img ociCore.Image, compartmentID *string) (InstanceImage, string, error) {
	if osName := *img.OperatingSystem; osName != ubuntuOS {
		return InstanceImage{}, "", errors.NotSupportedf("os %s", osName)
	}
	base, arch, isMinimal := parseUbuntuImage(img)
	version, err := NewImageVersion(img)
	if err != nil {
		return InstanceImage{}, "", err
	}
	return InstanceImage{
		ImageType:     getImageType(img),
		Id:            *img.Id,
		Base:          base,
		Raw:           img,
		CompartmentId: compartmentID,
		IsMinimal:     isMinimal,
		Version:       version,
	}, arch, nil
}

// parseUbuntuImage returns the base and architecture of the returned image
// from the OCI sdk.
func parseUbuntuImage(img ociCore.Image) (corebase.Base, string, bool) {
	var (
		arch      string = corearch.AMD64
		base      corebase.Base
		isMinimal bool
	)
	// On some cases, the retrieved OperatingSystemVersion can contain
	// the channel plus some extra information and in some others this
	// extra information might be missing.
	// Here are two examples:
	// - The retrieved image with name Canonical-Ubuntu-22.04-Minimal-aarch64-2023.08.27-0
	//   will contain the following values from the API:
	//     OperatingSystem:        Canonical Ubuntu
	//     OperatingSystemVersion: 22.04 Minimal aarch64
	//   In this case, we need to separate the channel (22.04) from the
	//   "postfix" (Minimal aarch64). The channel is needed to correctly
	//   make the base.
	// - The retrieved image with name Canonical-Ubuntu-22.04-aarch64-2023.08.23
	//   will contain the following values from the API:
	//     OperatingSystem:        Canonical Ubuntu
	//     OperatingSystemVersion: 22.04
	//   In this case, the OperatingSystemVersion is not consistent with
	//   the previous example. This is an error on OCI's response (or on
	//   the ubuntu image's metadata) so we need to find a workaround as
	//   explained in the NOTE a few lines below.
	channel, postfix, _ := strings.Cut(*img.OperatingSystemVersion, " ")
	base = corebase.MakeDefaultBase(corebase.UbuntuOS, channel)
	// if not found, means that the OperatingSystemVersion only contained
	// the channel.
	if strings.Contains(*img.DisplayName, "Minimal") ||
		strings.Contains(postfix, "Minimal") {
		isMinimal = true
	}

	if strings.Contains(*img.DisplayName, "aarch64") ||
		strings.Contains(postfix, "aarch64") {
		arch = corearch.ARM64
	}

	return base, arch, isMinimal
}

// instanceTypes will return the list of instanceTypes with information
// retrieved from OCI shapes supported by the imageID and compartmentID.
// TODO(nvinuesa) 2023-09-26
// We should only keep flexible shapes as OCI recommends:
// https://docs.oracle.com/en-us/iaas/Content/Compute/References/computeshapes.htm#flexible#previous-generation-shapes__previous-generation-vm#ariaid-title18
func instanceTypes(cli ComputeClient, compartmentID, imageID *string) ([]instances.InstanceType, error) {
	if cli == nil {
		return nil, errors.Errorf("cannot use nil client")
	}

	// fetch all shapes for the image from the provider
	shapes, err := cli.ListShapes(context.Background(), compartmentID, imageID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// convert shapes to InstanceType
	types := []instances.InstanceType{}
	for _, val := range shapes {
		var mem, cpus float32
		if val.MemoryInGBs != nil {
			mem = *val.MemoryInGBs * 1024
		}
		if val.Ocpus != nil {
			cpus = *val.Ocpus
		}
		archForShape, instanceType := parseArchAndInstType(val)

		// TODO 2023-04-12 (hml)
		// Can we add cost information for each instance type by
		// using the FREE, PAID, and LIMITED_FREE values ranked?
		// BillingType and IsBilledForStoppedInstance.
		newType := instances.InstanceType{
			Name:     *val.Shape,
			Arch:     archForShape,
			Mem:      uint64(mem),
			CpuCores: uint64(cpus),
			VirtType: &instanceType,
		}
		// If the shape is a flexible shape then the MemoryOptions and
		// OcpuOptions will not be nil and they  indicate the maximum
		// and minimum values. We assign the max memory and cpu cores
		// values to the instance type in that case.
		if val.MemoryOptions != nil && val.MemoryOptions.MaxInGBs != nil {
			maxMem := uint64(*val.MemoryOptions.MaxInGBs) * 1024
			newType.MaxMem = &maxMem
		}
		if val.OcpuOptions != nil && val.OcpuOptions.Max != nil {
			maxCpuCores := uint64(*val.OcpuOptions.Max)
			newType.MaxCpuCores = &maxCpuCores
		}
		types = append(types, newType)
	}
	return types, nil
}

func parseArchAndInstType(shape ociCore.Shape) (string, string) {
	var archType, instType string
	if shape.ProcessorDescription != nil {
		archType = archTypeByProcessorDescription(*shape.ProcessorDescription)
	}
	if shape.Shape == nil {
		return archType, instType
	}
	return archType, instTypeByShapeName(*shape.Shape)
}

func archTypeByProcessorDescription(input string) string {
	// ProcessorDescription:          &"2.55 GHz AMD EPYC™ 7J13 (Milan)",
	// ProcessorDescription:          &"2.6 GHz Intel® Xeon® Platinum 8358 (Ice Lake)",
	// ProcessorDescription:          &"3.0 GHz Ampere® Altra™",
	var archType string
	description := strings.ToLower(input)
	if strings.Contains(description, "ampere") {
		archType = corearch.ARM64
	} else if strings.Contains(description, "intel") || strings.Contains(description, "amd") {
		archType = corearch.AMD64
	}
	return archType
}

func instTypeByShapeName(shape string) string {
	// Shape: &"VM.GPU.A10.2",
	// Shape: &"VM.Optimized3.Flex",
	// Shape: &"VM.Standard.A1.Flex",
	// Shape: &"BM.GPU.A10.4",
	// Shape: &"BM.HPC2.36",
	// Shape: &"BM.Optimized3.36",
	// Shape: &"BM.Standard.A1.160",
	switch {
	case strings.HasPrefix(shape, "VM.GPU"), strings.HasPrefix(shape, "BM.GPU"):
		return GPUMachine.String()
	case strings.HasPrefix(shape, "VM."):
		return VirtualMachine.String()
	case strings.HasPrefix(shape, "BM."):
		return BareMetal.String()
	default:
		return ""
	}
}

func refreshImageCache(ctx context.Context, cli ComputeClient, compartmentID *string) (*ImageCache, error) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	if globalImageCache.isStale() == false {
		return globalImageCache, nil
	}

	items, err := cli.ListImages(context.Background(), compartmentID)
	if err != nil {
		return nil, err
	}

	images := map[corebase.Base]map[string][]InstanceImage{}

	for _, val := range items {
		img, arch, err := NewInstanceImage(val, compartmentID)
		if err != nil {
			if val.Id != nil {
				logger.Debugf(ctx, "error parsing image %q: %q", *val.Id, err)
			} else {
				logger.Debugf(ctx, "error parsing image %q", err)
			}
			continue
		}
		// For the moment juju does not support minimal ubuntu
		if img.IsMinimal {
			logger.Tracef(ctx, "ubuntu minimal images (%q), not supported", *val.DisplayName)
			continue
		}
		// Only set the instance types to the images that we correctly
		// parsed.
		instTypes, err := instanceTypes(cli, compartmentID, val.Id)
		if err != nil {
			return nil, err
		}
		// An image only suports one architecture, but since for each
		// image we retrieve the list of shapes from OCI and we map
		// them to InstanceTypes, we just make sure that all of the
		// shapes have the same architecture as the image and we log
		// in case one of them doesn't.
		for _, instType := range instTypes {
			if instType.Arch != arch {
				logger.Debugf(ctx, "instance type %s has arch %s while image %s only supports %s", instType.Name, instType.Arch, *val.Id, arch)
			}
		}
		img.SetInstanceTypes(instTypes)
		// TODO: ListImages can return more than one option for a base
		// based on time created. There is no guarantee that the same
		// shapes are used with all versions of the same images.
		if images[img.Base] == nil {
			images[img.Base] = make(map[string][]InstanceImage)
		}
		images[img.Base][arch] = append(images[img.Base][arch], img)
	}
	// Sort images of every base and arch
	for base := range images {
		for arch := range images[base] {
			sort.Sort(byVersion(images[base][arch]))
		}
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
	ctx context.Context,
	base corebase.Base,
	arch string,
	constraints coreconstraints.Value,
	imgCache *ImageCache,
) (*instances.InstanceSpec, string, error) {
	allImageMetadata := imgCache.ImageMetadata(base, arch, *constraints.VirtType)
	logger.Debugf(ctx, "received %d image(s): %v", len(allImageMetadata), allImageMetadata)

	ic := &instances.InstanceConstraint{
		Base:        base,
		Arch:        arch,
		Constraints: constraints,
	}
	filtered := []*imagemetadata.ImageMetadata{}
	// Filter by series. imgCache.supportedShapes() and
	// imgCache.imageMetadata() will return filtered values
	// by series already. This additional filtering is done
	// in case someone wants to use this function with values
	// not returned by the above two functions
	for _, val := range allImageMetadata {
		if val.Version != ic.Base.Channel.Track {
			continue
		}
		filtered = append(filtered, val)
	}

	instanceType := imgCache.SupportedShapes(base, arch)
	images := instances.ImageMetadataToImages(filtered)
	spec, err := instances.FindInstanceSpec(images, ic, instanceType)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	return spec, spec.Image.Id, nil
}
