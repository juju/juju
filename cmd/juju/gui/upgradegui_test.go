// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gui_test

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/gui"
	jujutesting "github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
)

type upgradeGUISuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&upgradeGUISuite{})

// run executes the upgrade-gui command passing the given args.
func (s *upgradeGUISuite) run(c *gc.C, args ...string) (string, error) {
	ctx, err := coretesting.RunCommand(c, gui.NewUpgradeGUICommand(), args...)
	return strings.Trim(coretesting.Stderr(ctx), "\n"), err
}

// calledFunc is returned by the patch* methods below, and when called reports
// whether the corresponding patched function has been called.
type calledFunc func() bool

func (s *upgradeGUISuite) patchClientGUIArchives(c *gc.C, returnedVersions []params.GUIArchiveVersion, returnedErr error) calledFunc {
	var called bool
	f := func(client *api.Client) ([]params.GUIArchiveVersion, error) {
		called = true
		return returnedVersions, returnedErr
	}
	s.PatchValue(gui.ClientGUIArchives, f)
	return func() bool {
		return called
	}
}

func (s *upgradeGUISuite) patchClientSelectGUIVersion(c *gc.C, expectedVers string, returnedErr error) calledFunc {
	var called bool
	f := func(client *api.Client, vers version.Number) error {
		called = true
		c.Assert(vers.String(), gc.Equals, expectedVers)
		return returnedErr
	}
	s.PatchValue(gui.ClientSelectGUIVersion, f)
	return func() bool {
		return called
	}
}

func (s *upgradeGUISuite) patchClientUploadGUIArchive(c *gc.C, expectedHash string, expectedSize int64, expectedVers string, returnedIsCurrent bool, returnedErr error) calledFunc {
	var called bool
	f := func(client *api.Client, r io.ReadSeeker, hash string, size int64, vers version.Number) (bool, error) {
		called = true
		c.Assert(hash, gc.Equals, expectedHash)
		c.Assert(size, gc.Equals, expectedSize)
		c.Assert(vers.String(), gc.Equals, expectedVers)
		return returnedIsCurrent, returnedErr
	}
	s.PatchValue(gui.ClientUploadGUIArchive, f)
	return func() bool {
		return called
	}
}

var upgradeGUIInputErrorsTests = []struct {
	about         string
	args          []string
	expectedError string
}{{
	about:         "too many arguments",
	args:          []string{"bad", "wolf"},
	expectedError: `unrecognized args: \["bad" "wolf"\]`,
}, {
	// TODO frankban: remove this case when we have GUI from simplestreams.
	about:         "upgrading to latest simplestreams version not implemented",
	expectedError: "upgrading to latest released version not implemented",
}, {
	// TODO frankban: remove this case when we have GUI from simplestreams.
	about:         "upgrading to simplestreams version not implemented",
	args:          []string{"2.1.41"},
	expectedError: `upgrading to a released version \(2.1.41\) not implemented`,
}, {
	about:         "archive path not found",
	args:          []string{"no-such-file"},
	expectedError: `invalid GUI release version or local path "no-such-file"`,
}}

func (s *upgradeGUISuite) TestUpgradeGUIInputErrors(c *gc.C) {
	for i, test := range upgradeGUIInputErrorsTests {
		c.Logf("\n%d: %s", i, test.about)
		_, err := s.run(c, test.args...)
		c.Assert(err, gc.ErrorMatches, test.expectedError)
	}
}

func (s *upgradeGUISuite) TestUpgradeGUIFileError(c *gc.C) {
	path, _, _ := saveGUIArchive(c, "2.0.0")
	err := os.Chmod(path, 0000)
	c.Assert(err, jc.ErrorIsNil)
	defer os.Chmod(path, 0600)
	out, err := s.run(c, path)
	c.Assert(err, gc.ErrorMatches, "cannot open GUI archive: .*")
	c.Assert(out, gc.Equals, "")
}

func (s *upgradeGUISuite) TestUpgradeGUIArchiveVersionNotValid(c *gc.C) {
	path, _, _ := saveGUIArchive(c, "bad-wolf")
	out, err := s.run(c, path)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade Juju GUI using ".*": invalid version "bad-wolf" in archive`)
	c.Assert(out, gc.Equals, "")
}

func (s *upgradeGUISuite) TestUpgradeGUIArchiveVersionNotFound(c *gc.C) {
	path, _, _ := saveGUIArchive(c, "")
	out, err := s.run(c, path)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade Juju GUI using ".*": cannot find Juju GUI version in archive`)
	c.Assert(out, gc.Equals, "")
}

func (s *upgradeGUISuite) TestUpgradeGUIGUIArchivesError(c *gc.C) {
	path, _, _ := saveGUIArchive(c, "2.1.0")
	s.patchClientGUIArchives(c, nil, errors.New("bad wolf"))
	out, err := s.run(c, path)
	c.Assert(err, gc.ErrorMatches, "cannot retrieve GUI versions from the controller: bad wolf")
	c.Assert(out, gc.Equals, "")
}

func (s *upgradeGUISuite) TestUpgradeGUIUploadGUIArchiveError(c *gc.C) {
	path, hash, size := saveGUIArchive(c, "2.2.0")
	s.patchClientGUIArchives(c, nil, nil)
	s.patchClientUploadGUIArchive(c, hash, size, "2.2.0", false, errors.New("bad wolf"))
	out, err := s.run(c, path)
	c.Assert(err, gc.ErrorMatches, "cannot upload Juju GUI: bad wolf")
	c.Assert(out, gc.Equals, "uploading Juju GUI 2.2.0")
}

func (s *upgradeGUISuite) TestUpgradeGUISelectGUIVersionError(c *gc.C) {
	path, hash, size := saveGUIArchive(c, "2.3.0")
	s.patchClientGUIArchives(c, nil, nil)
	s.patchClientUploadGUIArchive(c, hash, size, "2.3.0", false, nil)
	s.patchClientSelectGUIVersion(c, "2.3.0", errors.New("bad wolf"))
	out, err := s.run(c, path)
	c.Assert(err, gc.ErrorMatches, "cannot switch to new Juju GUI version: bad wolf")
	c.Assert(out, gc.Equals, "uploading Juju GUI 2.3.0\nupload completed")
}

var upgradeGUISuccessTests = []struct {
	// about describes the test.
	about string
	// archiveVersion is the version of the archive to be uploaded.
	archiveVersion string
	// existingVersions is a function returning a list of GUI archive versions
	// already included in the controller.
	existingVersions func(hash string) []params.GUIArchiveVersion
	// uploaded holds whether the archive has been actually uploaded. If an
	// archive with the same hash and version is already present in the
	// controller, the upload is not performed again.
	uploaded bool
	// selected holds whether a new GUI version must be selected. If the upload
	// upgraded the currently served version there is no need to perform
	// the API call to switch GUI version.
	selected bool
	// expectedOutput holds the expected upgrade-gui command output.
	expectedOutput string
}{{
	about:          "archive: first archive",
	archiveVersion: "2.0.0",
	expectedOutput: "uploading Juju GUI 2.0.0\nupload completed\nJuju GUI switched to version 2.0.0",
	uploaded:       true,
	selected:       true,
}, {
	about:          "archive: new archive",
	archiveVersion: "2.1.0",
	existingVersions: func(hash string) []params.GUIArchiveVersion {
		return []params.GUIArchiveVersion{{
			Version: version.MustParse("1.0.0"),
			SHA256:  "hash-1",
			Current: true,
		}}
	},
	uploaded:       true,
	selected:       true,
	expectedOutput: "uploading Juju GUI 2.1.0\nupload completed\nJuju GUI switched to version 2.1.0",
}, {
	about:          "archive: new archive, existing non-current version",
	archiveVersion: "2.0.42",
	existingVersions: func(hash string) []params.GUIArchiveVersion {
		return []params.GUIArchiveVersion{{
			Version: version.MustParse("2.0.42"),
			SHA256:  "hash-42",
			Current: false,
		}, {
			Version: version.MustParse("2.0.47"),
			SHA256:  "hash-47",
			Current: true,
		}}
	},
	uploaded:       true,
	selected:       true,
	expectedOutput: "uploading Juju GUI 2.0.42\nupload completed\nJuju GUI switched to version 2.0.42",
}, {
	about:          "archive: new archive, existing current version",
	archiveVersion: "2.0.47",
	existingVersions: func(hash string) []params.GUIArchiveVersion {
		return []params.GUIArchiveVersion{{
			Version: version.MustParse("2.0.47"),
			SHA256:  "hash-47",
			Current: true,
		}}
	},
	uploaded:       true,
	expectedOutput: "uploading Juju GUI 2.0.47\nupload completed\nJuju GUI at version 2.0.47",
}, {
	about:          "archive: existing archive, existing non-current version",
	archiveVersion: "2.0.42",
	existingVersions: func(hash string) []params.GUIArchiveVersion {
		return []params.GUIArchiveVersion{{
			Version: version.MustParse("2.0.42"),
			SHA256:  hash,
			Current: false,
		}, {
			Version: version.MustParse("2.0.47"),
			SHA256:  "hash-47",
			Current: true,
		}}
	},
	selected:       true,
	expectedOutput: "Juju GUI switched to version 2.0.42",
}, {
	about:          "archive: existing archive, existing current version",
	archiveVersion: "1.47.0",
	existingVersions: func(hash string) []params.GUIArchiveVersion {
		return []params.GUIArchiveVersion{{
			Version: version.MustParse("1.47.0"),
			SHA256:  hash,
			Current: true,
		}}
	},
	expectedOutput: "Juju GUI at version 1.47.0",
}, {
	about:          "archive: existing archive, different existing version",
	archiveVersion: "2.0.42",
	existingVersions: func(hash string) []params.GUIArchiveVersion {
		return []params.GUIArchiveVersion{{
			Version: version.MustParse("2.0.42"),
			SHA256:  "hash-42",
			Current: false,
		}, {
			Version: version.MustParse("2.0.47"),
			SHA256:  hash,
			Current: true,
		}}
	},
	uploaded:       true,
	selected:       true,
	expectedOutput: "uploading Juju GUI 2.0.42\nupload completed\nJuju GUI switched to version 2.0.42",
	// TODO frankban: add simplestreams cases when the feature is implemented.
}}

func (s *upgradeGUISuite) TestUpgradeGUISuccess(c *gc.C) {
	for i, test := range upgradeGUISuccessTests {
		c.Logf("\n%d: %s", i, test.about)

		// Create an fake Juju GUI archive.
		path, hash, size := saveGUIArchive(c, test.archiveVersion)

		// Patch the call to get existing archive versions.
		var existingVersions []params.GUIArchiveVersion
		if test.existingVersions != nil {
			existingVersions = test.existingVersions(hash)
		}
		guiArchivesCalled := s.patchClientGUIArchives(c, existingVersions, nil)

		// Patch the other calls to the controller.
		uploadGUIArchiveCalled := s.patchClientUploadGUIArchive(c, hash, size, test.archiveVersion, !test.selected, nil)
		selectGUIVersionCalled := s.patchClientSelectGUIVersion(c, test.archiveVersion, nil)

		// Run the command.
		out, err := s.run(c, path)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(out, gc.Equals, test.expectedOutput)
		c.Assert(guiArchivesCalled(), jc.IsTrue)
		c.Assert(uploadGUIArchiveCalled(), gc.Equals, test.uploaded)
		c.Assert(selectGUIVersionCalled(), gc.Equals, test.selected)
	}
}

func (s *upgradeGUISuite) TestUpgradeGUIIntegration(c *gc.C) {
	// Prepare a GUI archive.
	path, hash, size := saveGUIArchive(c, "2.42.0")

	// Upload the archive from command line.
	out, err := s.run(c, path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Equals, "uploading Juju GUI 2.42.0\nupload completed\nJuju GUI switched to version 2.42.0")

	// Check that the archive is present in the GUI storage server side.
	storage, err := s.State.GUIStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	metadata, err := storage.Metadata("2.42.0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata.SHA256, gc.Equals, hash)
	c.Assert(metadata.Size, gc.Equals, size)

	// Check that the uploaded version has been set as the current one.
	vers, err := s.State.GUIVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vers.String(), gc.Equals, "2.42.0")
}

// makeGUIArchive creates a Juju GUI tar.bz2 archive in memory, and returns a
// reader for the archive, its SHA256 hash and size.
func makeGUIArchive(c *gc.C, vers string) (r io.Reader, hash string, size int64) {
	if runtime.GOOS == "windows" {
		c.Skip("bzip2 command not available")
	}
	cmd := exec.Command("bzip2", "--compress", "--stdout", "--fast")

	stdin, err := cmd.StdinPipe()
	c.Assert(err, jc.ErrorIsNil)
	stdout, err := cmd.StdoutPipe()
	c.Assert(err, jc.ErrorIsNil)

	err = cmd.Start()
	c.Assert(err, jc.ErrorIsNil)

	tw := tar.NewWriter(stdin)
	if vers != "" {
		err = tw.WriteHeader(&tar.Header{
			Name:     filepath.Join("jujugui-"+vers, "jujugui"),
			Mode:     0700,
			Typeflag: tar.TypeDir,
		})
		c.Assert(err, jc.ErrorIsNil)
	}
	err = tw.Close()
	c.Assert(err, jc.ErrorIsNil)
	err = stdin.Close()
	c.Assert(err, jc.ErrorIsNil)

	h := sha256.New()
	r = io.TeeReader(stdout, h)
	b, err := ioutil.ReadAll(r)
	c.Assert(err, jc.ErrorIsNil)

	err = cmd.Wait()
	c.Assert(err, jc.ErrorIsNil)

	return bytes.NewReader(b), fmt.Sprintf("%x", h.Sum(nil)), int64(len(b))
}

// saveGUIArchive creates a Juju GUI tar.bz2 archive with the given version on
// disk, and return its path, SHA256 hash and size.
func saveGUIArchive(c *gc.C, vers string) (path, hash string, size int64) {
	r, hash, size := makeGUIArchive(c, vers)
	path = filepath.Join(c.MkDir(), "gui.tar.bz2")
	data, err := ioutil.ReadAll(r)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(path, data, 0600)
	c.Assert(err, jc.ErrorIsNil)
	return path, hash, size
}
