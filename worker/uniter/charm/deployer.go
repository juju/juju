package charm

import (
	"fmt"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/trivial"
	"os"
	"path/filepath"
	"time"
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
	if err := trivial.EnsureDir(d.path); err != nil {
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
	path, err := d.newDir("update")
	if err != nil {
		return err
	}
	var repo *GitDir
	if srcExists {
		repo, err = d.current.Clone(path)
	} else {
		repo = NewGitDir(path)
		err = repo.Init()
	}
	if err != nil {
		return err
	}

	// Write the desired new state and commit.
	if err = bun.ExpandTo(path); err != nil {
		return err
	}
	if err = WriteCharmURL(repo, url); err != nil {
		return err
	}
	if err = repo.Snapshotf("imported charm %s from %s", url, bun.Path); err != nil {
		return err
	}

	// Atomically rename fresh repository to current.
	tmplink := filepath.Join(path, "tmplink")
	if err = os.Symlink(path, tmplink); err != nil {
		return err
	}
	return os.Rename(tmplink, d.current.Path())
}

// Deploy deploys the current charm to the target directory.
func (d *Deployer) Deploy(target *GitDir) (err error) {
	defer func() {
		if err == ErrConflict {
			log.Printf("charm deployment completed with conflicts")
		} else if err != nil {
			err = fmt.Errorf("charm deployment failed: %s", err)
			log.Printf(err.Error())
		} else {
			log.Printf("charm deployment succeeded")
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
	url, err := ReadCharmURL(d.current)
	if err != nil {
		return err
	}
	path, err := d.newDir("install")
	if err != nil {
		return err
	}
	repo := NewGitDir(path)
	if err = repo.Init(); err != nil {
		return err
	}
	if err = repo.Pull(d.current); err != nil {
		return err
	}
	if err = repo.Snapshotf("deployed charm %s", url); err != nil {
		return err
	}
	log.Printf("deploying charm")
	return os.Rename(path, target.Path())
}

// upgrade pulls from current into target. If target has local changes, but
// no conflicts, it will be snapshotted before any changes are made.
func (d *Deployer) upgrade(target *GitDir) error {
	log.Printf("preparing charm upgrade")
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

// newDir creates a new timestamped directory with the given prefix. It
// assumes that the deployer will not need to create more than 10
// directories in any given second.
func (d *Deployer) newDir(prefix string) (string, error) {
	prefix = prefix + time.Now().Format("-%Y%m%d-%H%M%S")
	var err error
	var path string
	for i := 0; i < 10; i++ {
		path = filepath.Join(d.path, fmt.Sprintf("%s-%d", prefix, i))
		if err = os.Mkdir(path, 0755); err == nil {
			return path, nil
		} else if !os.IsExist(err) {
			break
		}
	}
	return "", fmt.Errorf("failed to create %q: %v", path, err)
}
