// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package wrench

import "os"

// Windows is not fully POSIX compliant
// there is no syscall.Stat_t on Windows
func isOwnedByJujuUser(fi os.FileInfo) bool {
	return true
}
