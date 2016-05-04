// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package commands

// Commands to patch
var patchedCommands = []string{"ssh", "scp"}

// fakecommand outputs its arguments to stdout for verification
var fakecommand = `#!/bin/bash

{
    echo "$@"

    # If a custom known_hosts file was passed, emit the contents of
    # that too.
    while (( "$#" )); do
        if [[ $1 = UserKnownHostsFile* ]]; then
            IFS=" " read -ra parts <<< $1
            cat "${parts[1]}"
            break
        fi
        shift
    done
}| tee $0.args



`
