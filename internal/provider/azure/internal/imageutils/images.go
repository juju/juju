// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imageutils

import (
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v2"
	"github.com/juju/errors"

	corearch "github.com/juju/juju/core/arch"
	jujubase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/provider/azure/internal/errorutils"
)

var logger = internallogger.GetLogger("juju.provider.azure")

const (
	ubuntuPublisher = "Canonical"

	dailyStream = "daily"

	// SKUs we prefer for 24.04 LTS and newer bases.
	planV2    = "server"
	planARM64 = "server-arm64"
	planV1    = "server-gen1"

	legacyPlanGen2Suffix  = "-gen2"
	legacyPlanARM64Suffix = "-arm64"

	defaultArchitecture = corearch.AMD64
)

// BaseImage gets an instances.Image for the specified base, image stream
// and location. The resulting Image's ID is in the URN format expected by
// Azure Resource Manager.
//
// For Ubuntu, we query the SKUs to determine the most recent point release
// for a series.
func BaseImage(
	ctx envcontext.ProviderCallContext,
	base jujubase.Base, stream, location, arch string,
	client *armcompute.VirtualMachineImagesClient,
	preferGen1Image bool,
) (*instances.Image, error) {
	if arch == "" {
		arch = defaultArchitecture
	}

	seriesOS := ostype.OSTypeForName(base.OS)

	var publisher, offering, sku string
	switch seriesOS {
	case ostype.Ubuntu:
		publisher = ubuntuPublisher
		var err error
		sku, offering, err = ubuntuSKU(ctx, base, stream, location, arch, client, preferGen1Image)
		if err != nil {
			return nil, errors.Annotatef(err, "selecting SKU for %s", base.DisplayString())
		}

	default:
		return nil, errors.NotSupportedf("deploying %s", seriesOS)
	}

	return &instances.Image{
		Id:       fmt.Sprintf("%s:%s:%s:latest", publisher, offering, sku),
		Arch:     arch,
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
// etc. and have SKUs `server`, `server-gen1`, `server-arm64`, etc.
//
// Since there are only a finite number of Ubuntu versions we support
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
func ubuntuSKU(
	ctx envcontext.ProviderCallContext, base jujubase.Base, stream, location, arch string,
	client *armcompute.VirtualMachineImagesClient, preferGen1Image bool,
) (string, string, error) {
	if ubuntuBaseIslegacy(base) {
		return legacyUbuntuSKU(ctx, base, stream, location, arch, client, preferGen1Image)
	}

	if arch != corearch.AMD64 && preferGen1Image {
		return "", "", errors.NotSupportedf("deploying %q with Gen1 image", arch)
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
	// We prefer to use v2 SKU if available.
	// If we don't find any v2 SKU, we return the v1 SKU.
	// If preferGen1Image is true, we return the v1 SKU.
	var v1SKU string
	for _, img := range result.VirtualMachineImageResourceArray {
		skuName := *img.Name

		if skuName == planARM64 {
			if arch == corearch.ARM64 {
				logger.Debugf("found Azure SKU Name: %q for arch %q", skuName, arch)
				return skuName, offer, nil
			}
			continue
		}
		if arch == corearch.ARM64 {
			continue
		}

		if skuName == planV2 && !preferGen1Image {
			logger.Debugf("found Azure SKU Name: %v", skuName)
			return skuName, offer, nil
		}
		if skuName == planV1 {
			v1SKU = skuName
			continue
		}
		logger.Debugf("ignoring Azure SKU Name: %v", skuName)
	}
	if v1SKU != "" {
		return v1SKU, offer, nil
	}
	return "", "", errors.NotFoundf("ubuntu %q SKUs for %v stream", base, stream)
}

func legacyUbuntuSKU(
	ctx envcontext.ProviderCallContext, base jujubase.Base, stream, location, arch string,
	client *armcompute.VirtualMachineImagesClient, preferGen1Image bool,
) (string, string, error) {
	series, err := jujubase.GetSeriesFromBase(base)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	offer := fmt.Sprintf("0001-com-ubuntu-server-%s", series)
	if stream == dailyStream {
		offer = fmt.Sprintf("%s-daily", offer)
	}

	logger.Debugf(
		"listing SKUs: Base=%s, Series=%s, Location=%s, Arch=%s, Stream=%s, Publisher=%s, Offer=%s",
		base.Channel.Track, series, location, arch, stream, ubuntuPublisher, offer,
	)
	result, err := client.ListSKUs(ctx, location, ubuntuPublisher, offer, nil)
	if err != nil {
		return "", "", errorutils.HandleCredentialError(errors.Annotate(err, "listing Ubuntu SKUs"), ctx)
	}

	skuName, err := selectUbuntuSKULegacy(base, series, stream, arch, result.VirtualMachineImageResourceArray, preferGen1Image)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	return skuName, offer, nil
}

func selectUbuntuSKULegacy(
	base jujubase.Base, series, stream, arch string,
	images []*armcompute.VirtualMachineImageResource, preferGen1Image bool,
) (string, error) {
	// We prefer to use v2 SKU if available.
	// If we don't find any v2 SKU, we return the v1 SKU.
	// If preferGen1Image is true, we return the v1 SKU.
	var v1SKU string
	desiredSKUVersionPrefix := strings.ReplaceAll(base.Channel.Track, ".", "_")

	validStream := func(skuName string) bool {
		if skuName == "" {
			return false
		}

		tag := getLegacyUbuntuSKUTag(skuName)
		logger.Debugf("SKU %q has tag %q", skuName, tag)
		var skuStream string
		switch tag {
		case "", "LTS":
			skuStream = imagemetadata.ReleasedStream
		case "DAILY", "DAILY-LTS":
			skuStream = dailyStream
		}
		if skuStream == "" || skuStream != stream {
			logger.Debugf("ignoring SKU %q (not in %q stream)", skuName, stream)
			return false
		}
		return true
	}

	for _, img := range images {
		skuName := *img.Name
		logger.Debugf("Azure SKU Name: %v", skuName)
		if !strings.HasPrefix(skuName, desiredSKUVersionPrefix) {
			logger.Debugf("ignoring SKU %q (does not match series %q)", skuName, series)
			continue
		}

		if strings.HasSuffix(skuName, legacyPlanARM64Suffix) {
			if arch == corearch.ARM64 && validStream(skuName) {
				return skuName, nil
			}
			continue
		}
		if arch == corearch.ARM64 {
			continue
		}

		if strings.HasSuffix(skuName, legacyPlanGen2Suffix) {
			if preferGen1Image || !validStream(skuName) {
				continue
			}
			return skuName, nil
		}
		v1SKU = skuName
	}
	if validStream(v1SKU) {
		return v1SKU, nil
	}
	return "", errors.NotFoundf("legacy ubuntu %q SKUs for %s stream", series, stream)
}

// getLegacyUbuntuSKUTag splits an UbuntuServer SKU and extracts the tag ("LTS") part.
// The SKU is expected to be in the format "${version_number}-${tag}[-${gen}]" or "${version_number}-${tag}-arm64".
// For example, "22_04-lts", "22_04-lts-gen2", "22_04-lts-arm64".
func getLegacyUbuntuSKUTag(sku string) string {
	var tag string
	parts := strings.SplitN(sku, "-", -1)
	if len(parts) > 1 {
		tag = strings.ToUpper(parts[1])
	}
	return tag
}
