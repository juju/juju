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
)

const (
	updatePrefix  = "update-"
	installPrefix = "install-"
)

// Deployer maintains a git repository tracking a series of charm versions,
// and can install and upgrade charm deployments to the current version.
type Deployer struct {
	path    string
	current *GitDir
}

// NewDeployer creates a new Deployer which stores its state in the supplied
// directory.
func NewDeployer(path string) *Deployer {
	return &Deployer{
		path:    path,
		current: NewGitDir(filepath.Join(path, "current")),
	}
}

// Stage causes subsequent calls to Deploy to deploy the supplied charm.
func (d *Deployer) Stage(bun *charm.Bundle, url *charm.URL) error {
	// Read present state of current.
	if err := os.MkdirAll(d.path, 0755); err != nil {
		return err
	}
	defer d.collectOrphans()
	srcExists, err := d.current.Exists()
	if err != nil {
		return err
	}
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
	if err = bun.ExpandTo(updatePath); err != nil {
		return err
	}
	if err = WriteCharmURL(repo, url); err != nil {
		return err
	}
	if err = repo.Snapshotf("Imported charm %q from %q.", url, bun.Path); err != nil {
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
func (d *Deployer) Deploy(target *GitDir) (err error) {
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
	if exists, err := target.Exists(); err != nil {
		return err
	} else if !exists {
		return d.install(target)
	}
	return d.upgrade(target)
}

// install creates a new deployment of current, and atomically moves it to
// target.
func (d *Deployer) install(target *GitDir) error {
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
func (d *Deployer) upgrade(target *GitDir) error {
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

// collectOrphans deletes all repos in path except the one pointed to by current.
// Errors are generally ignored; some are logged.
func (d *Deployer) collectOrphans() {
	current, err := os.Readlink(d.current.Path())
	if err != nil {
		return
	}
	if !filepath.IsAbs(current) {
		current = filepath.Join(d.path, current)
	}
	orphans, err := filepath.Glob(filepath.Join(d.path, fmt.Sprintf("%s*", updatePrefix)))
	if err != nil {
		return
	}
	installOrphans, err := filepath.Glob(filepath.Join(d.path, fmt.Sprintf("%s*", installPrefix)))
	if err != nil {
		return
	}
	orphans = append(orphans, installOrphans...)
	for _, repoPath := range orphans {
		if repoPath != d.path && repoPath != current {
			if err = os.RemoveAll(repoPath); err != nil {
				log.Warningf("worker/uniter/charm: failed to remove orphan repo at %s: %s", repoPath, err)
			}
		}
	}
}

// newDir creates a new timestamped directory with the given prefix. It
// assumes that the deployer will not need to create more than 10
// directories in any given second.
func (d *Deployer) newDir(prefix string) (string, error) {
	return ioutil.TempDir(d.path, prefix+time.Now().Format("20060102-150405"))
}
