// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/juju/juju/version"
)

var VERSION_NOTICE = fmt.Sprintf(`
[note type=caution]
The information in this doc is based on Juju version %v,
and may not accurately reflect other versions of Juju.
[/note]

`[1:], version.Current.String())

// For every doc in $DOCS_DIR, add VERSION_NOTICE to the top.
func main() {
	docsDir := mustEnv("DOCS_DIR") // directory to write output to

	check(filepath.WalkDir(docsDir, func(path string, file fs.DirEntry, err error) error {
		if file.IsDir() {
			// skip
			return nil
		}

		// Create new temp file to store the output
		tmpFile, err := os.CreateTemp(docsDir, file.Name())
		check(err)

		// Write version notice header
		_, err = tmpFile.WriteString(VERSION_NOTICE)
		check(err)

		// Open original file and copy all to the new file
		oldFile, err := os.Open(path)
		check(err)
		_, err = io.Copy(tmpFile, oldFile)
		check(err)

		// Delete old file and move new one to original place
		check(oldFile.Close())
		check(os.Remove(path))
		check(os.Rename(tmpFile.Name(), path))
		check(tmpFile.Close())
		return nil
	}))
}

// UTILITY FUNCTIONS

// Returns the value of the given environment variable, panicking if the var
// is not set.
func mustEnv(key string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		panic(fmt.Sprintf("env var %q not set", key))
	}
	return val
}

// check panics if the provided error is not nil.
func check(err error) {
	if err != nil {
		panic(err)
	}
}
