// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package shell

import (
	"fmt"

	"launchpad.net/juju-core/utils"
)

// DumpFileOnErrorScript returns a bash script that
// may be used to dump the contents of the specified
// file to stderr when the shell exits with an error.
func DumpFileOnErrorScript(filename string) string {
	script := `
dump_file() {
    code=$?
    if [ $code -ne 0 -a -e %s ]; then
        cat %s >&2
    fi
    exit $code
}
trap dump_file EXIT
`[1:]
	filename = utils.ShQuote(filename)
	return fmt.Sprintf(script, filename, filename)
}
