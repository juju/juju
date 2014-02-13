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

	"launchpad.net/juju-core/container/kvm"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

var notLinuxError = errors.New("The local provider is currently only available for Linux")

const installMongodUbuntu = "MongoDB server must be installed to enable the local provider:"
const aptAddRepositoryJujuStable = `
    sudo apt-add-repository ppa:juju/stable   # required for MongoDB SSL support
    sudo apt-get update`
const aptGetInstallMongodbServer = `
    sudo apt-get install mongodb-server`

const installMongodGeneric = `
MongoDB server must be installed to enable the local provider.
Please consult your operating system distribution's documentation
for instructions on installing the MongoDB server. Juju requires
a MongoDB server built with SSL support.
`

const installLxcUbuntu = `
Linux Containers (LXC) userspace tools must be
installed to enable the local provider:
  
    sudo apt-get install lxc`

const installLxcGeneric = `
Linux Containers (LXC) userspace tools must be installed to enable the
local provider. Please consult your operating system distribution's
documentation for instructions on installing the LXC userspace tools.`

const errUnsupportedOS = `Unsupported operating system: %s
The local provider is currently only available for Linux`

// mongod path bundled specifically for juju
var jujuMongodPath = "/usr/lib/juju/bin/mongod"

// mongo path to override our detection code for testing
var testMongodPath = ""

// lxclsPath is the path to "lxc-ls", an LXC userspace tool
// we check the presence of to determine whether the
// tools are installed. This is a variable only to support
// unit testing.
var lxclsPath = "lxc-ls"

// The operating system the process is running in.
// This is a variable only to support unit testing.
var goos = runtime.GOOS

// VerifyPrerequisites verifies the prerequisites of
// the local machine (machine 0) for running the local
// provider.
func VerifyPrerequisites(containerType instance.ContainerType) error {
	if goos != "linux" {
		return fmt.Errorf(errUnsupportedOS, goos)
	}
	if err := verifyMongod(); err != nil {
		return err
	}
	switch containerType {
	case instance.LXC:
		return verifyLxc()
	case instance.KVM:
		return kvm.VerifyKVMEnabled()
	}
	return fmt.Errorf("Unknown container type specified in the config.")
}

func mongodPath() string {
	if testMongodPath != "" {
		return testMongodPath
	}
	if _, err := os.Stat(jujuMongodPath); err == nil {
		return jujuMongodPath
	}

	path, _ := utils.RunCommand("which", "mongod")
	return path
}

func verifyMongod() error {
	path := mongodPath()
	if path == "" {
		return errors.New("Couldn't find mongod")
	}

	ver, err := utils.RunCommand(path, "--version")
	if err != nil {
		return fmt.Errorf("Error checking %q version: %v", path, err)
	}

}

func parseMongoVer(data string) (version float64, build int, err error) {
	// version returns a string that looks like:
	// db version v2.4.6
	// Thu Feb 13 11:22:23.957 git version: b9925db5eac369d77a3a5f5d98a145eaaacd9673
	start := 12
	end := strings.IndexAny(ver, '\r', '\n')
	errData := strings.Replace(data, "\n", " \\n ")

	if len(data) <= start || end < start {
		return 0, 0, fmt.Errorf("Can't parse mongo version string %q", errData)
	}

	data = data[start:end]
	sep := strings.LastIndex(data, ".")
	ver, err := stringconv.ParseFloat(data[:sep], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("Can't parse mongo version %q: %v", errData, err)
	}

	build, err = stringconv.ParseInt(data[sep:])
	if err != nil {
		return 0, 0, fmt.Errorf("Can't parse mongo version %q: %v", errData, err)
	}
	return ver, build, nil
}

func verifyLxc() error {
	_, err := exec.LookPath(lxclsPath)
	if err != nil {
		return wrapLxcNotFound(err)
	}
	return nil
}

func wrapMongodNotExist(err error) error {
	if utils.IsUbuntu() {
		series := version.Current.Series
		args := []interface{}{err, installMongodUbuntu}
		format := "%v\n%s\n%s"
		if series == "precise" || series == "quantal" {
			format += "%s"
			args = append(args, aptAddRepositoryJujuStable)
		}
		args = append(args, aptGetInstallMongodbServer)
		return fmt.Errorf(format, args...)
	}
	return fmt.Errorf("%v\n%s", err, installMongodGeneric)
}

func wrapLxcNotFound(err error) error {
	if utils.IsUbuntu() {
		return fmt.Errorf("%v\n%s", err, installLxcUbuntu)
	}
	return fmt.Errorf("%v\n%s", err, installLxcGeneric)
}
