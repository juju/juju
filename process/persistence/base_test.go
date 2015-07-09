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

	"github.com/juju/juju/process"
	"github.com/juju/juju/testing"
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

type ProcessDoc processDoc

func (doc ProcessDoc) convert() *processDoc {
	return (*processDoc)(&doc)
}

func (s *BaseSuite) NewDoc(proc process.Info) *processDoc {
	return &processDoc{
		DocID:  "proc#" + s.Unit.Id() + "#" + proc.ID(),
		UnitID: s.Unit.Id(),

		Name: proc.Name,
		Type: proc.Type,

		PluginID:       proc.Details.ID,
		OriginalStatus: proc.Details.Status.Label,

		PluginStatus: proc.Details.Status.Label,
	}
}

func (s *BaseSuite) SetDocs(procs ...process.Info) []*processDoc {
	var results []*processDoc
	for _, proc := range procs {
		procDoc := s.NewDoc(proc)
		results = append(results, procDoc)
		s.State.SetDocs(procDoc)
	}
	return results
}

func (s *BaseSuite) RemoveDoc(proc process.Info) {
	docID := "proc#" + s.Unit.Id() + "#" + proc.ID()
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

func (s *BaseSuite) NewProcesses(pType string, ids ...string) []process.Info {
	var processes []process.Info
	for _, id := range ids {
		name, pluginID := process.ParseID(id)
		if pluginID == "" {
			pluginID = fmt.Sprintf("%s-%s", name, utils.MustNewUUID())
		}

		processes = append(processes, process.Info{
			Process: charm.Process{
				Name: name,
				Type: pType,
			},
			Details: process.Details{
				ID: pluginID,
				Status: process.PluginStatus{
					Label: "running",
				},
			},
		})
	}
	return processes
}
