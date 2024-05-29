// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config_test

import "github.com/juju/juju/internal/packaging/source"

var (
	testedSource = source.PackageSource{
		Name: "Some Totally Official Source.",
		URL:  "some-source.com/packages",
		Key:  "some-key",
	}

	testedPrefs = source.PackagePreferences{
		Path:        "/etc/my-package-manager.d/prefs_file.conf",
		Explanation: "don't judge me",
		Package:     "some-package",
		Pin:         "releases/extra-special",
		Priority:    42,
	}
)
