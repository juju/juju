// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imageutils

import (
	stdcontext "context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2018-10-01/compute"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/arch"

	"github.com/juju/juju/core/os"
	jujuseries "github.com/juju/juju/core/series"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/azure/internal/errorutils"
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
	ctx context.ProviderCallContext,
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
		sku, offering, err = ubuntuSKU(ctx, series, stream, location, client)
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
		case "centos7", "centos8":
			sku = "7.3"
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

func offerForUbuntuSeries(series string) (string, string, error) {
	seriesVersion, err := jujuseries.SeriesVersion(series)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	oldSeries := set.NewStrings("trusty", "xenial", "bionic", "cosmic", "disco")
	if oldSeries.Contains(series) {
		return ubuntuOffering, seriesVersion, nil
	}
	seriesVersion = strings.ReplaceAll(seriesVersion, ".", "_")
	return fmt.Sprintf("0001-com-ubuntu-server-%s", series), seriesVersion, nil
}

// ubuntuSKU returns the best SKU for the Canonical:UbuntuServer offering,
// matching the given series.
func ubuntuSKU(ctx context.ProviderCallContext, series, stream, location string, client compute.VirtualMachineImagesClient) (string, string, error) {
	offer, seriesVersion, err := offerForUbuntuSeries(series)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	logger.Debugf("listing SKUs: Location=%s, Publisher=%s, Offer=%s", location, ubuntuPublisher, offer)
	sdkCtx := stdcontext.Background()
	result, err := client.ListSkus(sdkCtx, location, ubuntuPublisher, offer)
	if err != nil {
		return "", "", errorutils.HandleCredentialError(errors.Annotate(err, "listing Ubuntu SKUs"), ctx)
	}
	if result.Value == nil || len(*result.Value) == 0 {
		return "", "", errors.NotFoundf("Ubuntu SKUs")
	}
	skuNamesByVersion := make(map[ubuntuVersion]string)
	var versions ubuntuVersions
	for _, result := range *result.Value {
		skuName := to.String(result.Name)
		logger.Debugf("Found Azure SKU Name: %v", skuName)
		if !strings.HasPrefix(skuName, seriesVersion) {
			logger.Debugf("ignoring SKU %q (does not match series %q)", skuName, series)
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
// version ("14.04.3") and tag ("LTS") parts.
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
