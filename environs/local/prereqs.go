// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var notLinuxError = errors.New(`The local provider is currently only available for Linux`)

const installMongodUbuntu = `
MongoDB server must be installed to enable the local provider:

    sudo apt-get install mongodb-server
`

const installMongodGeneric = `
MongoDB server must be installed to enable the local provider.
Please consult your operating system distribution's documentation
for instructions on installing the MongoDB server.`

const installLxcUbuntu = `
Linux Containers (LXC) userspace tools must be
installed to enable the local provider:

    sudo apt-get install lxc
`

const installLxcGeneric = `
Linux Containers (LXC) userspace tools must be installed to enable the
local provider. Please consult your operating system distribution's
documentation for instructions on installing the LXC userspace tools.`

// mongodPath is the path to "mongod", the MongoDB server.
// This is a variable only to support unit testing.
var mongodPath = "/usr/bin/mongod"

// The operating system the process is running in.
// This is a variable only to support unit testing.
var goos = runtime.GOOS

// VerifyPrerequisites verifies the prerequisites of
// the local machine (machine 0) for running the local
// provider.
func VerifyPrerequisites() error {
	if goos != "linux" {
		return fmt.Errorf(`
Unsupported operating system: %s
The local provider is currently only available for Linux`[1:], goos)
	}
	err := verifyMongod()
	if err != nil {
		return err
	}
	return verifyLxc()
}

func verifyMongod() error {
	_, err := os.Stat(mongodPath)
	if err != nil {
		if os.IsNotExist(err) {
			return wrapMongodNotExist(err)
		} else {
			return err
		}
	}
	// TODO verify version?
	return nil
}

func verifyLxc() error {
	_, err := exec.LookPath("lxc-ls")
	if err != nil {
		if err, ok := err.(*exec.Error); ok && err.Err == exec.ErrNotFound {
			return wrapLxcNotFound(err)
		}
		return err
	}
	return nil
}

func isUbuntu() bool {
	out, err := exec.Command("lsb_release", "-i", "-s").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "Ubuntu"
}

func wrapMongodNotExist(err error) error {
	if isUbuntu() {
		return fmt.Errorf("%v\n%s", err, installMongodUbuntu)
	}
	return fmt.Errorf("%v\n%s", err, installMongodGeneric)
}

func wrapLxcNotFound(err error) error {
	if isUbuntu() {
		return fmt.Errorf("%v\n%s", err, installLxcUbuntu)
	}
	return fmt.Errorf("%v\n%s", err, installLxcGeneric)
}
