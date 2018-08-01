// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package ssh

import (
	"io"
)

// WrapStdin returns stdin with carriage returns stripped on windows.
func WrapStdin(reader io.Reader) io.Reader {
	return StripCRReader(reader)
}
