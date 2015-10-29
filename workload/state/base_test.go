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

func (s *baseWorkloadsSuite) newWorkload(pType string, id string) workload.Info {
	name, pluginID := workload.ParseID(id)
	if pluginID == "" {
		pluginID = fmt.Sprintf("%s-%s", name, utils.MustNewUUID())
	}

	return workload.Info{
		PayloadClass: charm.PayloadClass{
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
	}
}

type fakeWorkloadsPersistence struct {
	*gitjujutesting.Stub
	workloads map[string]*workload.Info
}

func (s *fakeWorkloadsPersistence) checkWorkload(c *gc.C, id string, expected workload.Info) {
	wl, ok := s.workloads[id]
	if !ok {
		c.Errorf("workload %q not found", id)
	} else {
		c.Check(wl, jc.DeepEquals, &expected)
	}
}

func (s *fakeWorkloadsPersistence) setWorkload(id string, wl *workload.Info) {
	if s.workloads == nil {
		s.workloads = make(map[string]*workload.Info)
	}
	s.workloads[id] = wl
}

func (s *fakeWorkloadsPersistence) Track(id string, info workload.Info) (bool, error) {
	s.AddCall("Track", id, info)
	if err := s.NextErr(); err != nil {
		return false, errors.Trace(err)
	}

	if _, ok := s.workloads[id]; ok {
		return false, nil
	}
	s.setWorkload(id, &info)
	return true, nil
}

func (s *fakeWorkloadsPersistence) SetStatus(id, status string) (bool, error) {
	s.AddCall("SetStatus", id, status)
	if err := s.NextErr(); err != nil {
		return false, errors.Trace(err)
	}

	wl, ok := s.workloads[id]
	if !ok {
		return false, nil
	}
	wl.Status.State = status
	wl.Details.Status.State = status
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

func (s *fakeWorkloadsPersistence) LookUp(name, rawID string) (string, error) {
	s.AddCall("LookUp", name, rawID)
	if err := s.NextErr(); err != nil {
		return "", errors.Trace(err)
	}

	for id, wl := range s.workloads {
		if wl.Name == name && wl.Details.ID == rawID {
			return id, nil
		}
	}
	return "", errors.NotFoundf("doc ID")
}

func (s *fakeWorkloadsPersistence) Untrack(id string) (bool, error) {
	s.AddCall("Untrack", id)
	if err := s.NextErr(); err != nil {
		return false, errors.Trace(err)
	}

	if _, ok := s.workloads[id]; !ok {
		return false, nil
	}
	delete(s.workloads, id)
	return true, nil
}
