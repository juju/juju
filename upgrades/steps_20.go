// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"os"
	"path/filepath"
)

// stateStepsFor20 returns upgrade steps for Juju 2.0 that manipulate state directly.
func stateStepsFor20() []Step {
	return []Step{
		&upgradeStep{
			description: "strip @local from local user names",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().StripLocalUserDomain()
			},
		},
		&upgradeStep{
			description: "rename addmodel permission to add-model",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RenameAddModelPermission()
			},
		},
	}
}

// stepsFor20 returns upgrade steps for Juju 2.0 that only need the API.
func stepsFor20() []Step {
	return []Step{
		&upgradeStep{
			description: "remove apiserver charm get cache",
			targets:     []Target{Controller},
			run:         removeCharmGetCache,
		},
	}
}

// removeCharmGetCache removes the cache directory that was previously
// used by the charms API endpoint. It is no longer necessary.
func removeCharmGetCache(context Context) error {
	dataDir := context.AgentConfig().DataDir()
	cacheDir := filepath.Join(dataDir, "charm-get-cache")
	return os.RemoveAll(cacheDir)
}
