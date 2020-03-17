// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import "os"

// defaultFileSystem implements the FileSystem for the DistroInfo.
type defaultFileSystem struct{}

func (defaultFileSystem) Open(path string) (*os.File, error) {
	return os.Open(path)
}
