// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"bytes"
	"fmt"
	"io"
	"time"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	jujuctesting "github.com/juju/juju/worker/uniter/runner/jujuc/testing"
)

func bufferBytes(stream io.Writer) []byte {
	return stream.(*bytes.Buffer).Bytes()
}

func bufferString(w io.Writer) string {
	return w.(*bytes.Buffer).String()
}

func cmdString(cmd string) string {
	return cmd + jujuc.CmdSuffix
}

type ContextSuite struct {
	jujuctesting.ContextSuite
	testing.BaseSuite

	rels map[int]*jujuctesting.ContextRelation
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

	metrics        []jujuc.Metric
	canAddMetrics  bool
	rebootPriority jujuc.RebootPriority
	shouldError    bool
}

func (c *Context) AddMetric(key, value string, created time.Time) error {
	if !c.canAddMetrics {
		return fmt.Errorf("metrics disabled")
	}
	c.metrics = append(c.metrics, jujuc.Metric{
		Key:   key,
		Value: value,
		Time:  created,
	})
	return c.Context.AddMetric(key, value, created)
}

func (c *Context) RequestReboot(priority jujuc.RebootPriority) error {
	c.rebootPriority = priority
	if c.shouldError {
		return fmt.Errorf("RequestReboot error!")
	} else {
		return nil
	}
}
