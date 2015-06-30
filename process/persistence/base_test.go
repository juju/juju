// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence_test

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

type baseProcessesSuite struct {
	testing.BaseSuite

	stub  *gitjujutesting.Stub
	charm names.CharmTag
	unit  names.UnitTag
}

func (s *baseProcessesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &gitjujutesting.Stub{}
	s.charm = names.NewCharmTag("local:series/dummy-1")
	s.unit = names.NewUnitTag("a-unit/0")
}

func (s *baseProcessesSuite) setUnit(id string) {
	if id == "" {
		s.unit = names.UnitTag{}
	} else {
		s.unit = names.NewUnitTag(id)
	}
}

func (s *baseProcessesSuite) setCharm(id string) {
	if id == "" {
		s.charm = names.CharmTag{}
	} else {
		s.charm = names.NewCharmTag(id)
	}
}

func (s *baseProcessesSuite) newDefinitions(pType string, names ...string) []charm.Process {
	var definitions []charm.Process
	for _, name := range names {
		definitions = append(definitions, charm.Process{
			Name: name,
			Type: pType,
		})
	}
	return definitions
}

func (s *baseProcessesSuite) newProcesses(pType string, names ...string) []process.Info {
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
	for i, definition := range s.newDefinitions(pType, names...) {
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
