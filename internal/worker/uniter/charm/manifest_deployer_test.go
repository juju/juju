// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	ft "github.com/juju/testing/filetesting"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter/charm"
	"github.com/juju/juju/internal/worker/uniter/charm/mocks"
)

type ManifestDeployerSuite struct {
	testing.BaseSuite
	bundles    *bundleReader
	targetPath string
	deployer   charm.Deployer
}

var _ = gc.Suite(&ManifestDeployerSuite{})

// because we generally use real charm bundles for testing, and charm bundling
// sets every file mode to 0755 or 0644, all our input data uses those modes as
// well.

func (s *ManifestDeployerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.bundles = &bundleReader{}
	s.targetPath = filepath.Join(c.MkDir(), "target")
	deployerPath := filepath.Join(c.MkDir(), "deployer")
	s.deployer = charm.NewManifestDeployer(s.targetPath, deployerPath, s.bundles, loggertesting.WrapCheckLog(c))
}

func (s *ManifestDeployerSuite) addMockCharm(revision int, bundle charm.Bundle) charm.BundleInfo {
	return s.bundles.AddBundle(charmURL(revision), bundle)
}

func (s *ManifestDeployerSuite) addCharm(c *gc.C, revision int, content ...ft.Entry) charm.BundleInfo {
	return s.bundles.AddCustomBundle(c, charmURL(revision), func(path string) {
		ft.Entries(content).Create(c, path)
	})
}

func (s *ManifestDeployerSuite) deployCharm(c *gc.C, revision int, content ...ft.Entry) charm.BundleInfo {
	info := s.addCharm(c, revision, content...)
	err := s.deployer.Stage(context.Background(), info, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.deployer.Deploy(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharm(c, revision, content...)
	return info
}

func (s *ManifestDeployerSuite) assertCharm(c *gc.C, revision int, content ...ft.Entry) {
	url, err := charm.ReadCharmURL(filepath.Join(s.targetPath, ".juju-charm"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url, gc.Equals, charmURL(revision).String())
	ft.Entries(content).Check(c, s.targetPath)
}

func (s *ManifestDeployerSuite) TestAbortStageWhenClosed(c *gc.C) {
	info := s.addMockCharm(1, mockBundle{})
	abort := make(chan struct{})
	errors := make(chan error)
	s.bundles.EnableWaitForAbort()
	go func() {
		errors <- s.deployer.Stage(context.Background(), info, abort)
	}()
	close(abort)
	err := <-errors
	c.Assert(err, gc.ErrorMatches, "charm read aborted")
}

func (s *ManifestDeployerSuite) TestDontAbortStageWhenNotClosed(c *gc.C) {
	info := s.addMockCharm(1, mockBundle{})
	abort := make(chan struct{})
	errors := make(chan error)
	stopWaiting := s.bundles.EnableWaitForAbort()
	go func() {
		errors <- s.deployer.Stage(context.Background(), info, abort)
	}()
	close(stopWaiting)
	err := <-errors
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ManifestDeployerSuite) TestDeployWithoutStage(c *gc.C) {
	err := s.deployer.Deploy(context.Background())
	c.Assert(err, gc.ErrorMatches, "charm deployment failed: no charm set")
}

func (s *ManifestDeployerSuite) TestInstall(c *gc.C) {
	s.deployCharm(c, 1,
		ft.File{Path: "some-file", Data: "hello", Perm: 0644},
		ft.Dir{Path: "some-dir", Perm: 0755},
		ft.Symlink{Path: "some-dir/some-link", Link: "../some-file"},
	)
}

func (s *ManifestDeployerSuite) TestUpgradeOverwrite(c *gc.C) {
	s.deployCharm(c, 1,
		ft.File{Path: "some-file", Data: "hello", Perm: 0644},
		ft.Dir{Path: "some-dir", Perm: 0755},
		ft.File{Path: "some-dir/another-file", Data: "to be removed", Perm: 0755},
		ft.Dir{Path: "another-dir", Perm: 0755},
		ft.Symlink{Path: "another-dir/some-link", Link: "../some-file"},
	)
	// Replace each of file, dir, and symlink with a different entry; in
	// the case of dir, checking that contained files are also removed.
	s.deployCharm(c, 2,
		ft.Symlink{Path: "some-file", Link: "no-longer-a-file"},
		ft.File{Path: "some-dir", Data: "no-longer-a-dir", Perm: 0644},
		ft.Dir{Path: "another-dir", Perm: 0755},
		ft.Dir{Path: "another-dir/some-link", Perm: 0755},
	)
}

func (s *ManifestDeployerSuite) TestUpgradePreserveUserFiles(c *gc.C) {
	originalCharmContent := ft.Entries{
		ft.File{Path: "charm-file", Data: "to-be-removed", Perm: 0644},
		ft.Dir{Path: "charm-dir", Perm: 0755},
	}
	s.deployCharm(c, 1, originalCharmContent...)

	// Add user files we expect to keep to the target dir.
	preserveUserContent := ft.Entries{
		ft.File{Path: "user-file", Data: "to-be-preserved", Perm: 0644},
		ft.Dir{Path: "user-dir", Perm: 0755},
		ft.File{Path: "user-dir/user-file", Data: "also-preserved", Perm: 0644},
	}.Create(c, s.targetPath)

	// Add some user files we expect to be removed.
	removeUserContent := ft.Entries{
		ft.File{Path: "charm-dir/user-file", Data: "whoops-removed", Perm: 0755},
	}.Create(c, s.targetPath)

	// Add some user files we expect to be replaced.
	ft.Entries{
		ft.File{Path: "replace-file", Data: "original", Perm: 0644},
		ft.Dir{Path: "replace-dir", Perm: 0755},
		ft.Symlink{Path: "replace-symlink", Link: "replace-file"},
	}.Create(c, s.targetPath)

	// Deploy an upgrade; all new content overwrites the old...
	s.deployCharm(c, 2,
		ft.File{Path: "replace-file", Data: "updated", Perm: 0644},
		ft.Dir{Path: "replace-dir", Perm: 0755},
		ft.Symlink{Path: "replace-symlink", Link: "replace-dir"},
	)

	// ...and other files are preserved or removed according to
	// source and location.
	preserveUserContent.Check(c, s.targetPath)
	removeUserContent.AsRemoveds().Check(c, s.targetPath)
	originalCharmContent.AsRemoveds().Check(c, s.targetPath)
}

func (s *ManifestDeployerSuite) TestUpgradeConflictResolveRetrySameCharm(c *gc.C) {
	// Create base install.
	s.deployCharm(c, 1,
		ft.File{Path: "shared-file", Data: "old", Perm: 0755},
		ft.File{Path: "old-file", Data: "old", Perm: 0644},
	)

	// Create mock upgrade charm that can (claim to) fail to expand...
	failDeploy := true
	upgradeContent := ft.Entries{
		ft.File{Path: "shared-file", Data: "new", Perm: 0755},
		ft.File{Path: "new-file", Data: "new", Perm: 0644},
	}
	mockCharm := mockBundle{
		paths: set.NewStrings(upgradeContent.Paths()...),
		expand: func(targetPath string) error {
			upgradeContent.Create(c, targetPath)
			if failDeploy {
				return fmt.Errorf("oh noes")
			}
			return nil
		},
	}
	info := s.addMockCharm(2, mockCharm)
	err := s.deployer.Stage(context.Background(), info, nil)
	c.Assert(err, jc.ErrorIsNil)

	// ...and see it fail to expand. We're not too bothered about the actual
	// content of the target dir at this stage, but we do want to check it's
	// still marked as based on the original charm...
	err = s.deployer.Deploy(context.Background())
	c.Assert(err, gc.Equals, charm.ErrConflict)
	s.assertCharm(c, 1)

	// ...and we want to verify that if we "fix the errors" and redeploy the
	// same charm...
	failDeploy = false
	err = s.deployer.Deploy(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	// ...we end up with the right stuff in play.
	s.assertCharm(c, 2, upgradeContent...)
	ft.Removed{Path: "old-file"}.Check(c, s.targetPath)
}

func (s *ManifestDeployerSuite) TestUpgradeConflictRevertRetryDifferentCharm(c *gc.C) {
	// Create base install and add a user file.
	s.deployCharm(c, 1,
		ft.File{Path: "shared-file", Data: "old", Perm: 0755},
		ft.File{Path: "old-file", Data: "old", Perm: 0644},
	)
	userFile := ft.File{Path: "user-file", Data: "user", Perm: 0644}.Create(c, s.targetPath)

	// Create a charm upgrade that never works (but still writes a bunch of files),
	// and deploy it.
	badUpgradeContent := ft.Entries{
		ft.File{Path: "shared-file", Data: "bad", Perm: 0644},
		ft.File{Path: "bad-file", Data: "bad", Perm: 0644},
	}
	badCharm := mockBundle{
		paths: set.NewStrings(badUpgradeContent.Paths()...),
		expand: func(targetPath string) error {
			badUpgradeContent.Create(c, targetPath)
			return fmt.Errorf("oh noes")
		},
	}
	badInfo := s.addMockCharm(2, badCharm)
	err := s.deployer.Stage(context.Background(), badInfo, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.deployer.Deploy(context.Background())
	c.Assert(err, gc.Equals, charm.ErrConflict)

	// Create a charm upgrade that creates a bunch of different files, without
	// error, and deploy it; check user files are preserved, and nothing from
	// charm 1 or 2 is.
	s.deployCharm(c, 3,
		ft.File{Path: "shared-file", Data: "new", Perm: 0755},
		ft.File{Path: "new-file", Data: "new", Perm: 0644},
	)
	userFile.Check(c, s.targetPath)
	ft.Removed{Path: "old-file"}.Check(c, s.targetPath)
	ft.Removed{Path: "bad-file"}.Check(c, s.targetPath)
}

var _ = gc.Suite(&RetryingBundleReaderSuite{})

type RetryingBundleReaderSuite struct {
	bundleReader *mocks.MockBundleReader
	bundleInfo   *mocks.MockBundleInfo
	bundle       *mocks.MockBundle
	clock        *testclock.Clock
	rbr          charm.RetryingBundleReader
}

func (s *RetryingBundleReaderSuite) TestReadBundleMaxAttemptsExceeded(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.bundleInfo.EXPECT().URL().Return("ch:focal/dummy-1").AnyTimes()
	s.bundleReader.EXPECT().Read(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.NotYetAvailablef("still in the oven")).AnyTimes()

	go func() {
		// We retry 10 times in total so we need to advance the clock 9
		// times to exceed the max retry attempts (the first attempt
		// does not use the clock).
		for i := 0; i < 9; i++ {
			c.Assert(s.clock.WaitAdvance(10*time.Second, time.Second, 1), jc.ErrorIsNil)
		}
	}()

	_, err := s.rbr.Read(context.Background(), s.bundleInfo, nil)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *RetryingBundleReaderSuite) TestReadBundleEventuallySucceeds(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.bundleInfo.EXPECT().URL().Return("ch:focal/dummy-1").AnyTimes()
	gomock.InOrder(
		s.bundleReader.EXPECT().Read(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.NotYetAvailablef("still in the oven")),
		s.bundleReader.EXPECT().Read(gomock.Any(), gomock.Any(), gomock.Any()).Return(s.bundle, nil),
	)

	go func() {
		// The first attempt should fail; advance the clock to trigger
		// another attempt which should succeed.
		c.Assert(s.clock.WaitAdvance(10*time.Second, time.Second, 1), jc.ErrorIsNil)
	}()

	got, err := s.rbr.Read(context.Background(), s.bundleInfo, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, s.bundle)
}

func (s *RetryingBundleReaderSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.bundleReader = mocks.NewMockBundleReader(ctrl)
	s.bundleInfo = mocks.NewMockBundleInfo(ctrl)
	s.bundle = mocks.NewMockBundle(ctrl)
	s.clock = testclock.NewClock(time.Now())
	s.rbr = charm.RetryingBundleReader{
		BundleReader: s.bundleReader,
		Clock:        s.clock,
		Logger:       loggertesting.WrapCheckLog(c),
	}

	return ctrl
}
