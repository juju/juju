// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"

	"github.com/juju/names/v6"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc/jujuctesting"
)

type relationSuite struct {
	ContextSuite
}

func (s *relationSuite) newHookContext(relid int, remote string, app string) (jujuc.Context, *relationInfo) {
	hctx, info := s.ContextSuite.NewHookContext()
	rInfo := &relationInfo{ContextInfo: info, stub: s.Stub}
	settings := jujuctesting.Settings{
		"private-address": "u-0.testing.invalid",
	}
	rInfo.setNextRelation("", s.Unit, app, settings) // peer0
	rInfo.setNextRelation("", s.Unit, app, settings) // peer1
	if relid >= 0 {
		rInfo.SetAsRelationHook(relid, remote)
		if app == "" {
			maybeApp, err := names.UnitApplication(remote)
			if err == nil {
				app = maybeApp
			}
		}
	}
	rInfo.SetRemoteApplicationName(app)

	return hctx, rInfo
}

type relationInfo struct {
	*jujuctesting.ContextInfo

	stub *testhelpers.Stub
	rels map[int]*jujuctesting.Relation
}

func (ri *relationInfo) reset() {
	ri.Relations.Reset()
	ri.RelationHook.Reset()
	ri.rels = nil
}

func (ri *relationInfo) setNextRelation(name, unit, app string, settings jujuctesting.Settings) int {
	if ri.rels == nil {
		ri.rels = make(map[int]*jujuctesting.Relation)
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
	relation.RemoteApplicationName = app
	ri.rels[id] = relation
	return id
}

func (ri *relationInfo) addRelatedApplications(relname string, count int) {
	if ri.rels == nil {
		ri.rels = make(map[int]*jujuctesting.Relation)
	}
	for i := 0; i < count; i++ {
		ri.setNextRelation(relname, "", ri.RemoteApplicationName, nil)
	}
}

func (ri *relationInfo) setRelations(id int, members []string) {
	relation := ri.rels[id]
	relation.Reset()
	for _, name := range members {
		relation.SetRelated(name, nil)
	}
}
