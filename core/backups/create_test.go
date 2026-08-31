// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/core/backups"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/testing"
)

// testStarted keeps the metadata timestamps, and so the archive
// filename, deterministic.
var testStarted = time.Date(2024, time.September, 9, 11, 59, 34, 0, time.UTC)

type createSuite struct {
	testing.BaseSuite
}

func TestCreateSuite(t *stdtesting.T) {
	tc.Run(t, &createSuite{})
}

func (s *createSuite) writeFile(c *tc.C, name, content string) string {
	path := filepath.Join(c.MkDir(), name)
	err := os.WriteFile(path, []byte(content), 0644)
	c.Assert(err, tc.ErrorIsNil)
	return path
}

func (s *createSuite) TestCreate(c *tc.C) {
	destDir := c.MkDir()
	file1 := s.writeFile(c, "jujud", "agent binary")
	file2 := s.writeFile(c, "system-identity", "ssh key")

	modelUUID := "deadbeef-0bad-400d-8000-4b1d0d06f00d"
	meta := backups.NewMetadata(testStarted)
	filename, err := backups.Create(meta, backups.CreateArgs{
		DestinationDir: destDir,
		Clock:          clock.WallClock,
		FilesToBackUp:  []string{file1, file2},
		DumpEntries: []backups.DumpEntry{{
			Name:   "controller.yaml",
			Reader: strings.NewReader("controller: data"),
		}, {
			Name:   "models/" + modelUUID + ".yaml",
			Reader: strings.NewReader("model: data"),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(filepath.Dir(filename), tc.Equals, destDir)
	c.Check(strings.HasPrefix(filepath.Base(filename),
		backups.FilenamePrefix), tc.IsTrue)

	// The metadata was marked complete.
	c.Check(meta.Checksum(), tc.Not(tc.Equals), "")
	c.Check(meta.Finished, tc.Not(tc.IsNil))

	archiveFile, err := os.Open(filename)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = archiveFile.Close() }()

	// BuildMetadata on the resulting archive agrees with the checksum
	// recorded during creation.
	built, err := backups.BuildMetadata(archiveFile)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(built.Checksum(), tc.Equals, meta.Checksum())

	_, err = archiveFile.Seek(0, io.SeekStart)
	c.Assert(err, tc.ErrorIsNil)
	ad, err := backups.NewArchiveDataReader(archiveFile)
	c.Assert(err, tc.ErrorIsNil)

	entries, contents := s.archiveEntries(c, ad)
	c.Check(entries, tc.DeepEquals, set.NewStrings(
		"juju-backup",
		"juju-backup/metadata.json",
		"juju-backup/root.tar",
		"juju-backup/dump",
		"juju-backup/dump/controller.yaml",
		"juju-backup/dump/models",
		"juju-backup/dump/models/"+modelUUID+".yaml",
	))
	c.Check(contents["juju-backup/dump/controller.yaml"],
		tc.Equals, "controller: data")
	c.Check(contents["juju-backup/dump/models/"+modelUUID+".yaml"],
		tc.Equals, "model: data")

	// The metadata file parses back to the current format version.
	storedMeta, err := backups.NewMetadataJSONReader(
		strings.NewReader(contents["juju-backup/metadata.json"]))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(storedMeta.FormatVersion, tc.Equals, int64(2))

	// root.tar bundles the files with the leading slash stripped.
	rootEntries, _ := s.tarEntries(c,
		strings.NewReader(contents["juju-backup/root.tar"]))
	c.Check(rootEntries.Contains(strings.TrimPrefix(file1, "/")), tc.IsTrue)
	c.Check(rootEntries.Contains(strings.TrimPrefix(file2, "/")), tc.IsTrue)

	// The staging directory was removed.
	staging, err := os.ReadDir(destDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(staging, tc.HasLen, 1)
	c.Check(staging[0].Name(), tc.Equals, filepath.Base(filename))
}

func (s *createSuite) archiveEntries(c *tc.C,
	ad *backups.ArchiveData,
) (set.Strings, map[string]string) {
	return s.tarEntries(c, ad.NewBuffer())
}

func (s *createSuite) tarEntries(c *tc.C,
	r io.Reader,
) (set.Strings, map[string]string) {
	names := set.NewStrings()
	contents := make(map[string]string)
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		c.Assert(err, tc.ErrorIsNil)
		names.Add(header.Name)
		if header.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			c.Assert(err, tc.ErrorIsNil)
			contents[header.Name] = string(data)
		}
	}
	return names, contents
}

func (s *createSuite) TestCreateDumpEntryNameEscapesDump(c *tc.C) {
	for _, name := range []string{
		"../evil.yaml",
		"models/../../evil.yaml",
		"..",
		"/etc/passwd",
		"/var/lib/juju/models/evil.yaml",
	} {
		_, err := backups.Create(backups.NewMetadata(testStarted), backups.CreateArgs{
			DestinationDir: c.MkDir(),
			Clock:          clock.WallClock,
			FilesToBackUp:  []string{s.writeFile(c, "file", "content")},
			DumpEntries: []backups.DumpEntry{{
				Name:   name,
				Reader: strings.NewReader("data"),
			}},
		})
		c.Check(err, tc.ErrorIs, coreerrors.NotValid,
			tc.Commentf("name %q", name))
	}
}

func (s *createSuite) TestCreateEmptyDumpEntryName(c *tc.C) {
	for _, name := range []string{"", ".", "./", "dump/.."} {
		_, err := backups.Create(backups.NewMetadata(testStarted), backups.CreateArgs{
			DestinationDir: c.MkDir(),
			Clock:          clock.WallClock,
			FilesToBackUp:  []string{s.writeFile(c, "file", "content")},
			DumpEntries: []backups.DumpEntry{{
				Name:   name,
				Reader: strings.NewReader("data"),
			}},
		})
		c.Check(err, tc.ErrorIs, coreerrors.NotValid,
			tc.Commentf("name %q", name))
		c.Check(err, tc.ErrorMatches, `empty dump entry name ".*": not valid`,
			tc.Commentf("name %q", name))
	}
}

func (s *createSuite) TestCreateMissingDestinationDir(c *tc.C) {
	destDir := filepath.Join(c.MkDir(), "missing")
	_, err := backups.Create(backups.NewMetadata(testStarted), backups.CreateArgs{
		DestinationDir: destDir,
		Clock:          clock.WallClock,
		FilesToBackUp:  []string{s.writeFile(c, "file", "content")},
	})
	c.Assert(err, tc.ErrorMatches,
		`backup destination directory ".*" does not exist`)
}

func (s *createSuite) TestCreateRelativeDestinationDir(c *tc.C) {
	_, err := backups.Create(backups.NewMetadata(testStarted), backups.CreateArgs{
		DestinationDir: "relative",
		Clock:          clock.WallClock,
		FilesToBackUp:  []string{s.writeFile(c, "file", "content")},
	})
	c.Assert(err, tc.ErrorMatches,
		`cannot use relative backup destination directory "relative"`)
}

func (s *createSuite) TestCreateMissingFilesToBackUp(c *tc.C) {
	_, err := backups.Create(backups.NewMetadata(testStarted), backups.CreateArgs{
		DestinationDir: c.MkDir(),
		Clock:          clock.WallClock,
	})
	c.Assert(err, tc.ErrorMatches, "missing list of files to back up")
}

func (s *createSuite) TestCreateMissingClock(c *tc.C) {
	_, err := backups.Create(backups.NewMetadata(testStarted), backups.CreateArgs{
		DestinationDir: c.MkDir(),
		FilesToBackUp:  []string{s.writeFile(c, "file", "content")},
	})
	c.Assert(err, tc.ErrorMatches, "missing clock")
}
