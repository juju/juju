// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package commands

const (
	// snap binary name.
	snapBinary = "snap"

	snapNoProxySettingFormat = `proxy.%s=""`
	snapProxySettingFormat   = `proxy.%s=%q`
)

// snapCmder is the packageCommander instantiation for snap-based systems.
var snapCmder = packageCommander{
	update:        makeNopCmd(),
	upgrade:       buildCommand(snapBinary, "refresh"),
	install:       buildCommand(snapBinary, "install"),
	addRepository: makeNopCmd(),
	// Note: proxy.{http,https} available since snapd 2.28
	proxySettingsFormat:   snapProxySettingFormat,
	noProxySettingsFormat: snapNoProxySettingFormat,
	setProxy:              buildCommand(snapBinary, "set system %s"),
}

func makeNopCmd() string {
	return buildCommand(":", "#No action here")
}
