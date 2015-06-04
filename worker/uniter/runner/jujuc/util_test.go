// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/state"
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

type ContextSuite struct {
	jujuctesting.ContextSuite
	testing.BaseSuite
	rels    map[int]*jujuctesting.ContextRelation
	storage map[names.StorageTag]*jujuctesting.ContextStorageAttachment
}

func (s *ContextSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)
	s.BaseSuite.SetUpTest(c)

	unit := "u/0"
	settings := jujuctesting.Settings{
		"private-address": "u-0.testing.invalid",
	}

	ctx := s.NewHookContext(nil)
	ctx.SetRelation(0, "peer0", unit, settings).UnitName = unit
	ctx.SetRelation(1, "peer1", unit, settings).UnitName = unit
	ctx.SetBlockStorage("data/0", "/dev/sda")

	s.rels = ctx.ContextRelations.Relations.Relations
	s.storage = ctx.ContextStorage.Info.Storage
}

func (s *ContextSuite) relUnits(id int) map[string]jujuctesting.Settings {
	return s.rels[id].Info.Units
}

func (s *ContextSuite) setRelation(id int, name string) {
	s.rels[id] = &jujuctesting.ContextRelation{
		Stub: s.Stub,
		Info: &jujuctesting.Relation{
			Id:   id,
			Name: name,
		},
	}
}

func (s *ContextSuite) newHookContext(c *gc.C) *Context {
	ctx := s.ContextSuite.NewHookContext(nil).Context
	return &Context{Context: *ctx}
}

func (s *ContextSuite) GetHookContext(c *gc.C, relid int, remote string) *Context {
	if relid != -1 {
		_, found := s.rels[relid]
		c.Assert(found, jc.IsTrue)
	}

	ctx := s.HookContext("u/0", charm.Settings{
		"empty":               nil,
		"monsters":            false,
		"spline-reticulation": 45.0,
		"title":               "My Title",
		"username":            "admin001",
	})
	ctx.Info.OwnerTag = "test-owner"
	ctx.Info.AvailabilityZone = "us-east-1a"
	ctx.Info.PublicAddress = "gimli.minecraft.testing.invalid"
	ctx.Info.PrivateAddress = "192.168.0.99"

	ctx.SetAsRelationHook(relid, remote)
	ctx.Info.Relations.Relations = s.rels
	ctx.Info.Storage.Storage = s.storage

	return &Context{Context: *ctx.Context}
}

func (s *ContextSuite) GetStorageHookContext(c *gc.C, storageId string) *Context {
	valid := names.IsValidStorage(storageId)
	c.Assert(valid, jc.IsTrue)
	storageTag := names.NewStorageTag(storageId)
	_, found := s.storage[storageTag]
	c.Assert(found, jc.IsTrue)
	ctx := s.GetHookContext(c, -1, "")
	ctx.ContextStorage.Info.StorageTag = storageTag
	return ctx
}

func (s *ContextSuite) GetStatusHookContext(c *gc.C) *Context {
	return s.newHookContext(c)
}

func (s *ContextSuite) GetStorageAddHookContext(c *gc.C) *Context {
	return s.newHookContext(c)
}

func setSettings(c *gc.C, ru *state.RelationUnit, settings map[string]interface{}) {
	node, err := ru.Settings()
	c.Assert(err, jc.ErrorIsNil)
	for _, k := range node.Keys() {
		node.Delete(k)
	}
	node.Update(settings)
	_, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)
}

type Context struct {
	jujuctesting.Context
	leaderCtx      jujuc.Context
	metrics        []jujuc.Metric
	canAddMetrics  bool
	rebootPriority jujuc.RebootPriority
	shouldError    bool
}

func (c *Context) AddMetric(key, value string, created time.Time) error {
	if !c.canAddMetrics {
		return fmt.Errorf("metrics disabled")
	}
	c.metrics = append(c.metrics, jujuc.Metric{key, value, created})
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

func (c *Context) relUnits(id int) map[string]jujuctesting.Settings {
	rctx := c.ContextRelations.Relations.Relations[id]
	return rctx.Info.Units
}

func (c *Context) setRelations(id int, members []string) {
	rctx := c.ContextRelations.Relations.Relations[id]
	rctx.Info.Units = map[string]jujuctesting.Settings{}
	for _, name := range members {
		rctx.Info.Units[name] = nil
	}
}

func cmdString(cmd string) string {
	return cmd + jujuc.CmdSuffix
}
