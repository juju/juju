// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package client_test

var expectedCommand = []string{
	"juju-run --no-context 'hostname'\n",
	"juju-run magic/0 'hostname'\n",
	"juju-run magic/1 'hostname'\n",
}

var echoInputShowArgs = `#!/bin/bash
# Write the args to stderr
echo "$*" >&2
# And echo stdin to stdout
while read line
do echo $line
done <&0
`

var echoInput = `#!/bin/bash
# And echo stdin to stdout
while read line
do echo $line
done <&0
`
