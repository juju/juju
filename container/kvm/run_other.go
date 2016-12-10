// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !linux
// +build !amd64 !arm64 !ppc64el

package kvm

import "github.com/juju/errors"

func runAsLibvirt(commands string, args ...string) (string, error) {
	return "", errors.New("kvm support is only available on linux amd64, arm64, and ppc64el")
}
