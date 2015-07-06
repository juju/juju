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
	Charm names.CharmTag
	Unit  names.UnitTag
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.Stub = &gitjujutesting.Stub{}
	s.State = &fakeStatePersistence{Stub: s.Stub}
	s.Charm = names.NewCharmTag("local:series/dummy-1")
	s.Unit = names.NewUnitTag("a-unit/0")
}

type ProcessInfoDocs struct {
	Definition *definitionDoc
	Launch     *launchDoc
	Proc       *processDoc
	Docs       []interface{}
}

type DefinitionDoc struct {
	DocID   string
	DocKind string
	UnitID  string
	Name    string
	Type    string
}

func (doc DefinitionDoc) convert() interface{} {
	return &definitionDoc{
		DocID:   doc.DocID,
		DocKind: doc.DocKind,
		UnitID:  doc.UnitID,
		Name:    doc.Name,
		Type:    doc.Type,
	}
}

type LaunchDoc struct {
	DocID     string
	DocKind   string
	PluginID  string
	RawStatus string
}

func (doc LaunchDoc) convert() interface{} {
	return &launchDoc{
		DocID:     doc.DocID,
		DocKind:   doc.DocKind,
		PluginID:  doc.PluginID,
		RawStatus: doc.RawStatus,
	}
}

type ProcessDoc struct {
	DocID        string
	DocKind      string
	Life         int
	PluginStatus string
}

func (doc ProcessDoc) convert() interface{} {
	return &processDoc{
		DocID:        doc.DocID,
		DocKind:      doc.DocKind,
		Life:         Life(doc.Life),
		PluginStatus: doc.PluginStatus,
	}
}

func (s *BaseSuite) NewDocs(proc process.Info) ProcessInfoDocs {
	docs := ProcessInfoDocs{}

	docs.Definition = &definitionDoc{
		DocID:   "c#" + s.Charm.Id() + "#" + proc.Name,
		DocKind: "definition",
		UnitID:  s.Unit.Id(),
		Name:    proc.Name,
		Type:    proc.Type,
	}
	docs.Docs = append(docs.Docs, docs.Definition)

	if proc.Details.ID != "" {
		docs.Launch = &launchDoc{
			DocID:     "u#" + s.Unit.Id() + "#charm#" + proc.ID() + "#launch",
			DocKind:   "launch",
			PluginID:  proc.Details.ID,
			RawStatus: proc.Details.Status.Label,
		}
		docs.Proc = &processDoc{
			DocID:        "u#" + s.Unit.Id() + "#charm#" + proc.ID(),
			DocKind:      "process",
			Life:         0,
			PluginStatus: proc.Details.Status.Label,
		}
		docs.Docs = append(docs.Docs, docs.Launch, docs.Proc)
	}

	return docs
}

func (s *BaseSuite) SetDocs(procs ...process.Info) []ProcessInfoDocs {
	var results []ProcessInfoDocs
	for _, proc := range procs {
		procDocs := s.NewDocs(proc)
		results = append(results, procDocs)
		s.State.SetDocs(procDocs.Docs...)
	}
	return results
}

func (s *BaseSuite) RemoveDoc(proc process.Info, kind string) {
	var docID string
	switch kind {
	case "definition":
		docID = "c#" + s.Charm.Id() + "#" + proc.Name
	case "launch":
		docID = "u#" + s.Unit.Id() + "#charm#" + proc.ID() + "#launch"
	case "process":
		docID = "u#" + s.Unit.Id() + "#charm#" + proc.ID()
	}
	delete(s.State.docs, docID)
}

func (s *BaseSuite) NewPersistence() *Persistence {
	return NewPersistence(s.State, &s.Charm, &s.Unit)
}

func (s *BaseSuite) SetUnit(id string) {
	if id == "" {
		s.Unit = names.UnitTag{}
	} else {
		s.Unit = names.NewUnitTag(id)
	}
}

func (s *BaseSuite) SetCharm(id string) {
	if id == "" {
		s.Charm = names.CharmTag{}
	} else {
		s.Charm = names.NewCharmTag(id)
	}
}

func (s *BaseSuite) NewDefinitions(pType string, names ...string) []charm.Process {
	var definitions []charm.Process
	for _, name := range names {
		definitions = append(definitions, charm.Process{
			Name: name,
			Type: pType,
		})
	}
	return definitions
}

func (s *BaseSuite) NewProcesses(pType string, names ...string) []process.Info {
	var ids []string
	for i, name := range names {
		name, id := process.ParseID(name)
		names[i] = name
		if id == "" {
			id = fmt.Sprintf("%s-%s", name, utils.MustNewUUID())
		}
		ids = append(ids, id)
	}

	var processes []process.Info
	for i, definition := range s.NewDefinitions(pType, names...) {
		id := ids[i]
		processes = append(processes, process.Info{
			Process: definition,
			Details: process.Details{
				ID: id,
				Status: process.Status{
					Label: "running",
				},
			},
		})
	}
	return processes
}
