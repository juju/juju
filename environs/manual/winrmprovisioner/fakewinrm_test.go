// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package winrmprovisioner_test

import (
	"fmt"
	"io"
)

const (
	winrmListenerAddr = "127.0.0.1"
)

var (
	NoValue = fmt.Errorf("No Value")
)

type fakeWinRM struct {
	password string
	err      error
	fakePing func() error
	fakeRun  func(cmd string, stdout, stderr io.Writer) error
}

func (f *fakeWinRM) Ping() {
	f.err = f.fakePing()
}

func (f *fakeWinRM) Run(cmd string, stdout io.Writer, stderr io.Writer) {
	f.err = f.fakeRun(cmd, stdout, stderr)
}

func (f fakeWinRM) Error() error {
	return f.err
}

func (f fakeWinRM) Password() string {
	return f.password
}

func (f fakeWinRM) Secure() bool {
	return false
}
