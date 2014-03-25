// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

// This file contains wrappers around the following executables:
//   uvt-simplestreams-libvirt
//   uvt-kvm
//   virsh
// Those executables are found in the following packages:
//   uvtool-libvirt
//   libvirt-bin
//
// These executables provide Juju's interface to dealing with kvm containers.
// The define how we start, stop and list running containers on the host

import (
	"fmt"
	"regexp"
	"strings"

	"launchpad.net/juju-core/utils"
)

var (
	// The regular expression for breaking up the results of 'virsh list'
	// (?m) - specify that this is a multiline regex
	// first part is the opaque identifier we don't care about
	// then the hostname, and lastly the status.
	machineListPattern = regexp.MustCompile(`(?m)^\s+\d+\s+(?P<hostname>[-\w]+)\s+(?P<status>.+)\s*$`)
)

// run the command and return the combined output.
func run(command string, args ...string) (output string, err error) {
	logger.Tracef("%s %v", command, args)
	output, err = utils.RunCommand(command, args...)
	logger.Tracef("output: %v", output)
	return output, err
}

// SyncImages updates the local cached images by reading the simplestreams
// data and downloading the cloud images to the uvtool pool (used by libvirt).
func SyncImages(series string, arch string) error {
	args := []string{
		"sync",
		fmt.Sprintf("arch=%s", arch),
		fmt.Sprintf("release=%s", series),
	}
	_, err := run("uvt-simplestreams-libvirt", args...)
	return err
}

type CreateMachineParams struct {
	Hostname      string
	Series        string
	Arch          string
	UserDataFile  string
	NetworkBridge string
	Memory        uint64
	CpuCores      uint64
	RootDisk      uint64
}

// CreateMachine creates a virtual machine and starts it.
func CreateMachine(params CreateMachineParams) error {
	if params.Hostname == "" {
		return fmt.Errorf("Hostname is required")
	}
	args := []string{
		"create",
		"--log-console-output", // do wonder where this goes...
	}
	if params.UserDataFile != "" {
		args = append(args, "--user-data", params.UserDataFile)
	}
	if params.NetworkBridge != "" {
		args = append(args, "--bridge", params.NetworkBridge)
	}
	if params.Memory != 0 {
		args = append(args, "--memory", fmt.Sprint(params.Memory))
	}
	if params.CpuCores != 0 {
		args = append(args, "--cpu", fmt.Sprint(params.CpuCores))
	}
	if params.RootDisk != 0 {
		args = append(args, "--disk", fmt.Sprint(params.RootDisk))
	}
	// TODO add memory, cpu and disk prior to hostname
	args = append(args, params.Hostname)
	if params.Series != "" {
		args = append(args, fmt.Sprintf("release=%s", params.Series))
	}
	if params.Arch != "" {
		args = append(args, fmt.Sprintf("arch=%s", params.Arch))
	}
	output, err := run("uvt-kvm", args...)
	logger.Debugf("is this the logged output?:\n%s", output)
	return err
}

// DestroyMachine destroys the virtual machine identified by hostname.
func DestroyMachine(hostname string) error {
	_, err := run("uvt-kvm", "destroy", hostname)
	return err
}

// AutostartMachine indicates that the virtual machines should automatically
// restart when the host restarts.
func AutostartMachine(hostname string) error {
	_, err := run("virsh", "autostart", hostname)
	return err
}

// ListMachines returns a map of machine name to state, where state is one of:
// running, idle, paused, shutdown, shut off, crashed, dying, pmsuspended.
func ListMachines() (map[string]string, error) {
	output, err := run("virsh", "-q", "list", "--all")
	if err != nil {
		return nil, err
	}
	// Split the output into lines.
	// Regex matching is the easiest way to match the lines.
	//   id hostname status
	// separated by whitespace, with whitespace at the start too.
	result := make(map[string]string)
	for _, s := range machineListPattern.FindAllStringSubmatchIndex(output, -1) {
		hostnameAndStatus := machineListPattern.ExpandString(nil, "$hostname $status", output, s)
		parts := strings.SplitN(string(hostnameAndStatus), " ", 2)
		result[parts[0]] = parts[1]
	}
	return result, nil
}
