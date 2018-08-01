// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config

const (
	// PackageManagerLoopFunction is a bash function that executes its arguments
	// in a loop with a delay until either the command either returns
	// with an exit code other than 100.
	PackageManagerLoopFunction = `
function package_manager_loop {
    local rc=
    while true; do
        if ($*); then
                return 0
        else
                rc=$?
        fi
        if [ $rc -eq 100 ]; then
                sleep 10s
                continue
        fi
        return $rc
    done
}
`
)

var (
	seriesRequiringCloudTools = map[string]bool{
		// TODO (aznashwan, all): add any other OS's
		// which require cloud tools' series here.
		"precise": true,
	}

	// DefaultPackages is a list of the default packages Juju'd like to see
	// installed on all it's machines.
	DefaultPackages = []string{
		// TODO (everyone): populate this list with all required packages.
		// for example:
		"curl",
	}

	backportsBySeries = map[string][]string{
		"trusty": []string{"lxd"},
	}
)
