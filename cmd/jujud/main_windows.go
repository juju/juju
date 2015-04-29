// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"code.google.com/p/winsvc/svc"

	"github.com/juju/juju/service/windows"
)

func serviceWrapper(f func() int) (int, error) {
	isInteractive, err := svc.IsAnInteractiveSession()
	if err != nil {
		return 1, err
	}

	if isInteractive {
		code := f()
		return code, nil
	}
	s := windows.NewSystemService("jujud", f)
	if err := s.Run(); err != nil {
		return 1, err
	}
	return 0, nil
}
