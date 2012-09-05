package charm

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/trivial"
	"os"
	"path/filepath"
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

// SetCharm causes subsequent calls to Deploy to deploy the supplied charm.
func (d *Deployer) SetCharm(bun *charm.Bundle, url *charm.URL) error {
	if err := trivial.EnsureDir(d.path); err != nil {
		return err
	}
	defer d.collectOrphans()
	srcExists, err := d.current.Exists()
	if err != nil {
		return err
	}
	if srcExists {
		prevURL, err := d.current.ReadCharmURL()
		if err != nil {
			return err
		}
		if *url == *prevURL {
			return nil
		}
	}
	tmp, err := ioutil.TempDir(d.path, "update-")
	if err != nil {
		return err
	}
	var tmpRepo *GitDir
	if srcExists {
		tmpRepo, err = d.current.Clone(tmp)
	} else {
		tmpRepo = NewGitDir(tmp)
		err = tmpRepo.Init()
	}
	if err != nil {
		return err
	}
	if err = bun.ExpandTo(tmp); err != nil {
		return err
	}
	if err = tmpRepo.WriteCharmURL(url); err != nil {
		return err
	}
	if err = tmpRepo.Snapshotf("imported charm %s from %s", url, bun.Path); err != nil {
		return err
	}
	tmplink := filepath.Join(tmp, "tmplink")
	if err = os.Symlink(tmp, tmplink); err != nil {
		return err
	}
	return os.Rename(tmplink, d.current.Path())
}

// Deploy deploys the current charm to the target directory.
func (d *Deployer) Deploy(target *GitDir) (err error) {
	defer func() {
		if err != nil {
			if err != ErrConflict {
				err = fmt.Errorf("deploy failed: %s", err)
			}
		} else {
			log.Printf("deploy complete")
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
	log.Printf("preparing new charm deployment")
	url, err := d.current.ReadCharmURL()
	if err != nil {
		return err
	}
	tmp, err := ioutil.TempDir(d.path, "init-")
	if err != nil {
		return err
	}
	tmpRepo := NewGitDir(tmp)
	if err = tmpRepo.Init(); err != nil {
		return err
	}
	if err = tmpRepo.Pull(d.current); err != nil {
		return err
	}
	if err = tmpRepo.Snapshotf("deployed charm %s", url); err != nil {
		return err
	}
	log.Printf("deploying charm")
	return os.Rename(tmp, target.Path())
}

// upgrade pulls from current into target. If target has local changes, but
// no conflicts, it will be snapshotted before any changes are made.
func (d *Deployer) upgrade(target *GitDir) error {
	log.Printf("preparing charm upgrade")
	url, err := d.current.ReadCharmURL()
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
			log.Printf("snapshotting dirty charm before upgrade")
			if err = target.Snapshotf("pre-upgrade snapshot"); err != nil {
				return err
			}
		}
	}
	log.Printf("deploying charm")
	if err := target.Pull(d.current); err != nil {
		return err
	}
	return target.Snapshotf("upgraded charm to %s", url)
}

// collectOrphans deletes all repos in path except the one pointed to by current.
// Errors are generally ignored; some are logged.
func (d *Deployer) collectOrphans() {
	current, err := os.Readlink(d.current.Path())
	if err != nil {
		return
	}
	filepath.Walk(d.path, func(path string, fi os.FileInfo, err error) error {
		if err != nil && path != d.path && path != current {
			if err = os.RemoveAll(path); err != nil {
				log.Debugf("failed to remove orphan repo at %s: %s", path, err)
			}
		}
		return err
	})
}
