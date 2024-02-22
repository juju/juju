// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"bytes"
	"fmt"
	"io"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc/jujuctesting"
	"github.com/juju/juju/testing"
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

func (s *ContextSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)
	s.BaseSuite.SetUpTest(c)
}

func (s *ContextSuite) newHookContext(c *gc.C) *Context {
	hctx, info := s.ContextSuite.NewHookContext()
	return &Context{
		Context: hctx,
		info:    info,
	}
}

func (s *ContextSuite) GetHookContext(c *gc.C, relid int, remote string) *Context {
	c.Assert(relid, gc.Equals, -1)
	return s.newHookContext(c)
}

func (s *ContextSuite) GetStatusHookContext(c *gc.C) *Context {
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
