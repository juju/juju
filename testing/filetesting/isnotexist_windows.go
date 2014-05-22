// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filetesting

import (
	"os"
)

// IsNotExist returns true if the error is consistent with an attempt to
// reference a file that does not exist.
func IsNotExist(err error) bool {
	return os.IsNotExist(err)
}
