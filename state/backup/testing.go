// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	gc "launchpad.net/gocheck"
)

// CheckFileCreated verifies that the filename was created on disk.
func CheckFileCreated(c *gc.C, filename string) bool {
	_, err := os.Stat(filename)
	created := err == nil
	if !created {
		c.Errorf("file not created: %q", filename)
	}
	return created
}

// CheckFileContents verifies the contents of the file against `expected`.
func CheckFileContents(c *gc.C, filename string, expected []byte) bool {
	content, err := ioutil.ReadFile(filename)
	c.Assert(err, gc.IsNil)
	return c.Check(string(content), gc.Equals, string(expected))
}

// CheckFileHash verifies the file's hash against the expected hash.
func CheckFileHash(c *gc.C, filename, expected string) bool {
	// Not windows-compatible?
	out, err := exec.Command("sha1sum", "--binary", filename).Output()
	c.Assert(err, gc.IsNil)
	hash := strings.Fields(string(out))[0]
	return c.Check(hash, gc.Equals, expected)
}

// TODO(ericsnow) Move to github.com/juju/utils/tar.
func iterTarFile(tarfile io.Reader) (next func() (*tar.Header, error)) {
	// Make a lua-style "iterator".
	entries := tar.NewReader(tarfile)
	done := false
	next = func() (*tar.Header, error) {
		if done {
			return nil, nil
		}
		hdr, err := entries.Next()
		if err == io.EOF {
			// end of archive
			done = true
			return nil, nil
		}
		if err != nil {
			// XXX Set done to true?
			return nil, fmt.Errorf("error iterating over tarfile: %v", err)
		}
		return hdr, nil
	}
	return next
}

// CheckTarball verifies the file as a valid tarball and that it has the
// expected files inside.  `expected` does not necessarily include every
// filename expected to be in the tarball.
func CheckTarball(c *gc.C, filename string, expected []string) bool {
	filenames := make([]string, len(expected))
	copy(filenames, expected)

	tarfile, err := os.Open(filename)
	c.Assert(err, gc.IsNil)

	nextEntry := iterTarFile(tarfile)
	for len(filenames) != 0 {
		header, err := nextEntry()
		c.Assert(err, gc.IsNil)
		if header == nil {
			break
		}
		for i, name := range filenames {
			if name == header.Name {
				filenames[i] = filenames[0]
				filenames = filenames[1:]
				break
			}
		}
	}
	c.Assert(filenames, gc.Equals, []string{})
	return false
}

// CheckArchive verfies that the backup archive is valid.
func CheckArchive(c *gc.C, filename, hash string, raw []byte, filenames []string) bool {
	if !CheckFileCreated(c, filename) {
		return false
	}
	res := true
	if raw != nil {
		res = res || CheckFileContents(c, filename, raw)
	}
	res = res || CheckFileHash(c, filename, hash)
	if filenames != nil {
		res = res || CheckTarball(c, filename, filenames)
	}
	return res
}
