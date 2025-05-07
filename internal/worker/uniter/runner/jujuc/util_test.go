// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"bytes"
	"fmt"
	"io"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc/jujuctesting"
)

const (
	formatYaml = iota
	formatJson
)

func bufferBytes(stream io.Writer) []byte {
	return stream.(*bytes.Buffer).Bytes()
}

func bufferString(w io.Writer) string {
	return w.(*bytes.Buffer).String()
}

type ContextSuite struct {
	jujuctesting.ContextSuite
	testing.BaseSuite
}

func (s *ContextSuite) SetUpTest(c *tc.C) {
	s.ContextSuite.SetUpTest(c)
	s.BaseSuite.SetUpTest(c)
}

func (s *ContextSuite) newHookContext(c *tc.C) *Context {
	hctx, info := s.ContextSuite.NewHookContext()
	return &Context{
		Context: hctx,
		info:    info,
	}
}

func (s *ContextSuite) GetHookContext(c *tc.C, relid int, remote string) *Context {
	c.Assert(relid, tc.Equals, -1)
	return s.newHookContext(c)
}

func (s *ContextSuite) GetStatusHookContext(c *tc.C) *Context {
	return s.newHookContext(c)
}

type Context struct {
	jujuc.Context
	info *jujuctesting.ContextInfo

	rebootPriority jujuc.RebootPriority
	shouldError    bool
}

func (c *Context) RequestReboot(priority jujuc.RebootPriority) error {
	c.rebootPriority = priority
	if c.shouldError {
		return fmt.Errorf("RequestReboot error!")
	} else {
		return nil
	}
}
