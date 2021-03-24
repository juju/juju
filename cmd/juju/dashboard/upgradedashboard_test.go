// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dashboard_test

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

	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/dashboard"
	envdashboard "github.com/juju/juju/environs/dashboard"
	"github.com/juju/juju/environs/simplestreams"
	jujutesting "github.com/juju/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type upgradeDashboardSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&upgradeDashboardSuite{})

// run executes the upgrade-dashboard command passing the given args.
func (s *upgradeDashboardSuite) run(c *gc.C, args ...string) (string, error) {
	ctx, err := cmdtesting.RunCommand(c, dashboard.NewUpgradeDashboardCommand(), args...)
	return strings.Trim(cmdtesting.Stderr(ctx), "\n"), err
}

// calledFunc is returned by the patch* methods below, and when called reports
// whether the corresponding patched function has been called.
type calledFunc func() bool

func (s *upgradeDashboardSuite) patchClientDashboardArchives(c *gc.C, returnedVersions []params.DashboardArchiveVersion, returnedErr error) calledFunc {
	var called bool
	f := func(client *controller.Client) ([]params.DashboardArchiveVersion, error) {
		called = true
		return returnedVersions, returnedErr
	}
	s.PatchValue(dashboard.ClientDashboardArchives, f)
	return func() bool {
		return called
	}
}

func (s *upgradeDashboardSuite) patchClientSelectDashboardVersion(c *gc.C, expectedVers string, returnedErr error) calledFunc {
	var called bool
	f := func(client *controller.Client, vers version.Number) error {
		called = true
		c.Assert(vers.String(), gc.Equals, expectedVers)
		return returnedErr
	}
	s.PatchValue(dashboard.ClientSelectDashboardVersion, f)
	return func() bool {
		return called
	}
}

func (s *upgradeDashboardSuite) patchClientUploadDashboardArchive(c *gc.C, expectedHash string, expectedSize int64, expectedVers string, returnedIsCurrent bool, returnedErr error) calledFunc {
	var called bool
	f := func(client *controller.Client, r io.ReadSeeker, hash string, size int64, vers version.Number) (bool, error) {
		called = true
		c.Assert(hash, gc.Equals, expectedHash)
		c.Assert(size, gc.Equals, expectedSize)
		c.Assert(vers.String(), gc.Equals, expectedVers)
		return returnedIsCurrent, returnedErr
	}
	s.PatchValue(dashboard.ClientUploadDashboardArchive, f)
	return func() bool {
		return called
	}
}

func (s *upgradeDashboardSuite) patchDashboardFetchMetadata(c *gc.C, returnedMetadata []*envdashboard.Metadata, returnedErr error) calledFunc {
	var called bool
	f := func(stream string, major, minor int, sources ...simplestreams.DataSource) ([]*envdashboard.Metadata, error) {
		called = true
		c.Assert(major, gc.Equals, jujuversion.Current.Major)
		c.Assert(minor, gc.Equals, jujuversion.Current.Minor)
		c.Assert(stream, gc.Equals, envdashboard.DevelStream)
		c.Assert(sources[0].Description(), gc.Equals, "dashboard simplestreams")
		return returnedMetadata, returnedErr
	}
	s.PatchValue(dashboard.DashboardFetchMetadata, f)
	return func() bool {
		return called
	}
}

var upgradeDashboardInputErrorsTests = []struct {
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
	expectedError: `invalid Dashboard release version or local path "no-such-file"`,
}}

func (s *upgradeDashboardSuite) TestUpgradeDashboardInputErrors(c *gc.C) {
	for i, test := range upgradeDashboardInputErrorsTests {
		c.Logf("\n%d: %s", i, test.about)
		_, err := s.run(c, test.args...)
		c.Assert(err, gc.ErrorMatches, test.expectedError)
	}
}

func (s *upgradeDashboardSuite) TestUpgradeDashboardListSuccess(c *gc.C) {
	s.patchDashboardFetchMetadata(c, []*envdashboard.Metadata{{
		Version: version.MustParse("2.2.0"),
	}, {
		Version: version.MustParse("2.1.1"),
	}, {
		Version: version.MustParse("2.1.0"),
	}}, nil)
	uploadCalled := s.patchClientUploadDashboardArchive(c, "", 0, "", false, nil)
	selectCalled := s.patchClientSelectDashboardVersion(c, "", nil)

	// Run the command to list available Juju Dashboard archive versions.
	out, err := s.run(c, "--list", "--dashboard-stream", "devel")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Equals, "2.2.0\n2.1.1\n2.1.0")

	// No uploads or switches are preformed.
	c.Assert(uploadCalled(), jc.IsFalse)
	c.Assert(selectCalled(), jc.IsFalse)
}

func (s *upgradeDashboardSuite) TestUpgradeDashboardListNoReleases(c *gc.C) {
	s.patchDashboardFetchMetadata(c, nil, nil)
	out, err := s.run(c, "--list", "--dashboard-stream", "devel")
	c.Assert(err, gc.ErrorMatches, "cannot list Juju Dashboard release versions: no available Juju Dashboard archives found")
	c.Assert(out, gc.Equals, "")
}

func (s *upgradeDashboardSuite) TestUpgradeDashboardListError(c *gc.C) {
	s.patchDashboardFetchMetadata(c, nil, errors.New("bad wolf"))
	out, err := s.run(c, "--list", "--dashboard-stream", "devel")
	c.Assert(err, gc.ErrorMatches, "cannot list Juju Dashboard release versions: cannot retrieve Juju Dashboard archive info: bad wolf")
	c.Assert(out, gc.Equals, "")
}

func (s *upgradeDashboardSuite) TestUpgradeDashboardFileError(c *gc.C) {
	path, _, _ := saveDashboardArchive(c, "2.0.0")
	err := os.Chmod(path, 0000)
	c.Assert(err, jc.ErrorIsNil)
	defer os.Chmod(path, 0600)
	out, err := s.run(c, path)
	c.Assert(err, gc.ErrorMatches, "cannot open Dashboard archive: .*")
	c.Assert(out, gc.Equals, "")
}

func (s *upgradeDashboardSuite) TestUpgradeDashboardArchiveVersionNotValid(c *gc.C) {
	path, _, _ := saveDashboardArchive(c, "bad-wolf")
	out, err := s.run(c, path)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade Juju Dashboard using ".*": invalid version "bad-wolf" in archive`)
	c.Assert(out, gc.Equals, "")
}

func (s *upgradeDashboardSuite) TestUpgradeDashboardArchiveVersionNotFound(c *gc.C) {
	path, _, _ := saveDashboardArchive(c, "")
	out, err := s.run(c, path)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade Juju Dashboard using ".*": cannot find Juju Dashboard version in archive`)
	c.Assert(out, gc.Equals, "")
}

func (s *upgradeDashboardSuite) TestUpgradeDashboardDashboardArchivesError(c *gc.C) {
	path, _, _ := saveDashboardArchive(c, "2.1.0")
	s.patchClientDashboardArchives(c, nil, errors.New("bad wolf"))
	out, err := s.run(c, path)
	c.Assert(err, gc.ErrorMatches, "cannot retrieve Dashboard versions from the controller: bad wolf")
	c.Assert(out, gc.Equals, "")
}

func (s *upgradeDashboardSuite) TestUpgradeDashboardUploadDashboardArchiveError(c *gc.C) {
	path, hash, size := saveDashboardArchive(c, "2.2.0")
	s.patchClientDashboardArchives(c, nil, nil)
	s.patchClientUploadDashboardArchive(c, hash, size, "2.2.0", false, errors.New("bad wolf"))
	out, err := s.run(c, path)
	c.Assert(err, gc.ErrorMatches, "cannot upload Juju Dashboard: bad wolf")
	c.Assert(out, gc.Equals, "using local Juju Dashboard archive\nuploading Juju Dashboard 2.2.0")
}

func (s *upgradeDashboardSuite) TestUpgradeDashboardSelectDashboardVersionError(c *gc.C) {
	path, hash, size := saveDashboardArchive(c, "2.3.0")
	s.patchClientDashboardArchives(c, nil, nil)
	s.patchClientUploadDashboardArchive(c, hash, size, "2.3.0", false, nil)
	s.patchClientSelectDashboardVersion(c, "2.3.0", errors.New("bad wolf"))
	out, err := s.run(c, path)
	c.Assert(err, gc.ErrorMatches, "cannot switch to new Juju Dashboard version: bad wolf")
	c.Assert(out, gc.Equals, "using local Juju Dashboard archive\nuploading Juju Dashboard 2.3.0\nupload completed")
}

func (s *upgradeDashboardSuite) TestUpgradeDashboardFromSimplestreamsReleaseErrors(c *gc.C) {
	tests := []struct {
		about            string
		arg              string
		returnedMetadata []*envdashboard.Metadata
		returnedErr      error
		expectedErr      string
	}{{
		about:       "last release: no releases found",
		expectedErr: "cannot upgrade to most recent release: no available Juju Dashboard archives found",
	}, {
		about:       "specific release: no releases found",
		arg:         "2.0.42",
		expectedErr: "cannot upgrade to release 2.0.42: no available Juju Dashboard archives found",
	}, {
		about:       "last release: error while fetching releases list",
		returnedErr: errors.New("bad wolf"),
		expectedErr: "cannot upgrade to most recent release: cannot retrieve Juju Dashboard archive info: bad wolf",
	}, {
		about:       "specific release: error while fetching releases list",
		arg:         "2.0.47",
		returnedErr: errors.New("bad wolf"),
		expectedErr: "cannot upgrade to release 2.0.47: cannot retrieve Juju Dashboard archive info: bad wolf",
	}, {
		about: "last release: error while opening the remote release resource",
		returnedMetadata: []*envdashboard.Metadata{
			makeDashboardMetadata(c, "2.2.0", "exterminate"),
			makeDashboardMetadata(c, "2.1.0", ""),
		},
		expectedErr: `cannot open Juju Dashboard archive at "https://1.2.3.4/path/to/dashboard/2.2.0": exterminate`,
	}, {
		about: "specific release: error while opening the remote release resource",
		arg:   "2.1.0",
		returnedMetadata: []*envdashboard.Metadata{
			makeDashboardMetadata(c, "2.2.0", ""),
			makeDashboardMetadata(c, "2.1.0", "boo"),
			makeDashboardMetadata(c, "2.0.0", ""),
		},
		expectedErr: `cannot open Juju Dashboard archive at "https://1.2.3.4/path/to/dashboard/2.1.0": boo`,
	}, {
		about: "specific release: not found in available releases",
		arg:   "2.1.0",
		returnedMetadata: []*envdashboard.Metadata{
			makeDashboardMetadata(c, "2.2.0", ""),
			makeDashboardMetadata(c, "2.0.0", ""),
		},
		expectedErr: "Juju Dashboard release version 2.1.0 not found",
	}}

	for i, test := range tests {
		c.Logf("\n%d: %s", i, test.about)

		s.patchDashboardFetchMetadata(c, test.returnedMetadata, test.returnedErr)
		out, err := s.run(c, test.arg, "--dashboard-stream", "devel")
		c.Assert(err, gc.ErrorMatches, test.expectedErr)
		c.Assert(out, gc.Equals, "")
	}
}

func (s *upgradeDashboardSuite) TestUpgradeDashboardSuccess(c *gc.C) {
	tests := []struct {
		// about describes the test.
		about string
		// returnedMetadata holds metadata information returned by simplestreams.
		returnedMetadata *envdashboard.Metadata
		// archiveVersion is the version of the archive to be uploaded.
		archiveVersion string
		// existingVersions is a function returning a list of Dashboard archive versions
		// already included in the controller.
		existingVersions func(hash string) []params.DashboardArchiveVersion
		// opened holds whether Juju Dashboard metadata information in simplestreams
		// has been opened.
		opened bool
		// uploaded holds whether the archive has been actually uploaded. If an
		// archive with the same hash and version is already present in the
		// controller, the upload is not performed again.
		uploaded bool
		// selected holds whether a new Dashboard version must be selected. If the upload
		// upgraded the currently served version there is no need to perform
		// the API call to switch Dashboard version.
		selected bool
		// expectedOutput holds the expected upgrade-dashboard command output.
		expectedOutput string
	}{{
		about:          "archive: first archive",
		archiveVersion: "2.0.0",
		expectedOutput: "using local Juju Dashboard archive\nuploading Juju Dashboard 2.0.0\nupload completed\nJuju Dashboard switched to version 2.0.0",
		uploaded:       true,
		selected:       true,
	}, {
		about:          "archive: new archive",
		archiveVersion: "2.1.0",
		existingVersions: func(hash string) []params.DashboardArchiveVersion {
			return []params.DashboardArchiveVersion{{
				Version: version.MustParse("1.0.0"),
				SHA256:  "hash-1",
				Current: true,
			}}
		},
		uploaded:       true,
		selected:       true,
		expectedOutput: "using local Juju Dashboard archive\nuploading Juju Dashboard 2.1.0\nupload completed\nJuju Dashboard switched to version 2.1.0",
	}, {
		about:          "archive: new archive, existing non-current version",
		archiveVersion: "2.0.42",
		existingVersions: func(hash string) []params.DashboardArchiveVersion {
			return []params.DashboardArchiveVersion{{
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
		expectedOutput: "using local Juju Dashboard archive\nuploading Juju Dashboard 2.0.42\nupload completed\nJuju Dashboard switched to version 2.0.42",
	}, {
		about:          "archive: new archive, existing current version",
		archiveVersion: "2.0.47",
		existingVersions: func(hash string) []params.DashboardArchiveVersion {
			return []params.DashboardArchiveVersion{{
				Version: version.MustParse("2.0.47"),
				SHA256:  "hash-47",
				Current: true,
			}}
		},
		uploaded:       true,
		expectedOutput: "using local Juju Dashboard archive\nuploading Juju Dashboard 2.0.47\nupload completed\nJuju Dashboard at version 2.0.47",
	}, {
		about:          "archive: existing archive, existing non-current version",
		archiveVersion: "2.0.42",
		existingVersions: func(hash string) []params.DashboardArchiveVersion {
			return []params.DashboardArchiveVersion{{
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
		expectedOutput: "Juju Dashboard switched to version 2.0.42",
	}, {
		about:          "archive: existing archive, existing current version",
		archiveVersion: "1.47.0",
		existingVersions: func(hash string) []params.DashboardArchiveVersion {
			return []params.DashboardArchiveVersion{{
				Version: version.MustParse("1.47.0"),
				SHA256:  hash,
				Current: true,
			}}
		},
		expectedOutput: "Juju Dashboard at version 1.47.0",
	}, {
		about:          "archive: existing archive, different existing version",
		archiveVersion: "2.0.42",
		existingVersions: func(hash string) []params.DashboardArchiveVersion {
			return []params.DashboardArchiveVersion{{
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
		expectedOutput: "using local Juju Dashboard archive\nuploading Juju Dashboard 2.0.42\nupload completed\nJuju Dashboard switched to version 2.0.42",
	}, {
		about:            "stream: first archive",
		archiveVersion:   "2.0.0",
		returnedMetadata: makeDashboardMetadata(c, "2.0.0", ""),
		expectedOutput:   "fetching Juju Dashboard archive\nuploading Juju Dashboard 2.0.0\nupload completed\nJuju Dashboard switched to version 2.0.0",
		opened:           true,
		uploaded:         true,
		selected:         true,
	}, {
		about:            "stream: new archive",
		archiveVersion:   "2.1.0",
		returnedMetadata: makeDashboardMetadata(c, "2.1.0", ""),
		existingVersions: func(hash string) []params.DashboardArchiveVersion {
			return []params.DashboardArchiveVersion{{
				Version: version.MustParse("1.0.0"),
				SHA256:  "hash-1",
				Current: true,
			}}
		},
		opened:         true,
		uploaded:       true,
		selected:       true,
		expectedOutput: "fetching Juju Dashboard archive\nuploading Juju Dashboard 2.1.0\nupload completed\nJuju Dashboard switched to version 2.1.0",
	}, {
		about:            "stream: new archive, existing non-current version",
		archiveVersion:   "2.0.42",
		returnedMetadata: makeDashboardMetadata(c, "2.0.42", ""),
		existingVersions: func(hash string) []params.DashboardArchiveVersion {
			return []params.DashboardArchiveVersion{{
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
		expectedOutput: "fetching Juju Dashboard archive\nuploading Juju Dashboard 2.0.42\nupload completed\nJuju Dashboard switched to version 2.0.42",
	}, {
		about:            "stream: new archive, existing current version",
		archiveVersion:   "2.0.47",
		returnedMetadata: makeDashboardMetadata(c, "2.0.47", ""),
		existingVersions: func(hash string) []params.DashboardArchiveVersion {
			return []params.DashboardArchiveVersion{{
				Version: version.MustParse("2.0.47"),
				SHA256:  "hash-47",
				Current: true,
			}}
		},
		opened:         true,
		uploaded:       true,
		expectedOutput: "fetching Juju Dashboard archive\nuploading Juju Dashboard 2.0.47\nupload completed\nJuju Dashboard at version 2.0.47",
	}, {
		about:            "stream: existing archive, existing non-current version",
		archiveVersion:   "2.0.42",
		returnedMetadata: makeDashboardMetadata(c, "2.0.42", ""),
		existingVersions: func(hash string) []params.DashboardArchiveVersion {
			return []params.DashboardArchiveVersion{{
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
		expectedOutput: "Juju Dashboard switched to version 2.0.42",
	}, {
		about:            "stream: existing archive, existing current version",
		archiveVersion:   "1.47.0",
		returnedMetadata: makeDashboardMetadata(c, "1.47.0", ""),
		existingVersions: func(hash string) []params.DashboardArchiveVersion {
			return []params.DashboardArchiveVersion{{
				Version: version.MustParse("1.47.0"),
				SHA256:  hash,
				Current: true,
			}}
		},
		opened:         true,
		expectedOutput: "Juju Dashboard at version 1.47.0",
	}, {
		about:            "stream: existing archive, different existing version",
		archiveVersion:   "2.0.42",
		returnedMetadata: makeDashboardMetadata(c, "2.0.42", ""),
		existingVersions: func(hash string) []params.DashboardArchiveVersion {
			return []params.DashboardArchiveVersion{{
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
		expectedOutput: "fetching Juju Dashboard archive\nuploading Juju Dashboard 2.0.42\nupload completed\nJuju Dashboard switched to version 2.0.42",
	}}

	for i, test := range tests {
		c.Logf("\n%d: %s", i, test.about)

		var arg string
		var hash string
		var size int64

		if test.returnedMetadata == nil {
			// Create an fake Juju Dashboard local archive.
			arg, hash, size = saveDashboardArchive(c, test.archiveVersion)
		} else {
			// Use the remote metadata information.
			arg = test.returnedMetadata.Version.String()
			hash = test.returnedMetadata.SHA256
			size = test.returnedMetadata.Size
		}

		// Patch the call to get simplestreams metadata information.
		fetchMetadataCalled := s.patchDashboardFetchMetadata(c, []*envdashboard.Metadata{test.returnedMetadata}, nil)

		// Patch the call to get existing archive versions.
		var existingVersions []params.DashboardArchiveVersion
		if test.existingVersions != nil {
			existingVersions = test.existingVersions(hash)
		}
		dashboardArchivesCalled := s.patchClientDashboardArchives(c, existingVersions, nil)

		// Patch the other calls to the controller.
		uploadDashboardArchiveCalled := s.patchClientUploadDashboardArchive(c, hash, size, test.archiveVersion, !test.selected, nil)
		selectDashboardVersionCalled := s.patchClientSelectDashboardVersion(c, test.archiveVersion, nil)

		// Run the command.
		out, err := s.run(c, arg, "--dashboard-stream", "devel")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(out, gc.Equals, test.expectedOutput)
		c.Assert(dashboardArchivesCalled(), jc.IsTrue)
		c.Assert(fetchMetadataCalled(), gc.Equals, test.opened)
		c.Assert(uploadDashboardArchiveCalled(), gc.Equals, test.uploaded)
		c.Assert(selectDashboardVersionCalled(), gc.Equals, test.selected)
	}
}

func (s *upgradeDashboardSuite) TestUpgradeDashboardIntegration(c *gc.C) {
	// Prepare a Dashboard archive.
	path, hash, size := saveDashboardArchive(c, "2.42.0")

	// Upload the archive from command line.
	out, err := s.run(c, path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Equals, "using local Juju Dashboard archive\nuploading Juju Dashboard 2.42.0\nupload completed\nJuju Dashboard switched to version 2.42.0")

	// Check that the archive is present in the Dashboard storage server side.
	storage, err := s.State.DashboardStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	metadata, err := storage.Metadata("2.42.0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata.SHA256, gc.Equals, hash)
	c.Assert(metadata.Size, gc.Equals, size)

	// Check that the uploaded version has been set as the current one.
	vers, err := s.State.DashboardVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vers.String(), gc.Equals, "2.42.0")
}

// makeDashboardArchive creates a Juju Dashboard tar.bz2 archive in memory, and returns a
// reader for the archive, its SHA256 hash and size.
func makeDashboardArchive(c *gc.C, vers string) (r io.Reader, hash string, size int64) {
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
		versionData := fmt.Sprintf(`{"version": %q}`, vers)
		err = tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeReg,
			Name:     "version.json",
			Size:     int64(len(versionData)),
			Mode:     0700,
		})
		c.Assert(err, jc.ErrorIsNil)
		_, err = io.WriteString(tw, versionData)
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

// saveDashboardArchive creates a Juju Dashboard tar.bz2 archive with the given version on
// disk, and return its path, SHA256 hash and size.
func saveDashboardArchive(c *gc.C, vers string) (path, hash string, size int64) {
	r, hash, size := makeDashboardArchive(c, vers)
	path = filepath.Join(c.MkDir(), "dashboard.tar.bz2")
	data, err := ioutil.ReadAll(r)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(path, data, 0600)
	c.Assert(err, jc.ErrorIsNil)
	return path, hash, size
}

// makeDashboardMetadata creates and return a Juju Dashboard archive metadata with the
// given version. If fetchError is not empty, trying to fetch the corresponding
// archive will return the given error.
func makeDashboardMetadata(c *gc.C, vers, fetchError string) *envdashboard.Metadata {
	path, hash, size := saveDashboardArchive(c, vers)
	metaPath := "/path/to/dashboard/" + vers
	return &envdashboard.Metadata{
		Version:  version.MustParse(vers),
		SHA256:   hash,
		Size:     size,
		Path:     metaPath,
		FullPath: "https://1.2.3.4" + metaPath,
		Source: &dataSource{
			DataSource: envdashboard.NewDataSource("htpps://1.2.3.4"),
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
