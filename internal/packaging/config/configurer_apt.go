// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config

import (
	"github.com/juju/juju/internal/packaging/source"
)

// AptConfigurer is the PackagingConfigurer implementation for apt-based systems.
type AptConfigurer struct{}

// NewAptPackagingConfigurer returns a PackagingConfigurer for apt-based systems.
func NewAptPackagingConfigurer() AptConfigurer {
	return AptConfigurer{}
}

// RenderPreferences is defined on the PackagingConfigurer interface.
func (c AptConfigurer) RenderPreferences(prefs source.PackagePreferences) (string, error) {
	return prefs.RenderPreferenceFile(AptPreferenceTemplate)
}
