// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"
	"testing"

	"github.com/juju/cmd/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination ./mocks/api_mock.go github.com/juju/juju/cmd/juju/charmhub CharmHubClient
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination ./mocks/os_mock.go github.com/juju/juju/cmd/juju/charmhub OSEnviron
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination ./mocks/fsys_mock.go github.com/juju/juju/cmd/modelcmd Filesystem,ReadSeekCloser

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func commandContextForTest(c *gc.C) *cmd.Context {
	var stdout, stderr bytes.Buffer
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	ctx.Stdout = &stdout
	ctx.Stderr = &stderr
	return ctx
}
