// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config

import (
	"github.com/juju/utils/packaging"
)

// yumConfigurer is the PackagingConfigurer implementation for apt-based systems.
type yumConfigurer struct {
	*baseConfigurer
}

// RenderSource is defined on the PackagingConfigurer interface.
func (c *yumConfigurer) RenderSource(src packaging.PackageSource) (string, error) {
	return src.RenderSourceFile(YumSourceTemplate)
}

// RenderPreferences is defined on the PackagingConfigurer interface.
func (c *yumConfigurer) RenderPreferences(src packaging.PackagePreferences) (string, error) {
	// TODO (aznashwan): research a way of using yum-priorities in the context
	// of single/multiple package pinning and implement it.
	return "", nil
}

// ApplyCloudArchiveTarget is defined on the PackagingConfigurer interface.
func (c *yumConfigurer) ApplyCloudArchiveTarget(pack string) []string {
	// TODO (aznashwan): implement target application when archive is available.
	return []string{pack}
}
