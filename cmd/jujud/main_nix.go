// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.
// +build !windows

package main

func serviceWrapper(f func() int) (int, error) {
	code := f()
	return code, nil
}
