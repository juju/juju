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
func WriteReplacementFile(filename string, data []byte) error {
	// Write the data to a temp file
	confDir := filepath.Dir(filename)
	tempFile, err := ioutil.TempFile(confDir, "")
	if err != nil {
		return err
	}
	defer os.Remove(tempFile.Name())
	err = ioutil.WriteFile(tempFile.Name(), []byte(data), 0644)
	if err != nil {
		return err
	}
	return os.Rename(tempFile.Name(), filename)
}
