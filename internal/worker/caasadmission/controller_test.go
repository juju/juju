// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission_test

import (
	"net/http"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/internal/worker/caasadmission"
	rbacmappertest "github.com/juju/juju/internal/worker/caasrbacmapper/test"
)

type ControllerSuite struct {
	controllerUUID string
	modelUUID      string
	modelName      string
}

type dummyMux struct {
	AddHandlerFunc    func(string, string, http.Handler) error
	RemoveHandlerFunc func(string, string)
}

var _ = gc.Suite(&ControllerSuite{})

func (d *dummyMux) AddHandler(i, j string, h http.Handler) error {
	if d.AddHandlerFunc == nil {
		return nil
	}
	return d.AddHandlerFunc(i, j, h)
}

func (d *dummyMux) RemoveHandler(i, j string) {
	if d.RemoveHandlerFunc != nil {
		d.RemoveHandlerFunc(i, j)
	}
}

func (s *ControllerSuite) SetUpTest(c *gc.C) {
	controllerUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	s.controllerUUID = controllerUUID.String()

	modelUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	s.modelUUID = modelUUID.String()

	s.modelName = "test-model"
}

func (s *ControllerSuite) TestControllerStartup(c *gc.C) {
	var (
		logger     = loggo.Logger{}
		rbacMapper = &rbacmappertest.Mapper{}
		waitGroup  = sync.WaitGroup{}
		path       = "/test"
	)
	// Setup function counter
	waitGroup.Add(2)
	mux := &dummyMux{
		AddHandlerFunc: func(m, p string, _ http.Handler) error {
			c.Assert(m, jc.DeepEquals, http.MethodPost)
			c.Assert(p, jc.DeepEquals, path)
			waitGroup.Done()
			return nil
		},
		RemoveHandlerFunc: func(_, _ string) {
			waitGroup.Done()
		},
	}
	creator := &dummyAdmissionCreator{
		EnsureMutatingWebhookConfigurationFunc: func() (func(), error) {
			waitGroup.Done()
			return func() { waitGroup.Done() }, nil
		},
	}

	ctrl, err := caasadmission.NewController(logger, mux, path, constants.LabelVersion1, creator, rbacMapper, s.controllerUUID, s.modelUUID, s.modelName)
	c.Assert(err, jc.ErrorIsNil)

	waitGroup.Wait()
	waitGroup.Add(2)
	ctrl.Kill()

	// Cleanup function counter
	waitGroup.Wait()
	err = ctrl.Wait()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerSuite) TestControllerStartupMuxError(c *gc.C) {
	var (
		logger     = loggo.Logger{}
		rbacMapper = &rbacmappertest.Mapper{}
		waitGroup  = sync.WaitGroup{}
		path       = "/test"
	)
	// Setup function counter
	waitGroup.Add(1)
	mux := &dummyMux{
		AddHandlerFunc: func(m, p string, _ http.Handler) error {
			waitGroup.Done()
			c.Assert(m, jc.DeepEquals, http.MethodPost)
			c.Assert(p, jc.DeepEquals, path)
			return errors.NewNotValid(nil, "not valid")
		},
	}
	creator := &dummyAdmissionCreator{}

	ctrl, err := caasadmission.NewController(logger, mux, path, constants.LabelVersion1, creator, rbacMapper, s.controllerUUID, s.modelUUID, s.modelName)
	c.Assert(err, jc.ErrorIsNil)

	waitGroup.Wait()
	ctrl.Kill()
	err = ctrl.Wait()
	c.Assert(errors.IsNotValid(err), jc.IsTrue)
}

func (s *ControllerSuite) TestControllerStartupAdmissionError(c *gc.C) {
	var (
		logger     = loggo.Logger{}
		rbacMapper = &rbacmappertest.Mapper{}
		waitGroup  = sync.WaitGroup{}
		path       = "/test"
	)
	// Setup function counter
	waitGroup.Add(1)
	mux := &dummyMux{}
	creator := &dummyAdmissionCreator{
		EnsureMutatingWebhookConfigurationFunc: func() (func(), error) {
			waitGroup.Done()
			return func() {}, errors.NewNotValid(nil, "not valid")
		},
	}

	ctrl, err := caasadmission.NewController(logger, mux, path, constants.LabelVersion1, creator, rbacMapper, s.controllerUUID, s.modelUUID, s.modelName)
	c.Assert(err, jc.ErrorIsNil)

	waitGroup.Wait()
	ctrl.Kill()
	err = ctrl.Wait()
	c.Assert(errors.IsNotValid(err), jc.IsTrue)
}
