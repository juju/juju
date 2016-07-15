// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config

import (
	"fmt"

	"github.com/juju/utils/packaging"
)

const (
	// UbuntuCloudArchiveUrl is the url of the cloud archive on Ubuntu.
	UbuntuCloudArchiveUrl = "http://ubuntu-cloud.archive.canonical.com/ubuntu"

	// CloudToolsPrefsPath defines the default location of
	// apt_preferences(5) file for the cloud-tools pocket.
	UbuntuCloudToolsPrefsPath = "/etc/apt/preferences.d/50-cloud-tools"

	// UbuntuCloudArchiveSigningKey is the PGP publivc key for the canonical
	// cloud archive on Ubuntu.
	UbuntuCloudArchiveSigningKey = `-----BEGIN PGP PUBLIC KEY BLOCK-----
Version: SKS 1.1.4
Comment: Hostname: keyserver.ubuntu.com

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
p/+af/HU1smBrOfIeRoxb8jQoHu3
=xg4S
-----END PGP PUBLIC KEY BLOCK-----`
)

// UbuntuDefaultPackages is the default package set we'd like to installed on
// all Ubuntu machines.
var UbuntuDefaultPackages = append(DefaultPackages, []string{
	// TODO (aznashwan, all): populate this list.
	"python-software-properties",
}...)

// UbuntuDefaultRepositories is the default repository set we'd like to enable
// on all Ubuntu machines.
var UbuntuDefaultRepositories = []string{
//TODO (aznashwan, all): populate this list.
}

// cloudArchivePackagesUbuntu maintains a list of Ubuntu packages that
// Configurer.IsCloudArchivePackage will reference when determining the
// --target-release for a given series.
// http://reqorts.qa.ubuntu.com/reports/ubuntu-server/cloud-archive/cloud-tools_versions.html
var cloudArchivePackagesUbuntu = map[string]struct{}{
	"cloud-image-utils":       struct{}{},
	"cloud-utils":             struct{}{},
	"curtin":                  struct{}{},
	"djorm-ext-pgarray":       struct{}{},
	"golang":                  struct{}{},
	"iproute2":                struct{}{},
	"isc-dhcp":                struct{}{},
	"juju-core":               struct{}{},
	"libseccomp":              struct{}{},
	"libv8-3.14":              struct{}{},
	"lxc":                     struct{}{},
	"maas":                    struct{}{},
	"mongodb":                 struct{}{},
	"mongodb-server":          struct{}{},
	"python-django":           struct{}{},
	"python-django-piston":    struct{}{},
	"python-jujuclient":       struct{}{},
	"python-tx-tftp":          struct{}{},
	"python-websocket-client": struct{}{},
	"raphael 2.1.0-1ubuntu1":  struct{}{},
	"simplestreams":           struct{}{},
	"txlongpoll":              struct{}{},
	"uvtool":                  struct{}{},
	"yui3":                    struct{}{},
}

// ubuntuToCentOSPackageNameMap is a map for converting package names from their
// names in Ubuntu repositories to their equivalent CentOS names.
var ubuntuToCentOSPackageNameMap = map[string]string{
// TODO(aznashwan, everyone): thouroughly research differing package
// names and add them to this map.
// NOTE: the following are the packages which currently count as cloud
// archive packages and require an equivalent on CentOS when an rpm
// cloud archive is up and running:
//
// "cloud-utils":		"???",
// "cloud-image-utils":	"???",
}

// configureCloudArchiveSourceUbuntu is a helper function which returns the
// cloud archive PackageSource and PackagePreferences for the given series for
// Ubuntu machines.
func configureCloudArchiveSourceUbuntu(series string) (packaging.PackageSource, packaging.PackagePreferences) {
	source := packaging.PackageSource{
		URL: fmt.Sprintf("deb %s %s-updates/cloud-tools main", UbuntuCloudArchiveUrl, series),
		Key: UbuntuCloudArchiveSigningKey,
	}

	prefs := packaging.PackagePreferences{
		Path:        UbuntuCloudToolsPrefsPath,
		Explanation: "Pin with lower priority, not to interfere with charms.",
		Package:     "*",
		Pin:         fmt.Sprintf("release n=%s-updates/cloud-tools", series),
		Priority:    400,
	}

	return source, prefs
}

// getTargetReleaseSpecifierUbuntu returns the specifier that can be passed to
// apt in order to ensure that it pulls the package from that particular source.
func getTargetReleaseSpecifierUbuntu(series string) string {
	switch series {
	case "precise":
		return "precise-updates/cloud-tools"
	default:
		return ""
	}
}
