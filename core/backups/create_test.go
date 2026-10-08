// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
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
	filename, err := backups.Create(c.Context(), meta, backups.CreateArgs{
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

	// root.tar archives the files with the leading slash stripped.
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

func (s *createSuite) TestCreatePreservesSymlinks(c *tc.C) {
	toolsDir := c.MkDir()
	agent := filepath.Join(toolsDir, "jujud")
	err := os.WriteFile(agent, []byte("agent binary"), 0755)
	c.Assert(err, tc.ErrorIsNil)

	links := map[string]string{
		"jujud-link":    "jujud",
		"chained-link":  "jujud-link",
		"absolute-link": agent,
		"broken-link":   "missing-jujud",
	}
	files := []string{agent}
	for name, target := range links {
		link := filepath.Join(toolsDir, name)
		c.Assert(os.Symlink(target, link), tc.ErrorIsNil)
		files = append(files, link)
	}

	filename, err := backups.Create(c.Context(), backups.NewMetadata(testStarted), backups.CreateArgs{
		DestinationDir: c.MkDir(),
		Clock:          clock.WallClock,
		FilesToBackUp:  files,
	})
	c.Assert(err, tc.ErrorIsNil)
	archiveFile, err := os.Open(filename)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = archiveFile.Close() }()
	ad, err := backups.NewArchiveDataReader(archiveFile)
	c.Assert(err, tc.ErrorIsNil)
	_, contents := s.archiveEntries(c, ad)

	tr := tar.NewReader(strings.NewReader(contents["juju-backup/root.tar"]))
	header, err := tr.Next()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(header.Typeflag, tc.Equals, byte(tar.TypeReg))
	content, err := io.ReadAll(tr)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(content), tc.Equals, "agent binary")

	seen := set.NewStrings()
	for range links {
		header, err := tr.Next()
		c.Assert(err, tc.ErrorIsNil)
		name := filepath.Base(header.Name)
		c.Check(header.Name, tc.Equals,
			strings.TrimPrefix(filepath.Join(toolsDir, name), "/"))
		c.Check(header.Typeflag, tc.Equals, byte(tar.TypeSymlink))
		c.Check(header.Linkname, tc.Equals, links[name])
		c.Check(header.Size, tc.Equals, int64(0))
		content, err := io.ReadAll(tr)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(content, tc.HasLen, 0)
		seen.Add(name)
	}
	c.Check(seen, tc.DeepEquals,
		set.NewStrings("jujud-link", "chained-link", "absolute-link", "broken-link"))
	_, err = tr.Next()
	c.Assert(err, tc.ErrorIs, io.EOF)
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
		_, err := backups.Create(c.Context(), backups.NewMetadata(testStarted), backups.CreateArgs{
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
		_, err := backups.Create(c.Context(), backups.NewMetadata(testStarted), backups.CreateArgs{
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
	_, err := backups.Create(c.Context(), backups.NewMetadata(testStarted), backups.CreateArgs{
		DestinationDir: destDir,
		Clock:          clock.WallClock,
		FilesToBackUp:  []string{s.writeFile(c, "file", "content")},
	})
	c.Assert(err, tc.ErrorMatches,
		`backup destination directory ".*" does not exist`)
}

func (s *createSuite) TestCreateRelativeDestinationDir(c *tc.C) {
	_, err := backups.Create(c.Context(), backups.NewMetadata(testStarted), backups.CreateArgs{
		DestinationDir: "relative",
		Clock:          clock.WallClock,
		FilesToBackUp:  []string{s.writeFile(c, "file", "content")},
	})
	c.Assert(err, tc.ErrorMatches,
		`cannot use relative backup destination directory "relative"`)
}

func (s *createSuite) TestCreateMissingFilesToBackUp(c *tc.C) {
	_, err := backups.Create(c.Context(), backups.NewMetadata(testStarted), backups.CreateArgs{
		DestinationDir: c.MkDir(),
		Clock:          clock.WallClock,
	})
	c.Assert(err, tc.ErrorMatches, "missing list of files to back up")
}

func (s *createSuite) TestCreateMissingClock(c *tc.C) {
	_, err := backups.Create(c.Context(), backups.NewMetadata(testStarted), backups.CreateArgs{
		DestinationDir: c.MkDir(),
		FilesToBackUp:  []string{s.writeFile(c, "file", "content")},
	})
	c.Assert(err, tc.ErrorMatches, "missing clock")
}

func (s *createSuite) TestBuildArchiveAndChecksumFailureRemovesArchive(c *tc.C) {
	dir := c.MkDir()
	filename := filepath.Join(dir, "archive.tar.gz")

	// A missing content directory fails the tar build after the
	// archive file has been created; the partial archive must not be
	// left behind.
	_, _, err := backups.BuildArchiveAndChecksum(filename, dir,
		filepath.Join(dir, "missing"))
	c.Assert(err, tc.NotNil)

	_, err = os.Stat(filename)
	c.Assert(err, tc.ErrorIs, os.ErrNotExist)
}

// TestCreateMetadataFailureRemovesArchive verifies that a failure after
// the archive has been written removes it: Create reporting an error
// must not leave a stray archive in the destination directory for
// listing and download to find.
func (s *createSuite) TestCreateMetadataFailureRemovesArchive(c *tc.C) {
	destDir := c.MkDir()
	file := s.writeFile(c, "jujud", "agent binary")

	// Metadata that already carries file info fails MarkComplete, which
	// runs only once the archive file is complete.
	meta := backups.NewMetadata(testStarted)
	c.Assert(meta.SetFileInfo(99, "not-the-real-checksum",
		"SHA-1, base64 encoded"), tc.ErrorIsNil)

	_, err := backups.Create(c.Context(), meta, backups.CreateArgs{
		DestinationDir: destDir,
		Clock:          clock.WallClock,
		FilesToBackUp:  []string{file},
		DumpEntries: []backups.DumpEntry{{
			Name:   "controller.yaml",
			Reader: strings.NewReader("controller: data"),
		}},
	})
	c.Assert(err, tc.ErrorMatches, "updating metadata: .*")

	// Neither the archive nor a staging directory is left behind.
	entries, rerr := os.ReadDir(destDir)
	c.Assert(rerr, tc.ErrorIsNil)
	var left []string
	for _, entry := range entries {
		left = append(left, entry.Name())
	}
	c.Check(left, tc.HasLen, 0, tc.Commentf("left behind: %v", left))
}

func (s *createSuite) TestCreateWithObjectEntries(c *tc.C) {
	destDir := c.MkDir()
	file1 := s.writeFile(c, "jujud", "agent binary")

	content := "charm blob content"
	sha256sum := sha256.Sum256([]byte(content))
	sha384sum := sha512.Sum384([]byte(content))
	sha256hex := hex.EncodeToString(sha256sum[:])
	sha384hex := hex.EncodeToString(sha384sum[:])

	var sourceCalls int
	meta := backups.NewMetadata(testStarted)
	filename, err := backups.Create(c.Context(), meta, backups.CreateArgs{
		DestinationDir: destDir,
		DataDir:        "/var/lib/juju",
		Clock:          clock.WallClock,
		FilesToBackUp:  []string{file1},
		ObjectEntries: []backups.ObjectEntry{{
			Namespace: "controller",
			SHA256:    sha256hex,
			SHA384:    sha384hex,
			Size:      int64(len(content)),
			Source: func(ctx context.Context) (io.ReadCloser, error) {
				sourceCalls++
				return io.NopCloser(strings.NewReader(content)), nil
			},
		}, {
			// Identical content in a different namespace is a
			// separate ordinary file in the archive.
			Namespace: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			SHA256:    sha256hex,
			SHA384:    sha384hex,
			Size:      int64(len(content)),
			Source: func(ctx context.Context) (io.ReadCloser, error) {
				sourceCalls++
				return io.NopCloser(strings.NewReader(content)), nil
			},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sourceCalls, tc.Equals, 2)

	archiveFile, err := os.Open(filename)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = archiveFile.Close() }()

	ad, err := backups.NewArchiveDataReader(archiveFile)
	c.Assert(err, tc.ErrorIsNil)

	_, contents := s.archiveEntries(c, ad)
	rootEntries, rootContents := s.tarEntries(c,
		strings.NewReader(contents["juju-backup/root.tar"]))

	// The objects land at the source data directory's object store
	// layout, one regular file per namespace.
	controllerPath := "var/lib/juju/objectstore/controller/" + sha384hex
	modelPath := "var/lib/juju/objectstore/deadbeef-0bad-400d-8000-4b1d0d06f00d/" + sha384hex
	c.Check(rootEntries.Contains(controllerPath), tc.IsTrue)
	c.Check(rootEntries.Contains(modelPath), tc.IsTrue)
	c.Check(rootContents[controllerPath], tc.Equals, content)
	c.Check(rootContents[modelPath], tc.Equals, content)
}

func (s *createSuite) TestCreateObjectEntryHashMismatch(c *tc.C) {
	destDir := c.MkDir()
	file1 := s.writeFile(c, "jujud", "agent binary")

	_, err := backups.Create(c.Context(), backups.NewMetadata(testStarted), backups.CreateArgs{
		DestinationDir: destDir,
		DataDir:        "/var/lib/juju",
		Clock:          clock.WallClock,
		FilesToBackUp:  []string{file1},
		ObjectEntries: []backups.ObjectEntry{{
			Namespace: "controller",
			SHA256:    "0ba904eae8773b70c75333db4de2f3ac45a8ad4ddba1b242f0b3cfc199391dd8",
			SHA384:    "3b1898395c3b4a3ffd8b4ab7b1b6c3e0c2a47600f0c6b7c2f5f2b6f5a3d9b5b1e6b1e0c7f4d9d7a1c8e5f3a2b1c0d9e8",
			Size:      7,
			Source: func(ctx context.Context) (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader("corrupt")), nil
			},
		}},
	})
	c.Assert(err, tc.ErrorMatches, `object ".*" in namespace "controller": SHA-384 mismatch.*`)

	// A failed archive build leaves no partial archive behind.
	entries, err := os.ReadDir(destDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.HasLen, 0)
}

func (s *createSuite) TestCreateObjectEntrySizeMismatch(c *tc.C) {
	destDir := c.MkDir()
	file1 := s.writeFile(c, "jujud", "agent binary")

	content := "short"
	sha256sum := sha256.Sum256([]byte(content))
	sha384sum := sha512.Sum384([]byte(content))

	_, err := backups.Create(c.Context(), backups.NewMetadata(testStarted), backups.CreateArgs{
		DestinationDir: destDir,
		DataDir:        "/var/lib/juju",
		Clock:          clock.WallClock,
		FilesToBackUp:  []string{file1},
		ObjectEntries: []backups.ObjectEntry{{
			Namespace: "controller",
			SHA256:    hex.EncodeToString(sha256sum[:]),
			SHA384:    hex.EncodeToString(sha384sum[:]),
			Size:      int64(len(content)) + 10,
			Source: func(ctx context.Context) (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader(content)), nil
			},
		}},
	})
	c.Assert(err, tc.ErrorMatches, `object ".*" in namespace "controller": streamed 5 bytes, expected 15`)

	entries, err := os.ReadDir(destDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.HasLen, 0)
}

func (s *createSuite) TestCreateObjectEntrySourceError(c *tc.C) {
	destDir := c.MkDir()
	file1 := s.writeFile(c, "jujud", "agent binary")

	_, err := backups.Create(c.Context(), backups.NewMetadata(testStarted), backups.CreateArgs{
		DestinationDir: destDir,
		DataDir:        "/var/lib/juju",
		Clock:          clock.WallClock,
		FilesToBackUp:  []string{file1},
		ObjectEntries: []backups.ObjectEntry{{
			Namespace: "controller",
			SHA256:    "0ba904eae8773b70c75333db4de2f3ac45a8ad4ddba1b242f0b3cfc199391dd8",
			SHA384:    "3b1898395c3b4a3ffd8b4ab7b1b6c3e0c2a47600f0c6b7c2f5f2b6f5a3d9b5b1e6b1e0c7f4d9d7a1c8e5f3a2b1c0d9e8",
			Size:      7,
			Source: func(ctx context.Context) (io.ReadCloser, error) {
				return nil, errors.New("read failure")
			},
		}},
	})
	c.Assert(err, tc.ErrorMatches, `opening object ".*" in namespace "controller": read failure`)

	entries, err := os.ReadDir(destDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.HasLen, 0)
}

func (s *createSuite) TestCreateDuplicateObjectEntry(c *tc.C) {
	_, err := backups.Create(c.Context(), backups.NewMetadata(testStarted), backups.CreateArgs{
		DestinationDir: c.MkDir(),
		DataDir:        "/var/lib/juju",
		Clock:          clock.WallClock,
		FilesToBackUp:  []string{s.writeFile(c, "file", "content")},
		ObjectEntries: []backups.ObjectEntry{{
			Namespace: "controller",
			SHA384:    "abc",
			Source:    func(ctx context.Context) (io.ReadCloser, error) { return nil, nil },
		}, {
			Namespace: "controller",
			SHA384:    "abc",
			Source:    func(ctx context.Context) (io.ReadCloser, error) { return nil, nil },
		}},
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *createSuite) TestCreateObjectEntryMissingDataDir(c *tc.C) {
	_, err := backups.Create(c.Context(), backups.NewMetadata(testStarted), backups.CreateArgs{
		DestinationDir: c.MkDir(),
		Clock:          clock.WallClock,
		FilesToBackUp:  []string{s.writeFile(c, "file", "content")},
		ObjectEntries: []backups.ObjectEntry{{
			Namespace: "controller",
			SHA384:    "abc",
			Source:    func(ctx context.Context) (io.ReadCloser, error) { return nil, nil },
		}},
	})
	c.Assert(err, tc.ErrorMatches, "missing data directory for object entries")
}
