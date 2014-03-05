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
	updatePrefix   = "update-"
	installPrefix  = "install-"
	gitCurrentPath = "current"
	manifestsPath  = "manifests"

	charmURLPath     = ".juju-charm"
	deployingURLPath = ".juju-deploying"
)

// Deployer is responsible for installing and upgrading charms.
type Deployer interface {
	Stage(info BundleInfo, abort <-chan struct{}) error
	Deploy() error

	// NotifyResolved must be called to inform the Deployer that a conflicted
	// deploy has been unblocked by some change.
	NotifyResolved() error

	// NotifyRevert must be called to inform the Deployer that a conflicted
	// deploy has failed irreparably, and future deployments should be made
	// based on the state before the attempted deploy was started.
	NotifyRevert() error
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
func (d *manifestDeployer) Deploy() (err error) {
	baseURL, baseManifest, err := d.loadManifest(charmURLPath)
	defer func() {
		if err != nil {
			if baseURL != nil {
				// We now treat any failure to overwrite the charm as a conflict,
				// because we know the charm itself is OK; it's thus plausible for
				// a user to get in there and fix it.
				log.Errorf("cannot upgrade charm: %v", err)
				err = ErrConflict
			} else {
				// ...but if we can't install at all, we just fail out as before,
				// because I'm not willing to mess around with the state machine to
				// accommodate a case rare enough that we've never heard of it
				// actually happening.
				err = fmt.Errorf("cannot install charm: %v", err)
			}
		}
	}()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	deployingURL, deployingManifest, err := d.loadManifest(deployingURLPath)
	if err == nil {
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

func (d *manifestDeployer) NotifyResolved() error {
	// Maybe it is resolved, maybe not. We'll find out soon enough, but we
	// don't need to take any action now.
	return nil
}

func (d *manifestDeployer) NotifyRevert() error {
	// The Deploy implementation always reverts when required anyway, so we
	// need take no action right now.
	return nil
}

// CharmPath returns the supplied path joined to the ManfiestDeployer's charm directory.
func (d *manifestDeployer) CharmPath(path string) string {
	return filepath.Join(d.charmPath, path)
}

// DataPath returns the supplied path joined to the ManfiestDeployer's data directory.
func (d *manifestDeployer) DataPath(path string) string {
	return filepath.Join(d.dataPath, path)
}

// removeDiff removes every path in oldManifest that is not present in newManifest.
func (d *manifestDeployer) removeDiff(oldManifest, newManifest set.Strings) error {
	diff := oldManifest.Difference(newManifest)
	for _, path := range diff.SortedValues() {
		fullPath := filepath.Join(d.charmPath, filepath.FromSlash(path))
		if err := os.RemoveAll(fullPath); err != nil {
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

// gitDeployer maintains a git repository tracking a series of charm versions,
// and can install and upgrade charm deployments to the current version. It
// should not be used under any circumstances.
type gitDeployer struct {
	target   *GitDir
	dataPath string
	bundles  BundleReader
	current  *GitDir
}

// NewGitDeployer creates a new Deployer which stores its state in the supplied
// directory.
func NewGitDeployer(charmPath, dataPath string, bundles BundleReader) Deployer {
	return &gitDeployer{
		target:   NewGitDir(charmPath),
		dataPath: dataPath,
		bundles:  bundles,
		current:  NewGitDir(filepath.Join(dataPath, gitCurrentPath)),
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
	defer collectGitOrphans(d.dataPath)
	srcExists, err := d.current.Exists()
	if err != nil {
		return err
	}
	url := info.URL()
	if srcExists {
		prevURL, err := d.current.ReadCharmURL()
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
	if err = repo.WriteCharmURL(url); err != nil {
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
	if exists, err := d.target.Exists(); err != nil {
		return err
	} else if !exists {
		return d.install()
	}
	return d.upgrade()
}

func (d *gitDeployer) NotifyResolved() error {
	return d.target.Snapshotf("Upgrade conflict resolved.")
}

func (d *gitDeployer) NotifyRevert() error {
	return d.target.Revert()
}

// install creates a new deployment of current, and atomically moves it to
// target.
func (d *gitDeployer) install() error {
	defer collectGitOrphans(d.dataPath)
	log.Infof("worker/uniter/charm: preparing new charm deployment")
	url, err := d.current.ReadCharmURL()
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
	return os.Rename(installPath, d.target.Path())
}

// upgrade pulls from current into target. If target has local changes, but
// no conflicts, it will be snapshotted before any changes are made.
func (d *gitDeployer) upgrade() error {
	log.Infof("worker/uniter/charm: preparing charm upgrade")
	url, err := d.current.ReadCharmURL()
	if err != nil {
		return err
	}
	if err := d.target.Init(); err != nil {
		return err
	}
	if dirty, err := d.target.Dirty(); err != nil {
		return err
	} else if dirty {
		if conflicted, err := d.target.Conflicted(); err != nil {
			return err
		} else if !conflicted {
			log.Infof("worker/uniter/charm: snapshotting dirty charm before upgrade")
			if err = d.target.Snapshotf("Pre-upgrade snapshot."); err != nil {
				return err
			}
		}
	}
	log.Infof("worker/uniter/charm: deploying charm")
	if err := d.target.Pull(d.current); err != nil {
		return err
	}
	return d.target.Snapshotf("Upgraded charm to %q.", url)
}

// collectGitOrphans deletes all repos in dataPath except the one pointed to by
// a git deployer's "current" symlink.
// Errors are generally ignored; some are logged. If current does not exist, *all*
// repos are orphans, and all will be deleted; this should only be the case when
// converting a gitDeployer to a manifestDeployer.
func collectGitOrphans(dataPath string) {
	current, err := os.Readlink(filepath.Join(dataPath, gitCurrentPath))
	if os.IsNotExist(err) {
		log.Warningf("worker/uniter/charm: no current staging repo")
	} else if err != nil {
		return
	} else if !filepath.IsAbs(current) {
		current = filepath.Join(dataPath, current)
	}
	orphans, err := filepath.Glob(filepath.Join(dataPath, fmt.Sprintf("%s*", updatePrefix)))
	if err != nil {
		return
	}
	installOrphans, err := filepath.Glob(filepath.Join(dataPath, fmt.Sprintf("%s*", installPrefix)))
	if err != nil {
		return
	}
	orphans = append(orphans, installOrphans...)
	for _, repoPath := range orphans {
		if repoPath != dataPath && repoPath != current {
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
