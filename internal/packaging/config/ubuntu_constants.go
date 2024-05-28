// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config

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
	"cloud-image-utils":       {},
	"cloud-utils":             {},
	"curtin":                  {},
	"djorm-ext-pgarray":       {},
	"golang":                  {},
	"iproute2":                {},
	"isc-dhcp":                {},
	"juju-core":               {},
	"libseccomp":              {},
	"libv8-3.14":              {},
	"lxc":                     {},
	"maas":                    {},
	"mongodb":                 {},
	"mongodb-server":          {},
	"python-django":           {},
	"python-django-piston":    {},
	"python-jujuclient":       {},
	"python-tx-tftp":          {},
	"python-websocket-client": {},
	"raphael 2.1.0-1ubuntu1":  {},
	"simplestreams":           {},
	"txlongpoll":              {},
	"uvtool":                  {},
	"yui3":                    {},
}
