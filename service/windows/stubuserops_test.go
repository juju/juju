// Copyright 2015 Cloudbase Solutions
// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !linux windows

package windows

import (
	"syscall"

	"github.com/juju/testing"
)

type StubGetPassword struct {
	*testing.Stub

	passwd string
}

func (p *StubGetPassword) SetPasswd(passwd string) {
	p.passwd = passwd
}

func (p *StubGetPassword) GetPassword() (string, error) {
	p.AddCall("getPassword")

	err := p.NextErr()
	if err != nil {
		return "", err
	}
	return p.passwd, nil
}

type StubLogonUser struct {
	*testing.Stub

	passwd string
}

func (p *StubLogonUser) LogonUser(username *uint16, domain *uint16,
	password *uint16, logonType uint32,
	logonProvider uint32) (handle syscall.Handle, err error) {
	p.AddCall("logonUser")

	err = p.NextErr()
	if err != nil {
		return syscall.InvalidHandle, err
	}
	// We don't really care about the handle. We are only interested in the
	// error returned by this function
	return syscall.InvalidHandle, nil
}
