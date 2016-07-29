// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package filetesting

import (
	"os"
)

// isNotExist returns true if the error is consistent with an attempt to
// reference a file that does not exist.
func isNotExist(err error) bool {
	return os.IsNotExist(err)
}
