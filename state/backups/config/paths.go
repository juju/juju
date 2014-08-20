// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"path/filepath"

	"github.com/juju/errors"
)

// DefaultPaths is a Paths value with all values set to defaults.
var DefaultPaths = paths{
	// TODO(ericsnow) Pull these from elsewhere in juju.
	dataDir:        "/var/lib/juju",
	startupDir:     "/etc/init",
	loggingConfDir: "/etc/rsyslog.d",
	logsDir:        "/var/log/juju",
	sshDir:         "/home/ubuntu/.ssh",
}

// Paths is an abstraction of FS paths important to juju state.
type Paths interface {
	// FindEvery returns all matching (and existing) paths, each fully
	// resolved.  paths is a 2-tuple of (kind, relPath), where kind is
	// one of the kinds the Paths recognizes and relPath is a relative
	// path or glob.  If any one of the relative paths does not match
	// then the method fails with errors.NotFound.
	FindEvery(paths ...[]string) ([]string, error)
}

type paths struct {
	// rootDir is treated as a directory and all paths are relative to
	// it.  If rootDir is an empty string, all paths (including relative
	// ones) are treated as-is.
	rootDir string

	dataDir        string
	startupDir     string
	loggingConfDir string
	logsDir        string
	sshDir         string
}

// NewPaths returns a new Paths value with the provided values set.
func NewPaths(data, startup, loggingConf, logs, ssh string) *paths {
	newPaths := paths{
		dataDir:        data,
		startupDir:     startup,
		loggingConfDir: loggingConf,
		logsDir:        logs,
		sshDir:         ssh,
	}
	return &newPaths
}

func (p *paths) resolve(kind, relPath string) (string, error) {
	var dirName string

	switch kind {
	case "data":
		dirName = p.dataDir
	case "startup":
		dirName = p.startupDir
	case "loggingConf":
		dirName = p.loggingConfDir
	case "logs":
		dirName = p.logsDir
	case "ssh":
		dirName = p.sshDir
	default:
		return "", errors.NotFoundf(kind)
	}

	return filepath.Join(p.rootDir, dirName, relPath), nil
}

func (p *paths) FindEvery(paths ...[]string) ([]string, error) {
	var filenames []string

	for _, pair := range paths {
		kind, relPath := pair[0], pair[1]
		glob, err := p.resolve(kind, relPath)
		if err != nil {
			return nil, errors.Trace(err)
		}

		found, err := filepath.Glob(glob)
		if err != nil {
			return nil, errors.Trace(err)
		}

		if found == nil {
			return nil, errors.NotFoundf("no files found for %q", glob)
		}

		filenames = append(filenames, found...)
	}

	return filenames, nil
}

// reRoot returns a new Paths value with the new root set.
func (p *paths) reRoot(rootDir string) *paths {
	newPaths := NewPaths(
		p.dataDir,
		p.startupDir,
		p.loggingConfDir,
		p.logsDir,
		p.sshDir,
	)
	newPaths.rootDir = rootDir
	return newPaths
}
