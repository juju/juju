// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"fmt"

	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/workload"
)

type BaseSuite struct {
	testing.BaseSuite

	Stub  *gitjujutesting.Stub
	State *fakeStatePersistence
	Unit  names.UnitTag
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.Stub = &gitjujutesting.Stub{}
	s.State = &fakeStatePersistence{Stub: s.Stub}
	s.Unit = names.NewUnitTag("a-unit/0")
}

type WorkloadDoc workloadDoc

func (doc WorkloadDoc) convert() *workloadDoc {
	return (*workloadDoc)(&doc)
}

func (s *BaseSuite) NewDoc(id string, wl workload.Info) *workloadDoc {
	return &workloadDoc{
		DocID:  "workload#" + s.Unit.Id() + "#" + id,
		UnitID: s.Unit.Id(),

		Name: wl.Name,
		Type: wl.Type,

		PluginID:       wl.Details.ID,
		OriginalStatus: wl.Details.Status.State,

		PluginStatus: wl.Details.Status.State,
	}
}

func (s *BaseSuite) SetDoc(id string, wl workload.Info) *workloadDoc {
	workloadDoc := s.NewDoc(id, wl)
	s.State.SetDocs(workloadDoc)
	return workloadDoc
}

func (s *BaseSuite) RemoveDoc(id string) {
	docID := "workload#" + s.Unit.Id() + "#" + id
	delete(s.State.docs, docID)
}

func (s *BaseSuite) NewPersistence() *Persistence {
	return NewPersistence(s.State, s.Unit)
}

func (s *BaseSuite) SetUnit(id string) {
	if id == "" {
		s.Unit = names.UnitTag{}
	} else {
		s.Unit = names.NewUnitTag(id)
	}
}

func (s *BaseSuite) NewWorkloads(pType string, ids ...string) []workload.Info {
	var workloads []workload.Info
	for _, id := range ids {
		wl := s.NewWorkload(pType, id)
		workloads = append(workloads, wl)
	}
	return workloads
}

func (s *BaseSuite) NewWorkload(pType string, id string) workload.Info {
	name, pluginID := workload.ParseID(id)
	if pluginID == "" {
		pluginID = fmt.Sprintf("%s-%s", name, utils.MustNewUUID())
	}

	return workload.Info{
		PayloadClass: charm.PayloadClass{
			Name: name,
			Type: pType,
		},
		Details: workload.Details{
			ID: pluginID,
			Status: workload.PluginStatus{
				State: "running",
			},
		},
	}
}
