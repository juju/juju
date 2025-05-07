// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/loggo/v2"
	"github.com/juju/tc"

	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	coretesting "github.com/juju/juju/internal/testing"
)

type SignMetadataSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(&SignMetadataSuite{})

func (s *SignMetadataSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	loggo.GetLogger("").SetLogLevel(loggo.INFO)
}

var expectedLoggingOutput = `signing 2 file\(s\) in .*subdir1.*
signing file .*file1\.json.*
signing file .*file2\.json.*
signing 1 file\(s\) in .*subdir2.*
signing file .*file3\.json.*
`

func makeFileNames(topLevel string) []string {
	return []string{
		filepath.Join(topLevel, "subdir1", "file1.json"),
		filepath.Join(topLevel, "subdir1", "file2.json"),
		filepath.Join(topLevel, "subdir1", "subdir2", "file3.json"),
	}
}

func setupJsonFiles(c *tc.C, topLevel string) {
	err := os.MkdirAll(filepath.Join(topLevel, "subdir1", "subdir2"), 0700)
	c.Assert(err, tc.ErrorIsNil)
	content := []byte("hello world")
	filenames := makeFileNames(topLevel)
	for _, filename := range filenames {
		err = os.WriteFile(filename, content, 0644)
		c.Assert(err, tc.ErrorIsNil)
	}
}

func assertSignedFile(c *tc.C, filename string) {
	r, err := os.Open(filename)
	c.Assert(err, tc.ErrorIsNil)
	defer r.Close()
	data, err := simplestreams.DecodeCheckSignature(r, sstesting.SignedMetadataPublicKey)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, "hello world\n")
}

func assertSignedFiles(c *tc.C, topLevel string) {
	filenames := makeFileNames(topLevel)
	for _, filename := range filenames {
		filename = strings.Replace(filename, ".json", ".sjson", -1)
		assertSignedFile(c, filename)
	}
}

func (s *SignMetadataSuite) TestSignMetadata(c *tc.C) {
	topLevel := c.MkDir()
	keyfile := filepath.Join(topLevel, "privatekey.asc")
	err := os.WriteFile(keyfile, []byte(sstesting.SignedMetadataPrivateKey), 0644)
	c.Assert(err, tc.ErrorIsNil)
	setupJsonFiles(c, topLevel)

	ctx := cmdtesting.Context(c)
	code := cmd.Main(
		newSignMetadataCommand(), ctx, []string{"-d", topLevel, "-k", keyfile, "-p", sstesting.PrivateKeyPassphrase})
	c.Assert(code, tc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(output, tc.Matches, expectedLoggingOutput)
	assertSignedFiles(c, topLevel)
}

func runSignMetadata(c *tc.C, args ...string) error {
	_, err := cmdtesting.RunCommand(c, newSignMetadataCommand(), args...)
	return err
}

func (s *SignMetadataSuite) TestSignMetadataErrors(c *tc.C) {
	err := runSignMetadata(c, "")
	c.Assert(err, tc.ErrorMatches, `directory must be specified`)
	err = runSignMetadata(c, "-d", "foo")
	c.Assert(err, tc.ErrorMatches, `keyfile must be specified`)
}
