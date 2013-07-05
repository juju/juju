// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package local

import (
	"launchpad.net/juju-core/environs/config"
)

var Provider = provider

func SetDefaultRootDir(rootdir string) (old string) {
	old, defaultRootDir = defaultRootDir, rootdir
	return
}

func ConfigNamespace(cfg *config.Config) string {
	localConfig, _ := provider.newConfig(cfg)
	return localConfig.namespace()
}
