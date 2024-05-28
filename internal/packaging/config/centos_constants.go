// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config

const (
	// CentOSCloudArchiveUrl is the url of the (future) cloud archive for CentOS.
	// TODO (people of the distant future): add this when it is available.
	// CentOSCloudArchiveUrl = "http://fill-me-in.com/cloud-archive.repo"

	// CentOSSourcesFile is the default file which lists all core sources
	// for yum packages on CentOS.
	CentOSSourcesFile = "/etc/yum/repos.d/CentOS-Base.repo"

	// ReplaceCentOSMirror is a mini-script which replaces the default CentOS
	// mirros with the one formatted in.
	ReplaceCentOSMirror = "sed -r -i -e 's|^mirrorlist|#mirrorlist|g' -e 's|#baseurl=.*|baseurl=%s|g' " +
		CentOSSourcesFile
)

// CentOSDefaultPackages is the default package set we'd like installed
// on all CentOS machines.
var CentOSDefaultPackages = append(DefaultPackages, []string{
	// TODO (aznashwan, all): further populate this list.
	"epel-release",
	"yum-utils",
}...)

// cloudArchivePackagesCentOS maintains a list of CentOS packages that
// Configurer.IsCloudArchivePackage will reference when determining the
// repository from which to install a package.
var cloudArchivePackagesCentOS = map[string]struct{}{
	// TODO (aznashwan, all): if a separate repository for
	// CentOS 7+ especially for Juju is to ever occur, please add the relevant
	// package listings here.
}
