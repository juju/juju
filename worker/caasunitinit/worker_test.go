// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitinit_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasoperator/mocks"
	"github.com/juju/juju/worker/caasunitinit"
)

type UnitInitWorkerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&UnitInitWorkerSuite{})

func (s *UnitInitWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *UnitInitWorkerSuite) TestWorker(c *gc.C) {
	startChan := make(chan []string, 1)
	startChan <- []string{"gitlab/0"}
	close(startChan)
	client := &fakeClient{
		ContainerStartWatcher: watchertest.NewMockStringsWatcher(startChan),
	}

	st := &testing.Stub{}
	cfg := caasunitinit.Config{
		Logger: loggo.GetLogger("test"),
		NewExecClient: func() (exec.Executor, error) {
			return &mocks.MockExecutor{}, nil
		},
		UnitProviderIDFunc: func(unit names.UnitTag) (string, error) {
			return "", errors.NotImplementedf("not required")
		},
		InitializeUnit: func(params caasunitinit.InitializeUnitParams, cancel <-chan struct{}) error {
			st.AddCall("InitializeUnit", params.UnitTag)
			return nil
		},
		ContainerStartWatcher: client,
	}

	w, err := caasunitinit.NewWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-worker.Dead(w):
	case <-time.After(coretesting.LongWait):
	}
	err = workertest.CheckKill(c, w)
	c.Assert(err, gc.ErrorMatches, "watcher closed channel")

	st.CheckCall(c, 0, "InitializeUnit", names.NewUnitTag("gitlab/0"))
}

func (s *UnitInitWorkerSuite) TestUnitDeadGraceful(c *gc.C) {
	startChan := make(chan []string, 1)
	startChan <- []string{"gitlab/0"}
	close(startChan)
	client := &fakeClient{
		ContainerStartWatcher: watchertest.NewMockStringsWatcher(startChan),
	}

	st := &testing.Stub{}
	cfg := caasunitinit.Config{
		Logger: loggo.GetLogger("test"),
		NewExecClient: func() (exec.Executor, error) {
			return &mocks.MockExecutor{}, nil
		},
		UnitProviderIDFunc: func(unit names.UnitTag) (string, error) {
			return "", errors.NotImplementedf("not required")
		},
		InitializeUnit: func(params caasunitinit.InitializeUnitParams, cancel <-chan struct{}) error {
			st.AddCall("InitializeUnit", params.UnitTag)
			return errors.NotFoundf("unit sadly died")
		},
		ContainerStartWatcher: client,
	}

	w, err := caasunitinit.NewWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-worker.Dead(w):
	case <-time.After(coretesting.LongWait):
	}
	err = workertest.CheckKill(c, w)
	c.Assert(err, gc.ErrorMatches, "watcher closed channel")

	st.CheckCall(c, 0, "InitializeUnit", names.NewUnitTag("gitlab/0"))
}

func (s *UnitInitWorkerSuite) TestInitializeFailed(c *gc.C) {
	startChan := make(chan []string, 1)
	startChan <- []string{"gitlab/0"}
	close(startChan)
	client := &fakeClient{
		ContainerStartWatcher: watchertest.NewMockStringsWatcher(startChan),
	}

	st := &testing.Stub{}
	cfg := caasunitinit.Config{
		Logger: loggo.GetLogger("test"),
		NewExecClient: func() (exec.Executor, error) {
			return &mocks.MockExecutor{}, nil
		},
		UnitProviderIDFunc: func(unit names.UnitTag) (string, error) {
			return "", errors.NotImplementedf("not required")
		},
		InitializeUnit: func(params caasunitinit.InitializeUnitParams, cancel <-chan struct{}) error {
			st.AddCall("InitializeUnit", params.UnitTag)
			return errors.Errorf("something bad happened")
		},
		ContainerStartWatcher: client,
	}

	w, err := caasunitinit.NewWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-worker.Dead(w):
	case <-time.After(coretesting.LongWait):
	}
	err = workertest.CheckKill(c, w)
	c.Assert(err, gc.ErrorMatches, `initializing unit "gitlab/0": something bad happened`)

	st.CheckCall(c, 0, "InitializeUnit", names.NewUnitTag("gitlab/0"))
}
