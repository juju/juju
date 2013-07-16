// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
)

var checkIfRoot = func() bool {
	return os.Getuid() == 0
}

var configChecker = schema.StrictFieldMap(
	schema.Fields{
		"root-dir": schema.String(),
	},
	schema.Defaults{
		"root-dir": "",
	},
)

type environConfig struct {
	*config.Config
	user          string
	attrs         map[string]interface{}
	runningAsRoot bool
}

func newEnvironConfig(config *config.Config, attrs map[string]interface{}) *environConfig {
	user := os.Getenv("USER")
	root := checkIfRoot()
	if root {
		sudo_user := os.Getenv("SUDO_USER")
		if sudo_user != "" {
			user = sudo_user
		}
	}
	return &environConfig{
		Config:        config,
		user:          user,
		attrs:         attrs,
		runningAsRoot: root,
	}
}

// Since it is technically possible for two different users on one machine to
// have the same local provider name, we need to have a simple way to
// namespace the file locations, but more importantly the lxc containers.
func (c *environConfig) namespace() string {
	return fmt.Sprintf("%s-%s", c.user, c.Name())
}

func (c *environConfig) rootDir() string {
	return c.attrs["root-dir"].(string)
}

func (c *environConfig) sharedStorageDir() string {
	return filepath.Join(c.rootDir(), "shared-storage")
}

func (c *environConfig) storageDir() string {
	return filepath.Join(c.rootDir(), "storage")
}

func (c *environConfig) mongoDir() string {
	return filepath.Join(c.rootDir(), "db")
}

func (c *environConfig) configFile(filename string) string {
	return filepath.Join(c.rootDir(), filename)
}

// sudoCallerIds returns the user id and group id of the SUDO caller.
// If either is unset, it returns zero for both values.
// An error is returned if the relevant environment variables
// are not valid integers.
func sudoCallerIds() (int, int, error) {
	uidStr := os.Getenv("SUDO_UID")
	gidStr := os.Getenv("SUDO_GID")

	if uidStr == "" || gidStr == "" {
		return 0, 0, nil
	}
	uid, err := strconv.Atoi(uidStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid value %q for SUDO_UID", uidStr)
	}
	gid, err := strconv.Atoi(gidStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid value %q for SUDO_GID", gidStr)
	}
	return uid, gid, nil
}

func (c *environConfig) createDirs() error {
	for _, dirname := range []string{
		c.sharedStorageDir(),
		c.storageDir(),
		c.mongoDir(),
	} {
		logger.Tracef("creating directory %s", dirname)
		if err := os.MkdirAll(dirname, 0755); err != nil {
			return err
		}
	}
	if c.runningAsRoot {
		// If we have SUDO_UID and SUDO_GID, start with rootDir(), and
		// change ownership of the directories.
		uid, gid, err := sudoCallerIds()
		if err != nil {
			return err
		}
		if uid != 0 || gid != 0 {
			filepath.Walk(c.rootDir(),
				func(path string, info os.FileInfo, err error) error {
					if info != nil && info.IsDir() {
						if err := os.Chown(path, uid, gid); err != nil {
							return err
						}
					}
					return nil
				})
		}
	}
	return nil
}
