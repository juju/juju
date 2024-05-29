// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config

import (
	"github.com/juju/juju/internal/packaging/source"
)

// aptConfigurer is the PackagingConfigurer implementation for apt-based systems.
type aptConfigurer struct {
	*baseConfigurer
}

// RenderSource is defined on the PackagingConfigurer interface.
func (c *aptConfigurer) RenderSource(src source.PackageSource) (string, error) {
	return src.RenderSourceFile(AptSourceTemplate)
}

// RenderPreferences is defined on the PackagingConfigurer interface.
func (c *aptConfigurer) RenderPreferences(prefs source.PackagePreferences) (string, error) {
	return prefs.RenderPreferenceFile(AptPreferenceTemplate)
}

// ApplyCloudArchiveTarget is defined on the PackagingConfigurer interface.
func (c *aptConfigurer) ApplyCloudArchiveTarget(pack string) []string {
	return []string{pack}
}
