// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd

import (
	"os"

	"github.com/juju/testing"
)

type StubFileOps struct {
	*testing.Stub
}

func (sfo *StubFileOps) RemoveAll(name string) error {
	sfo.AddCall("RemoveAll", name)

	return sfo.NextErr()
}

func (sfo *StubFileOps) MkdirAll(dirname string) error {
	sfo.AddCall("MkdirAll", dirname)

	return sfo.NextErr()
}

func (sfo *StubFileOps) CreateFile(filename string, data []byte, perm os.FileMode) error {
	sfo.AddCall("CreateFile", filename, data, perm)

	return sfo.NextErr()
}
