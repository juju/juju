// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
	"github.com/juju/juju/testing"
)

type baseProcessesSuite struct {
	testing.BaseSuite

	stub    *gitjujutesting.Stub
	persist *fakeProcsPersistence
}

func (s *baseProcessesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &gitjujutesting.Stub{}
	s.persist = &fakeProcsPersistence{Stub: s.stub}
}

func (s *baseProcessesSuite) newProcesses(pType string, ids ...string) []process.Info {
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
			Status: process.Status{
				State: process.StateRunning,
			},
			Details: process.Details{
				ID: pluginID,
				Status: process.PluginStatus{
					State: "running",
				},
			},
		})
	}
	return processes
}

type fakeProcsPersistence struct {
	*gitjujutesting.Stub
	procs map[string]*process.Info
}

func (s *fakeProcsPersistence) checkProcesses(c *gc.C, expectedList []process.Info) {
	c.Check(s.procs, gc.HasLen, len(expectedList))
	for _, expected := range expectedList {
		proc, ok := s.procs[expected.ID()]
		if !ok {
			c.Errorf("process %q not found", expected.ID())
		} else {
			c.Check(proc, jc.DeepEquals, &expected)
		}
	}
}

func (s *fakeProcsPersistence) setProcesses(procs ...*process.Info) {
	if s.procs == nil {
		s.procs = make(map[string]*process.Info)
	}
	for _, proc := range procs {
		s.procs[proc.ID()] = proc
	}
}

func (s *fakeProcsPersistence) Insert(info process.Info) (bool, error) {
	s.AddCall("Insert", info)
	if err := s.NextErr(); err != nil {
		return false, errors.Trace(err)
	}

	if _, ok := s.procs[info.ID()]; ok {
		return false, nil
	}
	s.setProcesses(&info)
	return true, nil
}

func (s *fakeProcsPersistence) SetStatus(id string, status process.CombinedStatus) (bool, error) {
	s.AddCall("SetStatus", id, status)
	if err := s.NextErr(); err != nil {
		return false, errors.Trace(err)
	}

	proc, ok := s.procs[id]
	if !ok {
		return false, nil
	}
	proc.Status = status.Status
	proc.Details.Status = status.PluginStatus
	return true, nil
}

func (s *fakeProcsPersistence) List(ids ...string) ([]process.Info, []string, error) {
	s.AddCall("List", ids)
	if err := s.NextErr(); err != nil {
		return nil, nil, errors.Trace(err)
	}

	var procs []process.Info
	var missing []string
	for _, id := range ids {
		if proc, ok := s.procs[id]; !ok {
			missing = append(missing, id)
		} else {
			procs = append(procs, *proc)
		}
	}
	return procs, missing, nil
}

func (s *fakeProcsPersistence) ListAll() ([]process.Info, error) {
	s.AddCall("ListAll")
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var procs []process.Info
	for _, proc := range s.procs {
		procs = append(procs, *proc)
	}
	return procs, nil
}

func (s *fakeProcsPersistence) Remove(id string) (bool, error) {
	s.AddCall("Remove", id)
	if err := s.NextErr(); err != nil {
		return false, errors.Trace(err)
	}

	if _, ok := s.procs[id]; !ok {
		return false, nil
	}
	delete(s.procs, id)
	return true, nil
}
