// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"

	"github.com/juju/testing"

	"github.com/juju/juju/worker/common/hookcommands"
	hookstesting "github.com/juju/juju/worker/common/hooks/testing"
)

type relationSuite struct {
	hookstesting.ContextSuite
}

func (s *relationSuite) newHookContext(relid int, remote string) (hookcommands.Context, *relationInfo) {
	hctx, info := s.ContextSuite.NewHookContextAndInfo()
	rInfo := &relationInfo{ContextInfo: info, stub: s.Stub}
	settings := hookstesting.Settings{
		"private-address": "u-0.testing.invalid",
	}
	rInfo.setNextRelation("", s.Unit, settings) // peer0
	rInfo.setNextRelation("", s.Unit, settings) // peer1
	if relid >= 0 {
		rInfo.SetAsRelationHook(relid, remote)
	}

	return hctx, rInfo
}

type relationInfo struct {
	*hookstesting.ContextInfo

	stub *testing.Stub
	rels map[int]*hookstesting.Relation
}

func (ri *relationInfo) reset() {
	ri.Relations.Reset()
	ri.RelationHook.Reset()
	ri.rels = nil
}

func (ri *relationInfo) setNextRelation(name, unit string, settings hookstesting.Settings) int {
	if ri.rels == nil {
		ri.rels = make(map[int]*hookstesting.Relation)
	}
	id := len(ri.rels)
	if name == "" {
		name = fmt.Sprintf("peer%d", id)
	}
	relation := ri.SetNewRelation(id, name, ri.stub)
	if unit != "" {
		relation.UnitName = unit
		relation.SetRelated(unit, settings)
	}
	ri.rels[id] = relation
	return id
}

func (ri *relationInfo) addRelatedServices(relname string, count int) {
	if ri.rels == nil {
		ri.rels = make(map[int]*hookstesting.Relation)
	}
	for i := 0; i < count; i++ {
		ri.setNextRelation(relname, "", nil)
	}
}

func (ri *relationInfo) setRelations(id int, members []string) {
	relation := ri.rels[id]
	relation.Reset()
	for _, name := range members {
		relation.SetRelated(name, nil)
	}
}
