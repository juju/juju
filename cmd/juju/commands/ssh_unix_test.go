// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package commands

// Commands to patch
var patchedCommands = []string{"ssh", "scp"}

// fakecommand outputs its arguments to stdout for verification
var fakecommand = `#!/bin/bash

echo "$@" | tee $0.args
`
