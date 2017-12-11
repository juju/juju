// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	// jujuos "github.com/juju/utils/os"
	"github.com/juju/utils/series"

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
)

// shapeSpecs is a map containing resource information
// about each shape. Unfortunately the API simply returns
// the name of the shape and nothing else. For details see:
// https://cloud.oracle.com/infrastructure/pricing
// https://cloud.oracle.com/infrastructure/compute/pricing
var shapeSpecs = map[string]ShapeSpec{
	"VM.Standard1.1": ShapeSpec{
		Cpus:   1,
		Memory: 7168,
		Type:   VirtualMachine,
	},
	"VM.Standard2.1": ShapeSpec{
		Cpus:   1,
		Memory: 15360,
		Type:   VirtualMachine,
	},
	"VM.Standard1.2": ShapeSpec{
		Cpus:   2,
		Memory: 14336,
		Type:   VirtualMachine,
	},
	"VM.Standard2.2": ShapeSpec{
		Cpus:   2,
		Memory: 30720,
		Type:   VirtualMachine,
	},
	"VM.Standard1.4": ShapeSpec{
		Cpus:   4,
		Memory: 28672,
		Type:   VirtualMachine,
	},
	"VM.Standard2.4": ShapeSpec{
		Cpus:   4,
		Memory: 61440,
		Type:   VirtualMachine,
	},
	"VM.Standard1.8": ShapeSpec{
		Cpus:   8,
		Memory: 57344,
		Type:   VirtualMachine,
	},
	"VM.Standard2.8": ShapeSpec{
		Cpus:   8,
		Memory: 122880,
		Type:   VirtualMachine,
	},
	"VM.Standard1.16": ShapeSpec{
		Cpus:   16,
		Memory: 114688,
		Type:   VirtualMachine,
	},
	"VM.Standard2.16": ShapeSpec{
		Cpus:   16,
		Memory: 245760,
		Type:   VirtualMachine,
	},
	"VM.Standard2.24": ShapeSpec{
		Cpus:   24,
		Memory: 327680,
		Type:   VirtualMachine,
	},
	"VM.DenseIO1.4": ShapeSpec{
		Cpus:   4,
		Memory: 61440,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
	},
	"VM.DenseIO1.8": ShapeSpec{
		Cpus:   8,
		Memory: 122880,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
	},
	"VM.DenseIO2.8": ShapeSpec{
		Cpus:   8,
		Memory: 122880,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
	},
	"VM.DenseIO1.16": ShapeSpec{
		Cpus:   16,
		Memory: 245760,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
	},

	"VM.DenseIO2.16": ShapeSpec{
		Cpus:   16,
		Memory: 245760,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
	},
	"VM.DenseIO2.24": ShapeSpec{
		Cpus:   24,
		Memory: 327680,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
	},
	"BM.Standard1.36": ShapeSpec{
		Cpus:   36,
		Memory: 262144,
		Type:   BareMetal,
	},
	"BM.Standard2.52": ShapeSpec{
		Cpus:   52,
		Memory: 786432,
		Type:   BareMetal,
	},
	"BM.HighIO1.36": ShapeSpec{
		Cpus:   36,
		Memory: 524288,
		Type:   BareMetal,
		Tags: []string{
			"highio",
		},
	},
	"BM.DenseIO1.36": ShapeSpec{
		Cpus:   1,
		Memory: 7168,
		Type:   BareMetal,
		Tags: []string{
			"denseio",
		},
	},
	"BM.DenseIO2.52": ShapeSpec{
		Cpus:   52,
		Memory: 786432,
		Type:   BareMetal,
		Tags: []string{
			"denseio",
		},
	},
	"BM.GPU2.2": ShapeSpec{
		Cpus:   28,
		Gpus:   2,
		Memory: 196608,
		Type:   GPUMachine,
		Tags: []string{
			"denseio",
		},
	},
}

var globalImageCache = &imageCache{}
var cacheMutex = &sync.Mutex{}

// ShapeSpec holds information about a shapes resource allocation
type ShapeSpec struct {
	// Cpus is the number of CPU cores available to the instance
	Cpus int
	// Gpus is the number of GPUs available to this instance
	Gpus int
	// Memory is the amount of RAM available to the instance
	Memory int
	Type   InstanceType
	Tags   []string
}

type InstanceType string
type ImageType string

type ImageVersion struct {
	TimeStamp time.Time
	Revision  int
}

func NewImageVersion(img ociCore.Image) (ImageVersion, error) {
	var imgVersion ImageVersion
	if img.DisplayName == nil {
		return imgVersion, errors.Errorf("image does not have a display bane")
	}
	fields := strings.Split(*img.DisplayName, "-")
	if len(fields) < 2 {
		return imgVersion, errors.Errorf("invalid image display name")
	}
	timeStamp, err := time.Parse("2006.01.02", fields[len(fields)-2])
	if err != nil {
		return imgVersion, err
	}

	revision, err := strconv.Atoi(fields[len(fields)-1])

	if err != nil {
		return imgVersion, err
	}

	imgVersion.TimeStamp = timeStamp
	imgVersion.Revision = revision
	return imgVersion, nil
}

type InstanceImage struct {
	// ImageType determins which type of image this is. Valid values are:
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
	if t[i].Version.TimeStamp.Before(t[j].Version.TimeStamp) {
		return true
	}
	if t[i].Version.TimeStamp.Equal(t[j].Version.TimeStamp) {
		if t[i].Version.Revision < t[j].Version.Revision {
			return true
		}
	}
	return false
}

type imageCache struct {
	images []InstanceImage

	// shapeToInstanceImageMap map[string][]InstanceImage

	lastRefresh time.Time
}

func (i *imageCache) isStale() bool {
	threshold := i.lastRefresh.Add(30 * time.Minute)
	now := time.Now()
	if now.After(threshold) {
		return true
	}
	return false
}

func (i imageCache) imageMetadata(series string, defaultVirtType string) []*imagemetadata.ImageMetadata {
	var metadata []*imagemetadata.ImageMetadata

	for _, val := range i.images {
		if val.Series == series {
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
	}

	return metadata
}

func (i imageCache) supportedShapes(series string) []instances.InstanceType {
	matches := map[string]int{}
	ret := []instances.InstanceType{}
	// TODO(gsamfira): Find a better way for this.
	for _, img := range i.images {
		if img.Series == series {
			for _, instType := range img.InstanceTypes {
				if _, ok := matches[instType.Name]; !ok {
					matches[instType.Name] = 1
					ret = append(ret, instType)
				}
			}
		}
	}
	return ret
}

func newImageCache(images []InstanceImage) *imageCache {
	now := time.Now()
	return &imageCache{
		images:      images,
		lastRefresh: now,
	}
}

func getImageType(img ociCore.Image) ImageType {
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
		return imgType, errors.NotSupportedf("os %s is not supported", osVersion)
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

func instanceTypes(cli common.ApiClient, compartmentID, imageID *string) ([]instances.InstanceType, error) {
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
	types := make([]instances.InstanceType, len(shapes.Items), len(shapes.Items))
	for key, val := range shapes.Items {
		spec, ok := shapeSpecs[*val.Shape]
		if !ok {
			logger.Debugf("shape %s does not have a mapping", *val.Shape)
			continue
		}
		instanceType := string(spec.Type)
		types[key].Name = *val.Shape
		types[key].Arches = arch
		types[key].Mem = uint64(spec.Memory)
		types[key].CpuCores = uint64(spec.Cpus)
		// root disk size is not configurable in OCI at the time of this writing
		// You get a 50 GB disk.
		types[key].RootDisk = RootDiskSize
		// its not really virtualization type. We have just 3 types of images:
		// bare metal, virtual and generic (works on metal and VM).
		types[key].VirtType = &instanceType
	}

	return types, nil
}

func refreshImageCache(cli common.ApiClient, compartmentID *string) (*imageCache, error) {
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
		return nil, err
	}

	images := []InstanceImage{}

	for _, val := range response.Items {
		instTypes, err := instanceTypes(cli, compartmentID, val.Id)
		if err != nil {
			return nil, err
		}
		img, err := NewInstanceImage(val, compartmentID)
		if err != nil {
			if !series.IsUnknownSeriesVersionError(err) && !errors.IsNotSupported(err) && !series.IsUnknownVersionSeriesError(err) {
				return nil, err
			}
			continue
		}
		img.SetInstanceTypes(instTypes)
		images = append(images, img)
	}
	globalImageCache = &imageCache{
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
