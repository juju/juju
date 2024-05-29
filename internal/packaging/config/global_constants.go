// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config

const (
	// PackageManagerLoopFunction is a bash function that executes its arguments
	// in a loop with a delay until either the command either returns
	// with an exit code other than 100. It times out after 5 failed attempts.
	PackageManagerLoopFunction = `
function package_manager_loop {
    local attempts=0
    local rc=
    while true; do
        attempts=$((attempts+1))
        if ($*); then
                return 0
        else
                rc=$?
        fi
        if [ $attempts -lt 5 -a $rc -eq 100 ]; then
                sleep 10s
                continue
        fi
        return $rc
    done
}
`
)

var (
	// DefaultPackages is a list of the default packages Juju'd like to see
	// installed on all it's machines.
	DefaultPackages = []string{
		// TODO (everyone): populate this list with all required packages.
		// for example:
		"curl",
	}
)
