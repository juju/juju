// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"encoding/base64"
	"fmt"

	"github.com/juju/utils/v4"
)

// addFile is a helper function returns all the required shell commands to write
// a file (be it text or binary) with regards to the given parameters
// NOTE: if the file already exists, it will be overwritten.
func addFileCmds(filename string, data []byte, mode uint, binary bool) []string {
	// Note: recent versions of cloud-init have the "write_files"
	// module, which can write arbitrary files. We currently support
	// 12.04 LTS, which uses an older version of cloud-init without
	// this module.
	// TODO (aznashwan): eagerly await 2017 and to do the right thing here
	p := utils.ShQuote(filename)

	cmds := []string{fmt.Sprintf("install -D -m %o /dev/null %s", mode, p)}
	// Don't use the shell's echo builtin here; the interpretation
	// of escape sequences differs between shells, namely bash and
	// dash. Instead, we use printf (or we could use /bin/echo).
	if binary {
		encoded := base64.StdEncoding.EncodeToString(data)
		cmds = append(cmds, fmt.Sprintf(`echo -n %s | base64 -d > %s`, encoded, p))
	} else {
		cmds = append(cmds, fmt.Sprintf(`echo %s > %s`, utils.ShQuote(string(data)), p))
	}

	return cmds
}

// addFileCopyCmds is a helper function returns all the required shell commands to copy
// a file (be it text or binary) with regards to the given parameters
// NOTE: if the file already exists, it will be overwritten.
func addFileCopyCmds(source string, filename string, mode uint) []string {
	s := utils.ShQuote(source)
	p := utils.ShQuote(filename)

	cmds := []string{fmt.Sprintf("install -D -m %o /dev/null %s", mode, p)}
	cmds = append(cmds, fmt.Sprintf(`cat %s > %s`, s, p))

	return cmds
}

// removeStringFromSlice is a helper function which removes a given string from
// the given slice, if it exists it returns the slice, be it modified or unmodified
func removeStringFromSlice(slice []string, val string) []string {
	for i, str := range slice {
		if str == val {
			slice = append(slice[:i], slice[i+1:]...)
		}
	}

	return slice
}
