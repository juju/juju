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

	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/gui"
	envgui "github.com/juju/juju/environs/gui"
	"github.com/juju/juju/environs/simplestreams"
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
	f := func(client *controller.Client) ([]params.GUIArchiveVersion, error) {
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
	f := func(client *controller.Client, vers version.Number) error {
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
	f := func(client *controller.Client, r io.ReadSeeker, hash string, size int64, vers version.Number) (bool, error) {
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

func (s *upgradeGUISuite) patchGUIFetchMetadata(c *gc.C, returnedMetadata []*envgui.Metadata, returnedErr error) calledFunc {
	var called bool
	f := func(stream string, sources ...simplestreams.DataSource) ([]*envgui.Metadata, error) {
		called = true
		c.Assert(stream, gc.Equals, envgui.ReleasedStream)
		c.Assert(sources[0].Description(), gc.Equals, "gui simplestreams")
		return returnedMetadata, returnedErr
	}
	s.PatchValue(gui.GUIFetchMetadata, f)
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
	about:         "listing and upgrading",
	args:          []string{"bad", "--list"},
	expectedError: "cannot provide arguments if --list is provided",
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

func (s *upgradeGUISuite) TestUpgradeGUIListSuccess(c *gc.C) {
	s.patchGUIFetchMetadata(c, []*envgui.Metadata{{
		Version: version.MustParse("2.2.0"),
	}, {
		Version: version.MustParse("2.1.1"),
	}, {
		Version: version.MustParse("2.1.0"),
	}}, nil)
	uploadCalled := s.patchClientUploadGUIArchive(c, "", 0, "", false, nil)
	selectCalled := s.patchClientSelectGUIVersion(c, "", nil)

	// Run the command to list available Juju GUI archive versions.
	out, err := s.run(c, "--list")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Equals, "2.2.0\n2.1.1\n2.1.0")

	// No uploads or switches are preformed.
	c.Assert(uploadCalled(), jc.IsFalse)
	c.Assert(selectCalled(), jc.IsFalse)
}

func (s *upgradeGUISuite) TestUpgradeGUIListNoReleases(c *gc.C) {
	s.patchGUIFetchMetadata(c, nil, nil)
	out, err := s.run(c, "--list")
	c.Assert(err, gc.ErrorMatches, "cannot list Juju GUI release versions: no available Juju GUI archives found")
	c.Assert(out, gc.Equals, "")
}

func (s *upgradeGUISuite) TestUpgradeGUIListError(c *gc.C) {
	s.patchGUIFetchMetadata(c, nil, errors.New("bad wolf"))
	out, err := s.run(c, "--list")
	c.Assert(err, gc.ErrorMatches, "cannot list Juju GUI release versions: cannot retrieve Juju GUI archive info: bad wolf")
	c.Assert(out, gc.Equals, "")
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
	c.Assert(out, gc.Equals, "using local Juju GUI archive\nuploading Juju GUI 2.2.0")
}

func (s *upgradeGUISuite) TestUpgradeGUISelectGUIVersionError(c *gc.C) {
	path, hash, size := saveGUIArchive(c, "2.3.0")
	s.patchClientGUIArchives(c, nil, nil)
	s.patchClientUploadGUIArchive(c, hash, size, "2.3.0", false, nil)
	s.patchClientSelectGUIVersion(c, "2.3.0", errors.New("bad wolf"))
	out, err := s.run(c, path)
	c.Assert(err, gc.ErrorMatches, "cannot switch to new Juju GUI version: bad wolf")
	c.Assert(out, gc.Equals, "using local Juju GUI archive\nuploading Juju GUI 2.3.0\nupload completed")
}

func (s *upgradeGUISuite) TestUpgradeGUIFromSimplestreamsReleaseErrors(c *gc.C) {
	tests := []struct {
		about            string
		arg              string
		returnedMetadata []*envgui.Metadata
		returnedErr      error
		expectedErr      string
	}{{
		about:       "last release: no releases found",
		expectedErr: "cannot upgrade to most recent release: no available Juju GUI archives found",
	}, {
		about:       "specific release: no releases found",
		arg:         "2.0.42",
		expectedErr: "cannot upgrade to release 2.0.42: no available Juju GUI archives found",
	}, {
		about:       "last release: error while fetching releases list",
		returnedErr: errors.New("bad wolf"),
		expectedErr: "cannot upgrade to most recent release: cannot retrieve Juju GUI archive info: bad wolf",
	}, {
		about:       "specific release: error while fetching releases list",
		arg:         "2.0.47",
		returnedErr: errors.New("bad wolf"),
		expectedErr: "cannot upgrade to release 2.0.47: cannot retrieve Juju GUI archive info: bad wolf",
	}, {
		about: "last release: error while opening the remote release resource",
		returnedMetadata: []*envgui.Metadata{
			makeGUIMetadata(c, "2.2.0", "exterminate"),
			makeGUIMetadata(c, "2.1.0", ""),
		},
		expectedErr: `cannot open Juju GUI archive at "https://1.2.3.4/path/to/gui/2.2.0": exterminate`,
	}, {
		about: "specific release: error while opening the remote release resource",
		arg:   "2.1.0",
		returnedMetadata: []*envgui.Metadata{
			makeGUIMetadata(c, "2.2.0", ""),
			makeGUIMetadata(c, "2.1.0", "boo"),
			makeGUIMetadata(c, "2.0.0", ""),
		},
		expectedErr: `cannot open Juju GUI archive at "https://1.2.3.4/path/to/gui/2.1.0": boo`,
	}, {
		about: "specific release: not found in available releases",
		arg:   "2.1.0",
		returnedMetadata: []*envgui.Metadata{
			makeGUIMetadata(c, "2.2.0", ""),
			makeGUIMetadata(c, "2.0.0", ""),
		},
		expectedErr: "Juju GUI release version 2.1.0 not found",
	}}

	for i, test := range tests {
		c.Logf("\n%d: %s", i, test.about)

		s.patchGUIFetchMetadata(c, test.returnedMetadata, test.returnedErr)
		out, err := s.run(c, test.arg)
		c.Assert(err, gc.ErrorMatches, test.expectedErr)
		c.Assert(out, gc.Equals, "")
	}
}

func (s *upgradeGUISuite) TestUpgradeGUISuccess(c *gc.C) {
	tests := []struct {
		// about describes the test.
		about string
		// returnedMetadata holds metadata information returned by simplestreams.
		returnedMetadata *envgui.Metadata
		// archiveVersion is the version of the archive to be uploaded.
		archiveVersion string
		// existingVersions is a function returning a list of GUI archive versions
		// already included in the controller.
		existingVersions func(hash string) []params.GUIArchiveVersion
		// opened holds whether Juju GUI metadata information in simplestreams
		// has been opened.
		opened bool
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
		expectedOutput: "using local Juju GUI archive\nuploading Juju GUI 2.0.0\nupload completed\nJuju GUI switched to version 2.0.0",
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
		expectedOutput: "using local Juju GUI archive\nuploading Juju GUI 2.1.0\nupload completed\nJuju GUI switched to version 2.1.0",
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
		expectedOutput: "using local Juju GUI archive\nuploading Juju GUI 2.0.42\nupload completed\nJuju GUI switched to version 2.0.42",
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
		expectedOutput: "using local Juju GUI archive\nuploading Juju GUI 2.0.47\nupload completed\nJuju GUI at version 2.0.47",
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
		expectedOutput: "using local Juju GUI archive\nuploading Juju GUI 2.0.42\nupload completed\nJuju GUI switched to version 2.0.42",
	}, {
		about:            "stream: first archive",
		archiveVersion:   "2.0.0",
		returnedMetadata: makeGUIMetadata(c, "2.0.0", ""),
		expectedOutput:   "fetching Juju GUI archive\nuploading Juju GUI 2.0.0\nupload completed\nJuju GUI switched to version 2.0.0",
		opened:           true,
		uploaded:         true,
		selected:         true,
	}, {
		about:            "stream: new archive",
		archiveVersion:   "2.1.0",
		returnedMetadata: makeGUIMetadata(c, "2.1.0", ""),
		existingVersions: func(hash string) []params.GUIArchiveVersion {
			return []params.GUIArchiveVersion{{
				Version: version.MustParse("1.0.0"),
				SHA256:  "hash-1",
				Current: true,
			}}
		},
		opened:         true,
		uploaded:       true,
		selected:       true,
		expectedOutput: "fetching Juju GUI archive\nuploading Juju GUI 2.1.0\nupload completed\nJuju GUI switched to version 2.1.0",
	}, {
		about:            "stream: new archive, existing non-current version",
		archiveVersion:   "2.0.42",
		returnedMetadata: makeGUIMetadata(c, "2.0.42", ""),
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
		opened:         true,
		uploaded:       true,
		selected:       true,
		expectedOutput: "fetching Juju GUI archive\nuploading Juju GUI 2.0.42\nupload completed\nJuju GUI switched to version 2.0.42",
	}, {
		about:            "stream: new archive, existing current version",
		archiveVersion:   "2.0.47",
		returnedMetadata: makeGUIMetadata(c, "2.0.47", ""),
		existingVersions: func(hash string) []params.GUIArchiveVersion {
			return []params.GUIArchiveVersion{{
				Version: version.MustParse("2.0.47"),
				SHA256:  "hash-47",
				Current: true,
			}}
		},
		opened:         true,
		uploaded:       true,
		expectedOutput: "fetching Juju GUI archive\nuploading Juju GUI 2.0.47\nupload completed\nJuju GUI at version 2.0.47",
	}, {
		about:            "stream: existing archive, existing non-current version",
		archiveVersion:   "2.0.42",
		returnedMetadata: makeGUIMetadata(c, "2.0.42", ""),
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
		opened:         true,
		selected:       true,
		expectedOutput: "Juju GUI switched to version 2.0.42",
	}, {
		about:            "stream: existing archive, existing current version",
		archiveVersion:   "1.47.0",
		returnedMetadata: makeGUIMetadata(c, "1.47.0", ""),
		existingVersions: func(hash string) []params.GUIArchiveVersion {
			return []params.GUIArchiveVersion{{
				Version: version.MustParse("1.47.0"),
				SHA256:  hash,
				Current: true,
			}}
		},
		opened:         true,
		expectedOutput: "Juju GUI at version 1.47.0",
	}, {
		about:            "stream: existing archive, different existing version",
		archiveVersion:   "2.0.42",
		returnedMetadata: makeGUIMetadata(c, "2.0.42", ""),
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
		opened:         true,
		uploaded:       true,
		selected:       true,
		expectedOutput: "fetching Juju GUI archive\nuploading Juju GUI 2.0.42\nupload completed\nJuju GUI switched to version 2.0.42",
	}}

	for i, test := range tests {
		c.Logf("\n%d: %s", i, test.about)

		var arg string
		var hash string
		var size int64

		if test.returnedMetadata == nil {
			// Create an fake Juju GUI local archive.
			arg, hash, size = saveGUIArchive(c, test.archiveVersion)
		} else {
			// Use the remote metadata information.
			arg = test.returnedMetadata.Version.String()
			hash = test.returnedMetadata.SHA256
			size = test.returnedMetadata.Size
		}

		// Patch the call to get simplestreams metadata information.
		fetchMetadataCalled := s.patchGUIFetchMetadata(c, []*envgui.Metadata{test.returnedMetadata}, nil)

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
		out, err := s.run(c, arg)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(out, gc.Equals, test.expectedOutput)
		c.Assert(guiArchivesCalled(), jc.IsTrue)
		c.Assert(fetchMetadataCalled(), gc.Equals, test.opened)
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
	c.Assert(out, gc.Equals, "using local Juju GUI archive\nuploading Juju GUI 2.42.0\nupload completed\nJuju GUI switched to version 2.42.0")

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

// makeGUIMetadata creates and return a Juju GUI archive metadata with the
// given version. If fetchError is not empty, trying to fetch the corresponding
// archive will return the given error.
func makeGUIMetadata(c *gc.C, vers, fetchError string) *envgui.Metadata {
	path, hash, size := saveGUIArchive(c, vers)
	metaPath := "/path/to/gui/" + vers
	return &envgui.Metadata{
		Version:  version.MustParse(vers),
		SHA256:   hash,
		Size:     size,
		Path:     metaPath,
		FullPath: "https://1.2.3.4" + metaPath,
		Source: &dataSource{
			DataSource: envgui.NewDataSource("htpps://1.2.3.4"),
			metaPath:   metaPath,
			path:       path,
			fetchError: fetchError,
			c:          c,
		},
	}
}

// datasource implements simplestreams.DataSource and overrides the Fetch
// method for testing purposes.
type dataSource struct {
	simplestreams.DataSource

	metaPath   string
	path       string
	fetchError string
	c          *gc.C
}

// Fetch implements simplestreams.DataSource.
func (ds *dataSource) Fetch(path string) (io.ReadCloser, string, error) {
	ds.c.Assert(path, gc.Equals, ds.metaPath)
	if ds.fetchError != "" {
		return nil, "", errors.New(ds.fetchError)
	}
	f, err := os.Open(ds.path)
	ds.c.Assert(err, jc.ErrorIsNil)
	return f, "", nil
}
