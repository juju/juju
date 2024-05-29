// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config

import "github.com/juju/juju/internal/packaging/source"

// yumConfigurer is the PackagingConfigurer implementation for apt-based systems.
type yumConfigurer struct {
	*baseConfigurer
}

// RenderSource is defined on the PackagingConfigurer interface.
func (c *yumConfigurer) RenderSource(src source.PackageSource) (string, error) {
	return src.RenderSourceFile(YumSourceTemplate)
}

// RenderPreferences is defined on the PackagingConfigurer interface.
func (c *yumConfigurer) RenderPreferences(src source.PackagePreferences) (string, error) {
	// TODO (aznashwan): research a way of using yum-priorities in the context
	// of single/multiple package pinning and implement it.
	return "", nil
}

// ApplyCloudArchiveTarget is defined on the PackagingConfigurer interface.
func (c *yumConfigurer) ApplyCloudArchiveTarget(pack string) []string {
	// TODO (aznashwan): implement target application when archive is available.
	return []string{pack}
}
