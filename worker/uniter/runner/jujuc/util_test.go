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
	"github.com/juju/juju/storage"
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

	s.rels = map[int]*jujuctesting.ContextRelation{}
	s.setRelation(0, "peer0")
	s.setRelation(1, "peer1")
	for _, rel := range s.rels {
		rel.Info.Units = map[string]jujuctesting.Settings{
			"u/0": {"private-address": "u-0.testing.invalid"},
		}
		rel.Info.UnitName = "u/0"
	}

	storageData0 := names.NewStorageTag("data/0")
	s.storage = map[names.StorageTag]*jujuctesting.ContextStorageAttachment{
		storageData0: &jujuctesting.ContextStorageAttachment{
			Stub: s.Stub,
			Info: &jujuctesting.StorageAttachment{
				storageData0,
				storage.StorageKindBlock,
				"/dev/sda",
			},
		},
	}
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
	ctx := s.ContextSuite.GetHookContext(c, nil)
	return &Context{Context: *ctx}
}

func (s *ContextSuite) GetHookContext(c *gc.C, relid int, remote string) *Context {
	if relid != -1 {
		_, found := s.rels[relid]
		c.Assert(found, jc.IsTrue)
	}

	info := jujuctesting.NewContextInfo()
	info.Name = "u/0"
	info.ConfigSettings = charm.Settings{
		"empty":               nil,
		"monsters":            false,
		"spline-reticulation": 45.0,
		"title":               "My Title",
		"username":            "admin001",
	}
	info.OwnerTag = "test-owner"
	info.AvailabilityZone = "us-east-1a"
	info.PublicAddress = "gimli.minecraft.testing.invalid"
	info.PrivateAddress = "192.168.0.99"
	info.HookRelation = relid
	info.RemoteUnitName = remote
	info.Relations = &jujuctesting.Relations{Relations: s.rels}
	info.Storage.Storage = s.storage

	ctx := s.ContextSuite.GetHookContext(c, info)
	return &Context{Context: *ctx}
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
