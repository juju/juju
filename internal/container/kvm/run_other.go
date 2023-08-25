// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !linux

package kvm

import "github.com/juju/errors"

func runAsLibvirt(_, _ string, _ ...string) (string, error) {
	return "", errors.New("kvm is only supported on linux amd64, arm64, and ppc64el")
}

func getUserUIDGID(_ string) (int, int, error) {
	return -1, -1, errors.New("kvm is only supported on linux amd64, arm64, and ppc64el")
}
