// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !linux

package kvm

import "github.com/juju/errors"

func runAsLibvirt(commands string, args ...string) (string, error) {
	return "", errors.New("kvm support is only available on linux amd64, arm64, and ppc64el")
}

func getUserUIDGID(_ string) (int, int, error) {
	return -1, -1, errors.New("kvm support is only available on linux amd64, arm64, and ppc64el")
}
