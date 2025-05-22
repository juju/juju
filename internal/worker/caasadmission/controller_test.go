// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission_test

import (
	"context"
	"net/http"
	"sync"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/caasadmission"
	rbacmappertest "github.com/juju/juju/internal/worker/caasrbacmapper/test"
)

type ControllerSuite struct {
}

type dummyMux struct {
	AddHandlerFunc    func(string, string, http.Handler) error
	RemoveHandlerFunc func(string, string)
}

func TestControllerSuite(t *stdtesting.T) {
	tc.Run(t, &ControllerSuite{})
}

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

func (s *ControllerSuite) TestControllerStartup(c *tc.C) {
	var (
		logger     = loggertesting.WrapCheckLog(c)
		rbacMapper = &rbacmappertest.Mapper{}
		waitGroup  = sync.WaitGroup{}
		path       = "/test"
	)
	// Setup function counter
	waitGroup.Add(2)
	mux := &dummyMux{
		AddHandlerFunc: func(m, p string, _ http.Handler) error {
			c.Assert(m, tc.DeepEquals, http.MethodPost)
			c.Assert(p, tc.DeepEquals, path)
			waitGroup.Done()
			return nil
		},
		RemoveHandlerFunc: func(_, _ string) {
			waitGroup.Done()
		},
	}
	creator := &dummyAdmissionCreator{
		EnsureMutatingWebhookConfigurationFunc: func(_ context.Context) (func(), error) {
			waitGroup.Done()
			return func() { waitGroup.Done() }, nil
		},
	}

	ctrl, err := caasadmission.NewController(logger, mux, path, constants.LabelVersion1, creator, rbacMapper)
	c.Assert(err, tc.ErrorIsNil)

	waitGroup.Wait()
	waitGroup.Add(2)
	ctrl.Kill()

	// Cleanup function counter
	waitGroup.Wait()
	err = ctrl.Wait()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ControllerSuite) TestControllerStartupMuxError(c *tc.C) {
	var (
		logger     = loggertesting.WrapCheckLog(c)
		rbacMapper = &rbacmappertest.Mapper{}
		waitGroup  = sync.WaitGroup{}
		path       = "/test"
	)
	// Setup function counter
	waitGroup.Add(1)
	mux := &dummyMux{
		AddHandlerFunc: func(m, p string, _ http.Handler) error {
			waitGroup.Done()
			c.Assert(m, tc.DeepEquals, http.MethodPost)
			c.Assert(p, tc.DeepEquals, path)
			return errors.NewNotValid(nil, "not valid")
		},
	}
	creator := &dummyAdmissionCreator{}

	ctrl, err := caasadmission.NewController(logger, mux, path, constants.LabelVersion1, creator, rbacMapper)
	c.Assert(err, tc.ErrorIsNil)

	waitGroup.Wait()
	ctrl.Kill()
	err = ctrl.Wait()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *ControllerSuite) TestControllerStartupAdmissionError(c *tc.C) {
	var (
		logger     = loggertesting.WrapCheckLog(c)
		rbacMapper = &rbacmappertest.Mapper{}
		waitGroup  = sync.WaitGroup{}
		path       = "/test"
	)
	// Setup function counter
	waitGroup.Add(1)
	mux := &dummyMux{}
	creator := &dummyAdmissionCreator{
		EnsureMutatingWebhookConfigurationFunc: func(_ context.Context) (func(), error) {
			waitGroup.Done()
			return func() {}, errors.NewNotValid(nil, "not valid")
		},
	}

	ctrl, err := caasadmission.NewController(logger, mux, path, constants.LabelVersion1, creator, rbacMapper)
	c.Assert(err, tc.ErrorIsNil)

	waitGroup.Wait()
	ctrl.Kill()
	err = ctrl.Wait()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}
