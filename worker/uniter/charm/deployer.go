// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"launchpad.net/juju-core/log"
)

const (
	updatePrefix  = "update-"
	installPrefix = "install-"
)

// Deployer is responsible for installing and upgrading charms.
type Deployer interface {

	// Stage must be called to prime the Deployer to install or upgrade the
	// bundle identified by the supplied info. The abort chan can be used to
	// notify an implementation that it need not complete the operation, and
	// can immediately error out if it convenient to do so. It must always
	// be safe to restage the same bundle, or to stage a new bundle.
	Stage(info BundleInfo, abort <-chan struct{}) error

	// Deploy will install or upgrade the most recently staged bundle.
	// Behaviour is undefined if Stage has not been called.
	Deploy() error

	// NotifyRevert must be called when a conflicted deploy is abandoned, in
	// preparation for a new upgrade.
	NotifyRevert() error

	// NotifyResolved must be called when the cause of a deploy conflict has
	// been resolved, and a new deploy attempt will be made.
	NotifyResolved() error
}

// gitDeployer maintains a git repository tracking a series of charm versions,
// and can install and upgrade charm deployments to the current version.
type gitDeployer struct {
	target   *GitDir
	dataPath string
	bundles  BundleReader
	current  *GitDir
}

// NewGitDeployer creates a new Deployer which stores its state in dataPath,
// and installs or upgrades the charm at charmPath.
func NewGitDeployer(charmPath, dataPath string, bundles BundleReader) Deployer {
	return &gitDeployer{
		target:   NewGitDir(charmPath),
		dataPath: dataPath,
		bundles:  bundles,
		current:  NewGitDir(filepath.Join(dataPath, "current")),
	}
}

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

func (d *gitDeployer) NotifyRevert() error {
	return d.target.Revert()
}

func (d *gitDeployer) NotifyResolved() error {
	return d.target.Snapshotf("Upgrade conflict resolved.")
}

// install creates a new deployment of current, and atomically moves it to
// target.
func (d *gitDeployer) install() error {
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
	return os.Rename(installPath, d.target.Path())
}

// upgrade pulls from current into target. If target has local changes, but
// no conflicts, it will be snapshotted before any changes are made.
func (d *gitDeployer) upgrade() error {
	log.Infof("worker/uniter/charm: preparing charm upgrade")
	url, err := ReadCharmURL(d.current)
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
