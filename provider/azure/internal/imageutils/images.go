// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imageutils

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/os"
	jujuseries "github.com/juju/utils/series"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
)

var logger = loggo.GetLogger("juju.provider.azure")

const (
	centOSPublisher = "OpenLogic"
	centOSOffering  = "CentOS"

	ubuntuPublisher = "Canonical"
	ubuntuOffering  = "UbuntuServer"

	windowsServerPublisher = "MicrosoftWindowsServer"
	windowsServerOffering  = "WindowsServer"

	windowsPublisher = "MicrosoftVisualStudio"
	windowsOffering  = "Windows"

	dailyStream = "daily"
)

// SeriesImage gets an instances.Image for the specified series, image stream
// and location. The resulting Image's ID is in the URN format expected by
// Azure Resource Manager.
//
// For Ubuntu, we query the SKUs to determine the most recent point release
// for a series.
func SeriesImage(
	series, stream, location string,
	client compute.VirtualMachineImagesClient,
) (*instances.Image, error) {
	seriesOS, err := jujuseries.GetOSFromSeries(series)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var publisher, offering, sku string
	switch seriesOS {
	case os.Ubuntu:
		publisher = ubuntuPublisher
		offering = ubuntuOffering
		sku, err = ubuntuSKU(series, stream, location, client)
		if err != nil {
			return nil, errors.Annotatef(err, "selecting SKU for %s", series)
		}

	case os.Windows:
		switch series {
		case "win81":
			publisher = windowsPublisher
			offering = windowsOffering
			sku = "8.1-Enterprise-N"
		case "win10":
			publisher = windowsPublisher
			offering = windowsOffering
			sku = "10-Enterprise"
		case "win2012":
			publisher = windowsServerPublisher
			offering = windowsServerOffering
			sku = "2012-Datacenter"
		case "win2012r2":
			publisher = windowsServerPublisher
			offering = windowsServerOffering
			sku = "2012-R2-Datacenter"
		default:
			return nil, errors.NotSupportedf("deploying %s", series)
		}

	case os.CentOS:
		publisher = centOSPublisher
		offering = centOSOffering
		switch series {
		case "centos7":
			sku = "7.1"
		default:
			return nil, errors.NotSupportedf("deploying %s", series)
		}

	default:
		// TODO(axw) CentOS
		return nil, errors.NotSupportedf("deploying %s", seriesOS)
	}

	return &instances.Image{
		Id:       fmt.Sprintf("%s:%s:%s:latest", publisher, offering, sku),
		Arch:     arch.AMD64,
		VirtType: "Hyper-V",
	}, nil
}

// ubuntuSKU returns the best SKU for the Canonical:UbuntuServer offering,
// matching the given series.
func ubuntuSKU(series, stream, location string, client compute.VirtualMachineImagesClient) (string, error) {
	seriesVersion, err := jujuseries.SeriesVersion(series)
	if err != nil {
		return "", errors.Trace(err)
	}
	logger.Debugf("listing SKUs: Location=%s, Publisher=%s, Offer=%s", location, ubuntuPublisher, ubuntuOffering)
	result, err := client.ListSkus(location, ubuntuPublisher, ubuntuOffering)
	if err != nil {
		return "", errors.Annotate(err, "listing Ubuntu SKUs")
	}
	if result.Value == nil || len(*result.Value) == 0 {
		return "", errors.NotFoundf("Ubuntu SKUs")
	}
	skuNamesByVersion := make(map[ubuntuVersion]string)
	var versions ubuntuVersions
	for _, result := range *result.Value {
		skuName := to.String(result.Name)
		if !strings.HasPrefix(skuName, seriesVersion) {
			logger.Debugf("ignoring SKU %q (does not match series %q)", skuName, series)
			continue
		}
		version, tag, err := parseUbuntuSKU(skuName)
		if err != nil {
			logger.Errorf("ignoring SKU %q (failed to parse: %s)", skuName, err)
			continue
		}
		var skuStream string
		switch tag {
		case "", "LTS":
			skuStream = imagemetadata.ReleasedStream
		case "DAILY", "DAILY-LTS":
			skuStream = dailyStream
		}
		if skuStream == "" || skuStream != stream {
			logger.Debugf("ignoring SKU %q (not in %q stream)", skuName, stream)
			continue
		}
		skuNamesByVersion[version] = skuName
		versions = append(versions, version)
	}
	if len(versions) == 0 {
		return "", errors.NotFoundf("Ubuntu SKUs for %s stream", stream)
	}
	sort.Sort(versions)
	bestVersion := versions[len(versions)-1]
	return skuNamesByVersion[bestVersion], nil
}

type ubuntuVersion struct {
	Year  int
	Month int
	Point int
}

// parseUbuntuSKU splits an UbuntuServer SKU into its
// version ("14.04.3") and tag ("LTS") parts.
func parseUbuntuSKU(sku string) (ubuntuVersion, string, error) {
	var version ubuntuVersion
	var tag string
	var err error
	parts := strings.Split(sku, "-")
	if len(parts) > 1 {
		tag = parts[1]
	}
	parts = strings.SplitN(parts[0], ".", 3)
	version.Year, err = strconv.Atoi(parts[0])
	if err != nil {
		return ubuntuVersion{}, "", errors.Trace(err)
	}
	version.Month, err = strconv.Atoi(parts[1])
	if err != nil {
		return ubuntuVersion{}, "", errors.Trace(err)
	}
	if len(parts) > 2 {
		version.Point, err = strconv.Atoi(parts[2])
		if err != nil {
			return ubuntuVersion{}, "", errors.Trace(err)
		}
	}
	return version, tag, nil
}

type ubuntuVersions []ubuntuVersion

func (v ubuntuVersions) Len() int {
	return len(v)
}

func (v ubuntuVersions) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

func (v ubuntuVersions) Less(i, j int) bool {
	vi, vj := v[i], v[j]
	if vi.Year < vj.Year {
		return true
	} else if vi.Year > vj.Year {
		return false
	}
	if vi.Month < vj.Month {
		return true
	} else if vi.Month > vj.Month {
		return false
	}
	return vi.Point < vj.Point
}
