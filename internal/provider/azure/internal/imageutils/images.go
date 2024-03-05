// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imageutils

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v2"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/core/arch"
	jujubase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/os"
	jujuos "github.com/juju/juju/core/os"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/provider/azure/internal/errorutils"
)

var logger = loggo.GetLogger("juju.provider.azure")

const (
	centOSPublisher = "OpenLogic"
	centOSOffering  = "CentOS"

	ubuntuPublisher = "Canonical"

	dailyStream = "daily"
)

// SeriesImage gets an instances.Image for the specified series, image stream
// and location. The resulting Image's ID is in the URN format expected by
// Azure Resource Manager.
//
// For Ubuntu, we query the SKUs to determine the most recent point release
// for a series.
func SeriesImage(
	ctx envcontext.ProviderCallContext,
	base jujubase.Base, stream, location string,
	client *armcompute.VirtualMachineImagesClient,
) (*instances.Image, error) {
	seriesOS := jujuos.OSTypeForName(base.OS)

	var publisher, offering, sku string
	switch seriesOS {
	case os.Ubuntu:
		series, err := jujubase.GetSeriesFromBase(base)
		if err != nil {
			return nil, errors.Trace(err)
		}
		publisher = ubuntuPublisher
		sku, offering, err = ubuntuSKU(ctx, series, stream, location, client)
		if err != nil {
			return nil, errors.Annotatef(err, "selecting SKU for %s", base.DisplayString())
		}

	case os.CentOS:
		publisher = centOSPublisher
		offering = centOSOffering
		switch base.Channel.Track {
		case "7", "8": // TODO: this doesn't look right. Add support for centos 9 stream.
			sku = "7.3"
		default:
			return nil, errors.NotSupportedf("deploying %s", base)
		}

	default:
		return nil, errors.NotSupportedf("deploying %s", seriesOS)
	}

	return &instances.Image{
		Id:       fmt.Sprintf("%s:%s:%s:latest", publisher, offering, sku),
		Arch:     arch.AMD64,
		VirtType: "Hyper-V",
	}, nil
}

func offerForUbuntuSeries(series string) (string, string, error) {
	seriesVersion, err := jujubase.SeriesVersion(series)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	seriesVersion = strings.ReplaceAll(seriesVersion, ".", "_")
	return fmt.Sprintf("0001-com-ubuntu-server-%s", series), seriesVersion, nil
}

// ubuntuSKU returns the best SKU for the Canonical:UbuntuServer offering,
// matching the given series.
func ubuntuSKU(ctx envcontext.ProviderCallContext, series, stream, location string, client *armcompute.VirtualMachineImagesClient) (string, string, error) {
	offer, seriesVersion, err := offerForUbuntuSeries(series)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	logger.Debugf("listing SKUs: Location=%s, Publisher=%s, Offer=%s", location, ubuntuPublisher, offer)
	result, err := client.ListSKUs(ctx, location, ubuntuPublisher, offer, nil)
	if err != nil {
		return "", "", errorutils.HandleCredentialError(errors.Annotate(err, "listing Ubuntu SKUs"), ctx)
	}
	skuNamesByVersion := make(map[ubuntuVersion]string)
	var versions ubuntuVersions
	for _, img := range result.VirtualMachineImageResourceArray {
		skuName := *img.Name
		logger.Debugf("Found Azure SKU Name: %v", skuName)
		if !strings.HasPrefix(skuName, seriesVersion) {
			logger.Debugf("ignoring SKU %q (does not match series %q with version %q)", skuName, series, seriesVersion)
			continue
		}
		version, tag, err := parseUbuntuSKU(skuName)
		if err != nil {
			logger.Errorf("ignoring SKU %q (failed to parse: %s)", skuName, err)
			continue
		}
		logger.Debugf("SKU has version %#v and tag %q", version, tag)
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
		return "", "", errors.NotFoundf("Ubuntu SKUs for %s stream", stream)
	}
	sort.Sort(versions)
	bestVersion := versions[len(versions)-1]
	return skuNamesByVersion[bestVersion], offer, nil
}

type ubuntuVersion struct {
	Year  int
	Month int
	Point int
}

// parseUbuntuSKU splits an UbuntuServer SKU into its
// version ("22_04.3") and tag ("LTS") parts.
func parseUbuntuSKU(sku string) (ubuntuVersion, string, error) {
	var version ubuntuVersion
	var tag string
	var err error
	parts := strings.SplitN(sku, "-", 2)
	if len(parts) > 1 {
		tag = strings.ToUpper(parts[1])
	}
	sep := "_"
	if strings.Contains(parts[0], ".") {
		sep = "."
	}
	parts = strings.SplitN(parts[0], sep, 3)
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
