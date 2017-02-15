// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package winrmprovisioner_test

import "io"

const (
	winrmListenerAddr = "127.0.0.1"
)

type fakeWinRM struct {
	password string
	fakePing func() error
	fakeRun  func(cmd string, stdout, stderr io.Writer) error
	secure   bool
}

func (f *fakeWinRM) Ping() error {
	return f.fakePing()
}

func (f *fakeWinRM) Run(cmd string, stdout io.Writer, stderr io.Writer) error {
	return f.fakeRun(cmd, stdout, stderr)
}

func (f fakeWinRM) Password() string {
	return f.password
}

func (f fakeWinRM) Secure() bool {
	return f.secure
}
