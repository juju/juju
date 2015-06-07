// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"

	"github.com/juju/testing"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
	jujuctesting "github.com/juju/juju/worker/uniter/runner/jujuc/testing"
)

type relationSuite struct {
	ContextSuite
}

func (s *relationSuite) newHookContext(relid int, remote string) (jujuc.Context, *relationInfo) {
	hctx, info := s.NewHookContext()
	rInfo := &relationInfo{ContextInfo: info, stub: s.Stub}
	settings := jujuctesting.Settings{
		"private-address": "u-0.testing.invalid",
	}
	rInfo.setNextRelation(s.Unit, settings) // peer0
	rInfo.setNextRelation(s.Unit, settings) // peer1
	if relid >= 0 {
		rInfo.SetAsRelationHook(relid, remote)
	}

	return hctx, rInfo
}

type relationInfo struct {
	*jujuctesting.ContextInfo

	stub *testing.Stub
	rels map[int]*jujuctesting.Relation
}

func (ri *relationInfo) setNextRelation(unit string, settings jujuctesting.Settings) int {
	if ri.rels == nil {
		ri.rels = make(map[int]*jujuctesting.Relation)
	}
	id := len(ri.rels)
	name := fmt.Sprintf("peer%d", id)
	relation := ri.SetNewRelation(id, name, ri.stub)
	relation.UnitName = unit
	relation.SetRelated(unit, settings)
	ri.rels[id] = relation
	return id
}

func (ri *relationInfo) addRelatedServices(relname string, count int) {
	for i := 0; i < count; i++ {
		id := len(ri.rels)
		ri.Relations.SetNewRelation(id, relname, ri.stub)
	}
}

func (ri *relationInfo) setRelations(id int, members []string) {
	relation := ri.rels[id]
	for _, name := range members {
		relation.SetRelated(name, nil)
	}
}
