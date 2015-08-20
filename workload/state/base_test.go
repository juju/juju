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

	"github.com/juju/juju/testing"
	"github.com/juju/juju/workload"
)

type baseWorkloadsSuite struct {
	testing.BaseSuite

	stub    *gitjujutesting.Stub
	persist *fakeWorkloadsPersistence
}

func (s *baseWorkloadsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &gitjujutesting.Stub{}
	s.persist = &fakeWorkloadsPersistence{Stub: s.stub}
}

func (s *baseWorkloadsSuite) newWorkloads(pType string, ids ...string) []workload.Info {
	var workloads []workload.Info
	for _, id := range ids {
		name, pluginID := workload.ParseID(id)
		if pluginID == "" {
			pluginID = fmt.Sprintf("%s-%s", name, utils.MustNewUUID())
		}

		workloads = append(workloads, workload.Info{
			Workload: charm.Workload{
				Name: name,
				Type: pType,
			},
			Status: workload.Status{
				State: workload.StateRunning,
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

type fakeWorkloadsPersistence struct {
	*gitjujutesting.Stub
	workloads map[string]*workload.Info
}

func (s *fakeWorkloadsPersistence) checkWorkloads(c *gc.C, expectedList []workload.Info) {
	c.Check(s.workloads, gc.HasLen, len(expectedList))
	for _, expected := range expectedList {
		wl, ok := s.workloads[expected.ID()]
		if !ok {
			c.Errorf("workload %q not found", expected.ID())
		} else {
			c.Check(wl, jc.DeepEquals, &expected)
		}
	}
}

func (s *fakeWorkloadsPersistence) setWorkloads(workloads ...*workload.Info) {
	if s.workloads == nil {
		s.workloads = make(map[string]*workload.Info)
	}
	for _, wl := range workloads {
		s.workloads[wl.ID()] = wl
	}
}

func (s *fakeWorkloadsPersistence) Insert(info workload.Info) (bool, error) {
	s.AddCall("Insert", info)
	if err := s.NextErr(); err != nil {
		return false, errors.Trace(err)
	}

	if _, ok := s.workloads[info.ID()]; ok {
		return false, nil
	}
	s.setWorkloads(&info)
	return true, nil
}

func (s *fakeWorkloadsPersistence) SetStatus(id string, status workload.CombinedStatus) (bool, error) {
	s.AddCall("SetStatus", id, status)
	if err := s.NextErr(); err != nil {
		return false, errors.Trace(err)
	}

	wl, ok := s.workloads[id]
	if !ok {
		return false, nil
	}
	wl.Status = status.Status
	wl.Details.Status = status.PluginStatus
	return true, nil
}

func (s *fakeWorkloadsPersistence) List(ids ...string) ([]workload.Info, []string, error) {
	s.AddCall("List", ids)
	if err := s.NextErr(); err != nil {
		return nil, nil, errors.Trace(err)
	}

	var workloads []workload.Info
	var missing []string
	for _, id := range ids {
		if wl, ok := s.workloads[id]; !ok {
			missing = append(missing, id)
		} else {
			workloads = append(workloads, *wl)
		}
	}
	return workloads, missing, nil
}

func (s *fakeWorkloadsPersistence) ListAll() ([]workload.Info, error) {
	s.AddCall("ListAll")
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var workloads []workload.Info
	for _, wl := range s.workloads {
		workloads = append(workloads, *wl)
	}
	return workloads, nil
}

func (s *fakeWorkloadsPersistence) Remove(id string) (bool, error) {
	s.AddCall("Remove", id)
	if err := s.NextErr(); err != nil {
		return false, errors.Trace(err)
	}

	if _, ok := s.workloads[id]; !ok {
		return false, nil
	}
	delete(s.workloads, id)
	return true, nil
}
