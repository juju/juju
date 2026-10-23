// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/tc"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/recovery"
)

func (s *validateSuite) TestReadArchiveStagesModelDumps(c *tc.C) {
	files := validFiles()
	archivePath, sum := writeArchive(c, withManifest(c, files))
	stagingDir := c.MkDir()
	c.Setenv("TMPDIR", stagingDir)

	contents, info, err := recovery.ReadArchive(c.Context(), loggertesting.WrapCheckLog(c), archivePath, sum)
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { c.Check(contents.Close(), tc.ErrorIsNil) })
	c.Check(info.Checksum, tc.Equals, sum)
	c.Check(contents.ControllerDump, tc.DeepEquals, files["juju-backup/dump/controller.yaml"])
	c.Check(contents.ModelDumps, tc.HasLen, 3)
	for modelUUID, filename := range contents.ModelDumps {
		c.Check(filepath.Dir(filepath.Dir(filename)), tc.Equals, stagingDir)
		data, err := os.ReadFile(filename)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(data, tc.DeepEquals, files["juju-backup/dump/models/"+modelUUID+".yaml"])
		stat, err := os.Stat(filename)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(stat.Mode().Perm(), tc.Equals, os.FileMode(0600))
	}

	c.Assert(contents.Close(), tc.ErrorIsNil)
	entries, err := os.ReadDir(stagingDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.HasLen, 0)
}

func (s *validateSuite) TestValidateArchiveCleansUpStaging(c *tc.C) {
	invalidMetadata := validFiles()
	invalidMetadata["juju-backup/metadata.json"] = []byte("invalid json")
	missingModel := validFiles()
	delete(missingModel, "juju-backup/dump/models/"+testModelAUUID+".yaml")
	invalidModel := k8sFiles("payload: [invalid yaml")
	invalidManifest := withManifest(c, validFiles())
	invalidManifest["juju-backup/dump/models/"+testModelAUUID+".yaml"] = []byte("tampered")
	for _, test := range []struct {
		name     string
		files    map[string][]byte
		checksum string
		error    string
		cancel   bool
	}{
		{name: "success", files: validFiles()},
		{name: "checksum", files: validFiles(), checksum: "deadbeef", error: "archive checksum mismatch:.*"},
		{name: "metadata", files: invalidMetadata, error: "parsing juju-backup/metadata.json:.*"},
		{name: "model inventory", files: missingModel, error: "archive is missing the database dump.*"},
		{name: "model decode", files: invalidModel, error: "model .*: decoding dump:.*"},
		{name: "manifest", files: invalidManifest, error: "juju-backup/manifest.json records.*"},
		{name: "cancelled", files: validFiles(), cancel: true},
	} {
		c.Logf("%s", test.name)
		archivePath, sum := writeArchive(c, test.files)
		stagingDir := c.MkDir()
		c.Setenv("TMPDIR", stagingDir)
		if test.checksum != "" {
			sum = test.checksum
		}
		ctx, cancel := context.WithCancel(c.Context())
		if test.cancel {
			cancel()
		}
		_, err := recovery.ValidateArchive(ctx, loggertesting.WrapCheckLog(c), archivePath, sum)
		cancel()
		if test.cancel {
			c.Check(err, tc.ErrorIs, context.Canceled)
		} else if test.error == "" {
			c.Assert(err, tc.ErrorIsNil)
		} else {
			c.Check(err, tc.ErrorMatches, test.error)
		}
		entries, err := os.ReadDir(stagingDir)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(entries, tc.HasLen, 0)
	}
}

func (s *validateSuite) TestReadArchiveRejectsOversizedModelDump(c *tc.C) {
	// Write only the header: rejection must precede reading the contents,
	// so this test needs neither a large allocation nor a large file.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	c.Assert(tw.WriteHeader(&tar.Header{
		Name:     "juju-backup/dump/models/" + testModelAUUID + ".yaml",
		Mode:     0600,
		Size:     recovery.MaxDumpSize + 1,
		Typeflag: tar.TypeReg,
	}), tc.ErrorIsNil)
	c.Assert(gz.Close(), tc.ErrorIsNil)
	archivePath := filepath.Join(c.MkDir(), "oversized.tar.gz")
	c.Assert(os.WriteFile(archivePath, buf.Bytes(), 0600), tc.ErrorIsNil)
	stagingDir := c.MkDir()
	c.Setenv("TMPDIR", stagingDir)

	_, _, err := recovery.ReadArchive(c.Context(), loggertesting.WrapCheckLog(c), archivePath, "")
	c.Check(err, tc.ErrorMatches, fmt.Sprintf("reading .*: file exceeds %d bytes", recovery.MaxDumpSize))
	entries, err := os.ReadDir(stagingDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.HasLen, 0)
}

func (s *validateSuite) TestUnpackObjectStore(c *tc.C) {
	files := validFiles()
	files["juju-backup/root.tar"] = rootTar(c)
	archivePath, _ := writeArchive(c, files)

	dest := c.MkDir()
	n, err := recovery.UnpackObjectStore(c.Context(), archivePath, dest)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(n, tc.Equals, 1)

	content, err := os.ReadFile(filepath.Join(dest, "objectstore", "some-ns", "somehash"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(content), tc.Equals, "blob content")
}

func (s *validateSuite) TestUnpackObjectStoreOverwritesPresentBlob(c *tc.C) {
	// The replacement's own bootstrap blobs are already on disk when
	// recovery unpacks: identical content is keyed by the same hash, so
	// unpacking overwrites rather than failing.
	files := validFiles()
	files["juju-backup/root.tar"] = rootTar(c)
	archivePath, _ := writeArchive(c, files)

	dest := c.MkDir()
	existing := filepath.Join(dest, "objectstore", "some-ns", "somehash")
	c.Assert(os.MkdirAll(filepath.Dir(existing), 0o700), tc.ErrorIsNil)
	c.Assert(os.WriteFile(existing, []byte("stale"), 0o600), tc.ErrorIsNil)

	n, err := recovery.UnpackObjectStore(c.Context(), archivePath, dest)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(n, tc.Equals, 1)

	content, err := os.ReadFile(existing)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(content), tc.Equals, "blob content")
}

func (s *validateSuite) TestUnpackObjectStoreRejectsTraversal(c *tc.C) {
	var inner bytes.Buffer
	tw := tar.NewWriter(&inner)
	content := []byte("x")
	c.Assert(tw.WriteHeader(&tar.Header{
		Name: "../escape", Mode: 0o600, Size: 1, Typeflag: tar.TypeReg,
	}), tc.ErrorIsNil)
	_, err := tw.Write(content)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tw.Close(), tc.ErrorIsNil)

	files := validFiles()
	files["juju-backup/root.tar"] = inner.Bytes()
	archivePath, _ := writeArchive(c, files)

	_, err = recovery.UnpackObjectStore(c.Context(), archivePath, c.MkDir())
	c.Assert(err, tc.ErrorMatches, "object archive contains unsafe path .*")
}

func (s *validateSuite) TestUnpackObjectStoreSkipsNonObjectStoreEntries(c *tc.C) {
	// Entries outside the object store subtree — k8s controllers archive
	// their tools binaries as symlinks — are never unpacked. The symlink
	// comes FIRST here: skipping must never stop the rest of the archive
	// from being unpacked.
	var inner bytes.Buffer
	tw := tar.NewWriter(&inner)
	c.Assert(tw.WriteHeader(&tar.Header{
		Name:     "var/lib/juju/tools/controller-0/jujuagentd",
		Typeflag: tar.TypeSymlink, Linkname: "/charm/bin/containeragent",
	}), tc.ErrorIsNil)
	content := []byte("blob content")
	c.Assert(tw.WriteHeader(&tar.Header{
		Name: "var/lib/juju/objectstore/ns/somehash", Mode: 0o600,
		Size: int64(len(content)), Typeflag: tar.TypeReg,
	}), tc.ErrorIsNil)
	_, err := tw.Write(content)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tw.Close(), tc.ErrorIsNil)

	files := validFiles()
	files["juju-backup/root.tar"] = inner.Bytes()
	archivePath, _ := writeArchive(c, files)

	dest := c.MkDir()
	n, err := recovery.UnpackObjectStore(c.Context(), archivePath, dest)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(n, tc.Equals, 1)

	got, err := os.ReadFile(filepath.Join(dest, "objectstore", "ns", "somehash"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(got), tc.Equals, "blob content")
	entries, err := os.ReadDir(dest)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.HasLen, 1)
	c.Check(entries[0].Name(), tc.Equals, "objectstore")
}

func (s *validateSuite) TestUnpackObjectStoreAcceptsTypeRegA(c *tc.C) {
	// Archives written by tars that do not set the typeflag carry
	// regular files as TypeRegA; the archive must still be found.
	var inner bytes.Buffer
	tw := tar.NewWriter(&inner)
	content := []byte("old-tar blob")
	c.Assert(tw.WriteHeader(&tar.Header{
		Name: "var/lib/juju/objectstore/ns/oldhash", Mode: 0o600,
		Size: int64(len(content)), Typeflag: tar.TypeRegA,
	}), tc.ErrorIsNil)
	_, err := tw.Write(content)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tw.Close(), tc.ErrorIsNil)

	files := validFiles()
	files["juju-backup/root.tar"] = inner.Bytes()
	archivePath, _ := writeArchive(c, files)

	dest := c.MkDir()
	n, err := recovery.UnpackObjectStore(c.Context(), archivePath, dest)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(n, tc.Equals, 1)

	got, err := os.ReadFile(filepath.Join(dest, "objectstore", "ns", "oldhash"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(got), tc.Equals, "old-tar blob")
}
