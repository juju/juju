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

func (s *BaseSuite) NewDoc(wl workload.Info) *workloadDoc {
	return &workloadDoc{
		DocID:  "workload#" + s.Unit.Id() + "#" + wl.ID(),
		UnitID: s.Unit.Id(),

		Name: wl.Name,
		Type: wl.Type,

		PluginID:       wl.Details.ID,
		OriginalStatus: wl.Details.Status.State,

		PluginStatus: wl.Details.Status.State,
	}
}

func (s *BaseSuite) SetDocs(workloads ...workload.Info) []*workloadDoc {
	var results []*workloadDoc
	for _, wl := range workloads {
		workloadDoc := s.NewDoc(wl)
		results = append(results, workloadDoc)
		s.State.SetDocs(workloadDoc)
	}
	return results
}

func (s *BaseSuite) RemoveDoc(wl workload.Info) {
	docID := "workload#" + s.Unit.Id() + "#" + wl.ID()
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
		name, pluginID := workload.SplitID(id)
		if pluginID == "" {
			pluginID = fmt.Sprintf("%s-%s", name, utils.MustNewUUID())
		}

		workloads = append(workloads, workload.Info{
			Workload: charm.Workload{
				Name: name,
				Type: pType,
			},
			Details: workload.Details{
				ID: pluginID,
				Status: workload.PluginStatus{
					State: "running",
				},
			},
		})
	}
	return workloads
}
