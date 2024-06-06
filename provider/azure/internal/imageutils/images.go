// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imageutils

import (
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v2"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/core/arch"
	jujubase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/os/ostype"
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

	dailyStream = "daily"

	plan = "server-gen1"
)

// BaseImage gets an instances.Image for the specified base, image stream
// and location. The resulting Image's ID is in the URN format expected by
// Azure Resource Manager.
//
// For Ubuntu, we query the SKUs to determine the most recent point release
// for a series.
func BaseImage(
	ctx context.ProviderCallContext,
	base jujubase.Base, stream, location string,
	client *armcompute.VirtualMachineImagesClient,
) (*instances.Image, error) {
	seriesOS := ostype.OSTypeForName(base.OS)

	var publisher, offering, sku string
	switch seriesOS {
	case ostype.Ubuntu:
		publisher = ubuntuPublisher
		var err error
		sku, offering, err = ubuntuSKU(ctx, base, stream, location, client)
		if err != nil {
			return nil, errors.Annotatef(err, "selecting SKU for %s", base.DisplayString())
		}

	case ostype.CentOS:
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

// legacyUbuntuBases is a slice of bases which use the old-style offer
// id formatted like "0001-com-ubuntu-server-${series}".
//
// Recently Canonical changed the format for images offer ids
// and SKUs in Azure. The threshold for this change was noble, so if
// we want to deploy bases before noble, we must branch and use the
// old format.
//
// The old format offer ids have format `0001-com-ubuntu-server-${series}`
// or `001-com-ubuntu-server-${series}-daily` and have SKUs formatted
// `${version_number}-lts`, `${version_number}-gen2`, etc.
//
// The new format offer ids have format `ubuntu-${version_number}`,
// `ubuntu-${version_number}-lts`, `ubuntu-${version_number}-lts-daily`,
// etc. and have SKUs `server`, `server-gen1`m, `server-arm64`, etc.
//
// Since there are only a finte number of Ubuntu versions we support
// before Noble, we hardcode this list. So when new versions of Ubuntu
// are
//
// All Ubuntu images we support outside of this list have offer
// id like "ubuntu-${version}-lts" or "ubuntu-${version}"
var legacyUbuntuBases = []jujubase.Base{
	jujubase.MustParseBaseFromString("ubuntu@20.04"),
	jujubase.MustParseBaseFromString("ubuntu@20.10"),
	jujubase.MustParseBaseFromString("ubuntu@21.04"),
	jujubase.MustParseBaseFromString("ubuntu@21.10"),
	jujubase.MustParseBaseFromString("ubuntu@22.04"),
	jujubase.MustParseBaseFromString("ubuntu@22.10"),
	jujubase.MustParseBaseFromString("ubuntu@23.04"),
	jujubase.MustParseBaseFromString("ubuntu@23.10"),
}

func ubuntuBaseIslegacy(base jujubase.Base) bool {
	for _, oldBase := range legacyUbuntuBases {
		if base.IsCompatible(oldBase) {
			return true
		}
	}
	return false
}

// ubuntuSKU returns the best SKU for the Canonical:UbuntuServer offering,
// matching the given series.
func ubuntuSKU(ctx context.ProviderCallContext, base jujubase.Base, stream, location string, client *armcompute.VirtualMachineImagesClient) (string, string, error) {
	if ubuntuBaseIslegacy(base) {
		return legacyUbuntuSKU(ctx, base, stream, location, client)
	}

	offer := fmt.Sprintf("ubuntu-%s", strings.ReplaceAll(base.Channel.Track, ".", "_"))
	if base.IsUbuntuLTS() {
		offer = fmt.Sprintf("%s-lts", offer)
	}
	if stream == dailyStream {
		offer = fmt.Sprintf("%s-daily", offer)
	}

	logger.Debugf("listing SKUs: Location=%s, Publisher=%s, Offer=%s", location, ubuntuPublisher, offer)
	result, err := client.ListSKUs(ctx, location, ubuntuPublisher, offer, nil)
	if err != nil {
		return "", "", errorutils.HandleCredentialError(errors.Annotate(err, "listing Ubuntu SKUs"), ctx)
	}
	for _, img := range result.VirtualMachineImageResourceArray {
		skuName := *img.Name
		if skuName == plan {
			logger.Debugf("found Azure SKU Name: %v", skuName)
			return skuName, offer, nil
		}
		logger.Debugf("ignoring Azure SKU Name: %v", skuName)
	}
	return "", "", errors.NotFoundf("ubuntu %q SKUs for %v stream", base, stream)
}

func legacyUbuntuSKU(ctx context.ProviderCallContext, base jujubase.Base, stream, location string, client *armcompute.VirtualMachineImagesClient) (string, string, error) {
	series, err := jujubase.GetSeriesFromBase(base)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	offer := fmt.Sprintf("0001-com-ubuntu-server-%s", series)
	if stream == dailyStream {
		offer = fmt.Sprintf("%s-daily", offer)
	}
	desiredSKUPrefix := strings.ReplaceAll(base.Channel.Track, ".", "_")

	logger.Debugf("listing SKUs: Location=%s, Publisher=%s, Offer=%s", location, ubuntuPublisher, offer)
	result, err := client.ListSKUs(ctx, location, ubuntuPublisher, offer, nil)
	if err != nil {
		return "", "", errorutils.HandleCredentialError(errors.Annotate(err, "listing Ubuntu SKUs"), ctx)
	}
	for _, img := range result.VirtualMachineImageResourceArray {
		skuName := *img.Name
		logger.Debugf("found Azure SKU Name: %v", skuName)
		if !strings.HasPrefix(skuName, desiredSKUPrefix) {
			logger.Debugf("ignoring SKU %q (does not match series %q)", skuName, series)
			continue
		}
		tag := getLegacyUbuntuSKUTag(skuName)
		logger.Debugf("SKU has tag %q", tag)
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
		return skuName, offer, nil

	}
	return "", "", errors.NotFoundf("legacy ubuntu %q SKUs for %s stream", series, stream)
}

// getLegacyUbuntuSKUTag splits an UbuntuServer SKU and extracts
// the tag ("LTS") part.
func getLegacyUbuntuSKUTag(sku string) string {
	var tag string
	parts := strings.SplitN(sku, "-", 2)
	if len(parts) > 1 {
		tag = strings.ToUpper(parts[1])
	}
	return tag
}
