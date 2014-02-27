// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/set"
)

const (
	updatePrefix  = "update-"
	installPrefix = "install-"
	manifestsPath = "manifests"

	charmURLPath     = ".juju-charm"
	deployingURLPath = ".juju-deploying"
)

// Deployer is responsible for installing and upgrading charms.
type Deployer interface {
	Stage(info BundleInfo, abort <-chan struct{}) error
	Deploy() error
}

// manifestDeployer tracks the manifests of a series of charm bundles, and
// uses those to remove files used only by old charms.
type manifestDeployer struct {
	charmPath string
	dataPath  string
	bundles   BundleReader
	staged    struct {
		url      *charm.URL
		bundle   *charm.Bundle
		manifest set.Strings
	}
}

func NewManifestDeployer(charmPath, dataPath string, bundles BundleReader) Deployer {
	return &manifestDeployer{
		charmPath: charmPath,
		dataPath:  dataPath,
		bundles:   bundles,
	}
}

// Stage is defined in the Deployer interface.
func (d *manifestDeployer) Stage(info BundleInfo, abort <-chan struct{}) error {
	if err := d.purgeGitDeployer(); err != nil {
		return err
	}
	bundle, err := d.bundles.Read(info, abort)
	if err != nil {
		return err
	}
	manifest, err := bundle.Manifest()
	if err != nil {
		return err
	}
	url := info.URL()
	if err := d.storeManifest(url, manifest); err != nil {
		return err
	}
	d.staged.url = url
	d.staged.bundle = bundle
	d.staged.manifest = manifest
	return nil
}

func (d *manifestDeployer) storeManifest(url *charm.URL, manifest set.Strings) error {
	if err := os.MkdirAll(d.DataPath(manifestsPath), 0755); err != nil {
		return err
	}
	name := charm.Quote(url.String())
	path := filepath.Join(d.DataPath(manifestsPath), name)
	return utils.WriteYaml(path, manifest.SortedValues())
}

func (d *manifestDeployer) loadManifest(urlFilePath string) (*charm.URL, set.Strings, error) {
	url, err := ReadCharmURL(d.CharmPath(urlFilePath))
	if err != nil {
		return nil, set.NewStrings(), err
	}
	name := charm.Quote(url.String())
	path := filepath.Join(d.DataPath(manifestsPath), name)
	manifest := []string{}
	err = utils.ReadYaml(path, &manifest)
	if os.IsNotExist(err) {
		log.Warningf("manifest not found at %q: files from charm %q may be left unremoved", path, url)
		err = nil
	}
	return url, set.NewStrings(manifest...), err
}

// Deploy is defined in the Deployer interface.
func (d *manifestDeployer) Deploy() error {
	baseURL, baseManifest, err := d.loadManifest(charmURLPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	deployingURL, deployingManifest, err := d.loadManifest(deployingURLPath)
	if err == nil {
		// A deploy was already in progress.
		log.Debugf("detected interrupted deploy of charm %q", deployingURL)
		if *deployingURL != *d.staged.url {
			// Remove any files from the interrupted deploy.
			log.Debugf("removing files from charm %q", deployingURL)
			if err := d.removeDiff(deployingManifest, baseManifest); err != nil {
				return err
			}
		}
	} else if os.IsNotExist(err) {
		if baseURL != nil && *baseURL == *d.staged.url {
			log.Debugf("already deployed charm %q", deployingURL)
			return nil
		}
	} else {
		return err
	}
	// Write or overwrite the deploying URL to point to the staged one.
	log.Debugf("preparing to deploy charm %q", deployingURL)
	if err := d.startDeploy(); err != nil {
		return err
	}
	// Delete files in the base version not present in the staged charm.
	if err := d.removeDiff(baseManifest, d.staged.manifest); err != nil {
		return err
	}
	// Overwrite whatever's in place with the staged charm.
	log.Debugf("deploying charm %q", d.staged.url)
	if err := d.staged.bundle.ExpandTo(d.charmPath); err != nil {
		return err
	}
	// Move the deploying file over the charm URL file, and we're done.
	log.Debugf("finishing deploy of charm %q", d.staged.url)
	return d.finishDeploy()
}

// CharmPath returns the supplied path joined to the ManfiestDeployer's charm directory.
func (d *manifestDeployer) CharmPath(path string) string {
	return filepath.Join(d.charmPath, path)
}

// CharmPath returns the supplied path joined to the ManfiestDeployer's data directory.
func (d *manifestDeployer) DataPath(path string) string {
	return filepath.Join(d.dataPath, path)
}

// removeDiff removes every path in oldManifest that is not present in newManifest.
func (d *manifestDeployer) removeDiff(oldManifest, newManifest set.Strings) error {
	for _, path := range oldManifest.Difference(newManifest).SortedValues() {
		if err := os.RemoveAll(filepath.Join(d.charmPath, path)); err != nil {
			return err
		}
	}
	return nil
}

// startDeploy persists the fact that we've started deploying the staged bundle.
func (d *manifestDeployer) startDeploy() error {
	if d.staged.url == nil {
		return fmt.Errorf("no charm staged; cannot start deploying")
	}
	if err := os.MkdirAll(d.charmPath, 0755); err != nil {
		return err
	}
	return WriteCharmURL(d.CharmPath(deployingURLPath), d.staged.url)
}

// finishDeploy persists the fact that we've finished deploying the staged bundle.
func (d *manifestDeployer) finishDeploy() error {
	oldPath := d.CharmPath(deployingURLPath)
	newPath := d.CharmPath(charmURLPath)
	return os.Rename(oldPath, newPath)
}

func (d *manifestDeployer) purgeGitDeployer() error {
	return nil //fmt.Errorf("notimpl")
}

//-
//-
//-
//-
//-
//-
//-
//-
//-
//-
//-
//-

// gitDeployer maintains a git repository tracking a series of charm versions,
// and can install and upgrade charm deployments to the current version.
type gitDeployer struct {
	charmPath string
	dataPath  string
	bundles   BundleReader
	current   *GitDir
}

// NewGitDeployer creates a new Deployer which stores its state in the supplied
// directory.
func NewGitDeployer(charmPath, dataPath string, bundles BundleReader) Deployer {
	return &gitDeployer{
		charmPath: charmPath,
		dataPath:  dataPath,
		bundles:   bundles,
		current:   NewGitDir(filepath.Join(dataPath, "current")),
	}
}

// Stage causes subsequent calls to Deploy to deploy the supplied charm.
func (d *gitDeployer) Stage(info BundleInfo, abort <-chan struct{}) error {
	// Make sure we've got an actual bundle available.
	bundle, err := d.bundles.Read(info, abort)
	if err != nil {
		return err
	}

	// Read present state of current.
	if err := os.MkdirAll(d.dataPath, 0755); err != nil {
		return err
	}
	defer d.collectOrphans()
	srcExists, err := d.current.Exists()
	if err != nil {
		return err
	}
	url := info.URL()
	if srcExists {
		prevURL, err := ReadCharmURL(d.current)
		if err != nil {
			return err
		}
		if *url == *prevURL {
			return nil
		}
	}

	// Prepare a fresh repository for the update, using current's history
	// if it exists.
	updatePath, err := d.newDir(updatePrefix)
	if err != nil {
		return err
	}
	var repo *GitDir
	if srcExists {
		repo, err = d.current.Clone(updatePath)
	} else {
		repo = NewGitDir(updatePath)
		err = repo.Init()
	}
	if err != nil {
		return err
	}

	// Write the desired new state and commit.
	if err = bundle.ExpandTo(updatePath); err != nil {
		return err
	}
	if err = WriteCharmURL(repo, url); err != nil {
		return err
	}
	if err = repo.Snapshotf("Imported charm %q from %q.", url, bundle.Path); err != nil {
		return err
	}

	// Atomically rename fresh repository to current.
	tmplink := filepath.Join(updatePath, "tmplink")
	if err = os.Symlink(updatePath, tmplink); err != nil {
		return err
	}
	return os.Rename(tmplink, d.current.Path())
}

// Deploy deploys the current charm to the target directory.
func (d *gitDeployer) Deploy() (err error) {
	defer func() {
		if err == ErrConflict {
			log.Warningf("worker/uniter/charm: charm deployment completed with conflicts")
		} else if err != nil {
			err = fmt.Errorf("charm deployment failed: %s", err)
			log.Errorf("worker/uniter/charm: %v", err)
		} else {
			log.Infof("worker/uniter/charm: charm deployment succeeded")
		}
	}()
	if exists, err := d.current.Exists(); err != nil {
		return err
	} else if !exists {
		return fmt.Errorf("no charm set")
	}
	target := NewGitDir(d.charmPath)
	if exists, err := target.Exists(); err != nil {
		return err
	} else if !exists {
		return d.install(target)
	}
	return d.upgrade(target)
}

// install creates a new deployment of current, and atomically moves it to
// target.
func (d *gitDeployer) install(target *GitDir) error {
	defer d.collectOrphans()
	log.Infof("worker/uniter/charm: preparing new charm deployment")
	url, err := ReadCharmURL(d.current)
	if err != nil {
		return err
	}
	installPath, err := d.newDir(installPrefix)
	if err != nil {
		return err
	}
	repo := NewGitDir(installPath)
	if err = repo.Init(); err != nil {
		return err
	}
	if err = repo.Pull(d.current); err != nil {
		return err
	}
	if err = repo.Snapshotf("Deployed charm %q.", url); err != nil {
		return err
	}
	log.Infof("worker/uniter/charm: deploying charm")
	return os.Rename(installPath, target.Path())
}

// upgrade pulls from current into target. If target has local changes, but
// no conflicts, it will be snapshotted before any changes are made.
func (d *gitDeployer) upgrade(target *GitDir) error {
	log.Infof("worker/uniter/charm: preparing charm upgrade")
	url, err := ReadCharmURL(d.current)
	if err != nil {
		return err
	}
	if err := target.Init(); err != nil {
		return err
	}
	if dirty, err := target.Dirty(); err != nil {
		return err
	} else if dirty {
		if conflicted, err := target.Conflicted(); err != nil {
			return err
		} else if !conflicted {
			log.Infof("worker/uniter/charm: snapshotting dirty charm before upgrade")
			if err = target.Snapshotf("Pre-upgrade snapshot."); err != nil {
				return err
			}
		}
	}
	log.Infof("worker/uniter/charm: deploying charm")
	if err := target.Pull(d.current); err != nil {
		return err
	}
	return target.Snapshotf("Upgraded charm to %q.", url)
}

// collectOrphans deletes all repos in dataPath except the one pointed to by current.
// Errors are generally ignored; some are logged.
func (d *gitDeployer) collectOrphans() {
	current, err := os.Readlink(d.current.Path())
	if err != nil {
		return
	}
	if !filepath.IsAbs(current) {
		current = filepath.Join(d.dataPath, current)
	}
	orphans, err := filepath.Glob(filepath.Join(d.dataPath, fmt.Sprintf("%s*", updatePrefix)))
	if err != nil {
		return
	}
	installOrphans, err := filepath.Glob(filepath.Join(d.dataPath, fmt.Sprintf("%s*", installPrefix)))
	if err != nil {
		return
	}
	orphans = append(orphans, installOrphans...)
	for _, repoPath := range orphans {
		if repoPath != d.dataPath && repoPath != current {
			if err = os.RemoveAll(repoPath); err != nil {
				log.Warningf("worker/uniter/charm: failed to remove orphan repo at %s: %s", repoPath, err)
			}
		}
	}
}

// newDir creates a new timestamped directory with the given prefix. It
// assumes that the deployer will not need to create more than 10
// directories in any given second.
func (d *gitDeployer) newDir(prefix string) (string, error) {
	return ioutil.TempDir(d.dataPath, prefix+time.Now().Format("20060102-150405"))
}
