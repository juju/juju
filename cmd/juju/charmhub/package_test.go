// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination ./mocks/api_mock.go github.com/juju/juju/cmd/juju/charmhub CharmHubClient
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination ./mocks/os_mock.go github.com/juju/juju/cmd/juju/charmhub OSEnviron
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination ./mocks/fsys_mock.go github.com/juju/juju/cmd/modelcmd Filesystem,ReadSeekCloser

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

func commandContextForTest(c *tc.C) *cmd.Context {
	var stdout, stderr bytes.Buffer
	ctx, err := cmd.DefaultContext()
	c.Assert(err, tc.ErrorIsNil)
	ctx.Stdout = &stdout
	ctx.Stderr = &stderr
	return ctx
}
