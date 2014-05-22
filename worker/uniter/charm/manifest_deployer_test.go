// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"fmt"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
	ft "launchpad.net/juju-core/testing/filetesting"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/worker/uniter/charm"
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
	s.deployer = charm.NewManifestDeployer(s.targetPath, deployerPath, s.bundles)
}

func (s *ManifestDeployerSuite) addMockCharm(c *gc.C, revision int, bundle charm.Bundle) charm.BundleInfo {
	return s.bundles.AddBundle(c, charmURL(revision), bundle)
}

func (s *ManifestDeployerSuite) addCharm(c *gc.C, revision int, content ...ft.Entry) charm.BundleInfo {
	return s.bundles.AddCustomBundle(c, charmURL(revision), func(path string) {
		ft.Entries(content).Create(c, path)
	})
}

func (s *ManifestDeployerSuite) deployCharm(c *gc.C, revision int, content ...ft.Entry) charm.BundleInfo {
	info := s.addCharm(c, revision, content...)
	err := s.deployer.Stage(info, nil)
	c.Assert(err, gc.IsNil)
	err = s.deployer.Deploy()
	c.Assert(err, gc.IsNil)
	s.assertCharm(c, revision, content...)
	return info
}

func (s *ManifestDeployerSuite) assertCharm(c *gc.C, revision int, content ...ft.Entry) {
	url, err := charm.ReadCharmURL(filepath.Join(s.targetPath, ".juju-charm"))
	c.Assert(err, gc.IsNil)
	c.Assert(url, gc.DeepEquals, charmURL(revision))
	ft.Entries(content).Check(c, s.targetPath)
}

func (s *ManifestDeployerSuite) TestAbortStageWhenClosed(c *gc.C) {
	info := s.addMockCharm(c, 1, mockBundle{})
	abort := make(chan struct{})
	errors := make(chan error)
	s.bundles.EnableWaitForAbort()
	go func() {
		errors <- s.deployer.Stage(info, abort)
	}()
	close(abort)
	err := <-errors
	c.Assert(err, gc.ErrorMatches, "charm read aborted")
}

func (s *ManifestDeployerSuite) TestDontAbortStageWhenNotClosed(c *gc.C) {
	info := s.addMockCharm(c, 1, mockBundle{})
	abort := make(chan struct{})
	errors := make(chan error)
	stopWaiting := s.bundles.EnableWaitForAbort()
	go func() {
		errors <- s.deployer.Stage(info, abort)
	}()
	close(stopWaiting)
	err := <-errors
	c.Assert(err, gc.IsNil)
}

func (s *ManifestDeployerSuite) TestDeployWithoutStage(c *gc.C) {
	err := s.deployer.Deploy()
	c.Assert(err, gc.ErrorMatches, "charm deployment failed: no charm set")
}

func (s *ManifestDeployerSuite) TestInstall(c *gc.C) {
	s.deployCharm(c, 1,
		ft.File{"some-file", "hello", 0644},
		ft.Dir{"some-dir", 0755},
		ft.Symlink{"some-dir/some-link", "../some-file"},
	)
}

func (s *ManifestDeployerSuite) TestUpgradeOverwrite(c *gc.C) {
	s.deployCharm(c, 1,
		ft.File{"some-file", "hello", 0644},
		ft.Dir{"some-dir", 0755},
		ft.File{"some-dir/another-file", "to be removed", 0755},
		ft.Dir{"another-dir", 0755},
		ft.Symlink{"another-dir/some-link", "../some-file"},
	)
	// Replace each of file, dir, and symlink with a different entry; in
	// the case of dir, checking that contained files are also removed.
	s.deployCharm(c, 2,
		ft.Symlink{"some-file", "no-longer-a-file"},
		ft.File{"some-dir", "no-longer-a-dir", 0644},
		ft.Dir{"another-dir", 0755},
		ft.Dir{"another-dir/some-link", 0755},
	)
}

func (s *ManifestDeployerSuite) TestUpgradePreserveUserFiles(c *gc.C) {
	originalCharmContent := ft.Entries{
		ft.File{"charm-file", "to-be-removed", 0644},
		ft.Dir{"charm-dir", 0755},
	}
	s.deployCharm(c, 1, originalCharmContent...)

	// Add user files we expect to keep to the target dir.
	preserveUserContent := ft.Entries{
		ft.File{"user-file", "to-be-preserved", 0644},
		ft.Dir{"user-dir", 0755},
		ft.File{"user-dir/user-file", "also-preserved", 0644},
	}.Create(c, s.targetPath)

	// Add some user files we expect to be removed.
	removeUserContent := ft.Entries{
		ft.File{"charm-dir/user-file", "whoops-removed", 0755},
	}.Create(c, s.targetPath)

	// Add some user files we expect to be replaced.
	ft.Entries{
		ft.File{"replace-file", "original", 0644},
		ft.Dir{"replace-dir", 0755},
		ft.Symlink{"replace-symlink", "replace-file"},
	}.Create(c, s.targetPath)

	// Deploy an upgrade; all new content overwrites the old...
	s.deployCharm(c, 2,
		ft.File{"replace-file", "updated", 0644},
		ft.Dir{"replace-dir", 0755},
		ft.Symlink{"replace-symlink", "replace-dir"},
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
		ft.File{"shared-file", "old", 0755},
		ft.File{"old-file", "old", 0644},
	)

	// Create mock upgrade charm that can (claim to) fail to expand...
	failDeploy := true
	upgradeContent := ft.Entries{
		ft.File{"shared-file", "new", 0755},
		ft.File{"new-file", "new", 0644},
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
	info := s.addMockCharm(c, 2, mockCharm)
	err := s.deployer.Stage(info, nil)
	c.Assert(err, gc.IsNil)

	// ...and see it fail to expand. We're not too bothered about the actual
	// content of the target dir at this stage, but we do want to check it's
	// still marked as based on the original charm...
	err = s.deployer.Deploy()
	c.Assert(err, gc.Equals, charm.ErrConflict)
	s.assertCharm(c, 1)

	// ...and we want to verify that if we "fix the errors" and redeploy the
	// same charm...
	failDeploy = false
	err = s.deployer.NotifyResolved()
	c.Assert(err, gc.IsNil)
	err = s.deployer.Deploy()
	c.Assert(err, gc.IsNil)

	// ...we end up with the right stuff in play.
	s.assertCharm(c, 2, upgradeContent...)
	ft.Removed{"old-file"}.Check(c, s.targetPath)
}

func (s *ManifestDeployerSuite) TestUpgradeConflictRevertRetryDifferentCharm(c *gc.C) {
	// Create base install and add a user file.
	s.deployCharm(c, 1,
		ft.File{"shared-file", "old", 0755},
		ft.File{"old-file", "old", 0644},
	)
	userFile := ft.File{"user-file", "user", 0644}.Create(c, s.targetPath)

	// Create a charm upgrade that never works (but still writes a bunch of files),
	// and deploy it.
	badUpgradeContent := ft.Entries{
		ft.File{"shared-file", "bad", 0644},
		ft.File{"bad-file", "bad", 0644},
	}
	badCharm := mockBundle{
		paths: set.NewStrings(badUpgradeContent.Paths()...),
		expand: func(targetPath string) error {
			badUpgradeContent.Create(c, targetPath)
			return fmt.Errorf("oh noes")
		},
	}
	badInfo := s.addMockCharm(c, 2, badCharm)
	err := s.deployer.Stage(badInfo, nil)
	c.Assert(err, gc.IsNil)
	err = s.deployer.Deploy()
	c.Assert(err, gc.Equals, charm.ErrConflict)

	// Notify the Deployer that it'll be expected to revert the changes from
	// the last attempt.
	err = s.deployer.NotifyRevert()
	c.Assert(err, gc.IsNil)

	// Create a charm upgrade that creates a bunch of different files, without
	// error, and deploy it; check user files are preserved, and nothing from
	// charm 1 or 2 is.
	s.deployCharm(c, 3,
		ft.File{"shared-file", "new", 0755},
		ft.File{"new-file", "new", 0644},
	)
	userFile.Check(c, s.targetPath)
	ft.Removed{"old-file"}.Check(c, s.targetPath)
	ft.Removed{"bad-file"}.Check(c, s.targetPath)
}
