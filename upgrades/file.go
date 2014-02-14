// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"io/ioutil"
	"os"
	"path/filepath"
)

// WriteReplacementFile create a file called filename with the specified
// contents, allowing for the fact that filename may already exist.
// It first writes data to a temp file and then renames so that if the
// writing fails, any original content is preserved.
func WriteReplacementFile(filename string, data []byte, perm os.FileMode) error {
	// Write the data to a temp file
	confDir := filepath.Dir(filename)
	tempDir, err := ioutil.TempDir(confDir, "")
	if err != nil {
		return err
	}
	tempFile := filepath.Join(tempDir, "newfile")
	defer os.RemoveAll(tempDir)
	err = ioutil.WriteFile(tempFile, []byte(data), perm)
	if err != nil {
		return err
	}
	return os.Rename(tempFile, filename)
}
