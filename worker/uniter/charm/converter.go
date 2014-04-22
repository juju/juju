// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"os"
	"path/filepath"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/utils/set"
)

// NewDeployer returns a Deployer of whatever kind is currently in use for the
// supplied paths, or a manifest deployer if none exists yet. It is a var so
// that it can be patched for uniter tests.
var NewDeployer = newDeployer

func newDeployer(charmPath, dataPath string, bundles BundleReader) (Deployer, error) {
	gitDeployer := NewGitDeployer(charmPath, dataPath, bundles).(*gitDeployer)
	if exists, err := gitDeployer.current.Exists(); err != nil {
		return nil, err
	} else if exists {
		return gitDeployer, nil
	}
	return NewManifestDeployer(charmPath, dataPath, bundles), nil
}

// FixDeployer ensures that the supplied Deployer address points to a manifest
// deployer. If a git deployer is passed into FixDeployer, it will be converted
// to a manifest deployer, and the git deployer data will be removed. The charm
// is assumed to be in a stable state; this should not be called if there is any
// chance the git deployer is partway through an upgrade, or in a conflicted state.
// It is a var so that it can be patched for uniter tests.
var FixDeployer = fixDeployer

func fixDeployer(deployer *Deployer) error {
	if manifestDeployer, ok := (*deployer).(*manifestDeployer); ok {
		// This works around a race at the very end of this func, in which
		// the process could have been killed after removing the "current"
		// symlink but before removing the orphan repos from the data dir.
		collectGitOrphans(manifestDeployer.dataPath)
		return nil
	}
	gitDeployer, ok := (*deployer).(*gitDeployer)
	if !ok {
		return fmt.Errorf("cannot fix unknown deployer type: %T", *deployer)
	}
	logger.Infof("converting git-based deployer to manifest deployer")
	manifestDeployer := &manifestDeployer{
		charmPath: gitDeployer.target.Path(),
		dataPath:  gitDeployer.dataPath,
		bundles:   gitDeployer.bundles,
	}

	// Ensure that the staged charm matches the deployed charm: it's possible
	// that the uniter was stopped after staging, but before deploying, a new
	// bundle.
	deployedURL, err := ReadCharmURL(manifestDeployer.CharmPath(charmURLPath))
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// If we deployed something previously, we need to copy some state over.
	if deployedURL != nil {
		if err := ensureCurrentGitCharm(gitDeployer, deployedURL); err != nil {
			return err
		}
		// Now we know we've got the right stuff checked out in gitDeployer.current,
		// we can turn that into a manifest that will be used in future upgrades...
		// even if users desparate for space deleted the original bundle.
		manifest, err := gitManifest(gitDeployer.current.Path())
		if err != nil {
			return err
		}
		if err := manifestDeployer.storeManifest(deployedURL, manifest); err != nil {
			return err
		}
	}

	// We're left with the staging repo and a symlink to it. We decide deployer
	// type by checking existence of the symlink's target, so we start off by
	// trashing the symlink itself; collectGitOrphans will then delete all the
	// original deployer's repos.
	if err := os.RemoveAll(gitDeployer.current.Path()); err != nil {
		return err
	}
	// Note potential race alluded to at the start of this func.
	collectGitOrphans(gitDeployer.dataPath)

	// Phew. Done.
	*deployer = manifestDeployer
	return nil
}

// ensureCurrentGitCharm checks out progressively earlier versions of the
// gitDeployer's current staging repo, until it finds one in which the
// content of charmURLPath matches the supplied charm URL.
func ensureCurrentGitCharm(gitDeployer *gitDeployer, expectURL *charm.URL) error {
	i := 1
	repo := gitDeployer.current
	for {
		stagedURL, err := gitDeployer.current.ReadCharmURL()
		if err != nil {
			return err
		}
		logger.Debugf("staged url: %s", stagedURL)
		if *stagedURL == *expectURL {
			return nil
		}
		if err := repo.cmd("checkout", fmt.Sprintf("master~%d", i)); err != nil {
			return err
		}
		i++
	}
}

// gitManifest returns every file path in the supplied directory, *except* for:
//    * paths below .git, because we don't need to track every file: we just
//      want them all gone
//    * charmURLPath, because we don't ever want to remove that: that's how
//      the manifestDeployer keeps track of what version it's upgrading from.
// All paths are slash-separated, to match the bundle manifest format.
func gitManifest(linkPath string) (set.Strings, error) {
	dirPath, err := os.Readlink(linkPath)
	if err != nil {
		return set.NewStrings(), err
	}
	manifest := set.NewStrings()
	err = filepath.Walk(dirPath, func(path string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}
		switch relPath {
		case ".", charmURLPath:
			return nil
		case ".git":
			err = filepath.SkipDir
		}
		manifest.Add(filepath.ToSlash(relPath))
		return err
	})
	if err != nil {
		return set.NewStrings(), err
	}
	return manifest, nil
}
