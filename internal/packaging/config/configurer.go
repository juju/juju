// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config

// baseConfigurer is the base type of a Configurer object.
type baseConfigurer struct {
	defaultPackages      []string
	cloudArchivePackages map[string]struct{}
}

// DefaultPackages is defined on the PackagingConfigurer interface.
func (c *baseConfigurer) DefaultPackages() []string {
	return c.defaultPackages
}

// IsCloudArchivePackage is defined on the PackagingConfigurer interface.
func (c *baseConfigurer) IsCloudArchivePackage(pack string) bool {
	_, ok := c.cloudArchivePackages[pack]
	return ok
}
