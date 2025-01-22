// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config

import (
	"text/template"
)

const (
	// LegacyAptSourcesFile is the default file which list all core
	// sources for apt packages on an apt-based system for ubuntu
	// distros before noble
	LegacyAptSourcesFile = "/etc/apt/sources.list"

	// LegacyAptSourcesFile is the default file which list all core
	// sources for apt packages on an apt-based system for ubuntu
	// distros noble or after
	AptSourcesFile = "/etc/apt/sources.list.d/ubuntu.sources"

	// AptListsDirectory is the location of the APT sources list.
	AptListsDirectory = "/var/lib/apt/lists"

	// AptConfigDirectory is the default directory in which
	// apt configuration files are stored.
	AptConfigDirectory = "/etc/apt/apt.conf.d"

	// AptSourceListPrefix is a shell program that translates an
	// APT source (piped from stdin) to a file prefix. The algorithm
	// involves stripping up to one trailing slash, stripping the
	// URL scheme prefix, and finally translating slashes to
	// underscores.
	AptSourceListPrefix = `sed 's,.*://,,' | sed 's,/$,,' | tr / _`
)

var (
	// AptProxyConfigFile is the full file path for the proxy settings that are
	// written by cloudinit and the machine environ worker.
	AptProxyConfigFile = AptConfigDirectory + "/95-juju-proxy-settings"

	// AptPreferenceTemplate is the template specific to an apt preference file.
	AptPreferenceTemplate = template.Must(template.New("").Parse(`
Explanation: {{.Explanation}}
Package: {{.Package}}
Pin: {{.Pin}}
Pin-Priority: {{.Priority}}
`[1:]))
)
