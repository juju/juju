// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"bufio"
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

// CheckTarball verifies the file as a valid tarball and that it has the
// expected files inside.  `expected` does not necessarily include every
// filename expected to be in the tarball.
func CheckTarball(c *gc.C, filename string, expected []string) bool {
	// Not windows-compatible?
	args := []string{
		"--list",
		"--file=" + filename,
	}
	cmd := exec.Command("tar", args...)
	pipe, err := cmd.StdoutPipe()
	c.Assert(err, gc.IsNil)
	err = cmd.Start()
	c.Assert(err, gc.IsNil)

	lines := bufio.NewReader(pipe)
	for {
		nameBytes, _, err := lines.ReadLine()
		if err == io.EOF {
			break
		}
		c.Assert(err, gc.IsNil)
		name := string(nameBytes)
		// Skip blank lines.
		if name == "" {
			continue
		}
		// Look for a match.
		for i, expName := range expected {
			if name == expName {
				// Delete the element.
				expected = append(expected[:i], expected[i+1:]...)
				break
			}
		}
		// Stop early.
		if len(expected) == 0 {
			break
		}
	}

	err = cmd.Wait()
	if c.Check(err, gc.IsNil) {
		return c.Check(expected, gc.Equals, []string{})
	}
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
