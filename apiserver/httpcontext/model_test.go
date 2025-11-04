// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpcontext

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/apiserverhttp"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type ModelHandlersSuite struct {
	testhelpers.IsolationSuite

	controllerModelHandler *ControllerModelHandler
	queryHandler           *QueryModelHandler

	server *httptest.Server
}

func TestModelHandlersSuite(t *testing.T) {
	tc.Run(t, &ModelHandlersSuite{})
}

func (s *ModelHandlersSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelUUID, _ := RequestModelUUID(r.Context())
		io.WriteString(w, modelUUID)
	})
	s.controllerModelHandler = &ControllerModelHandler{
		Handler:             h,
		ControllerModelUUID: coremodel.UUID(coretesting.ModelTag.Id()),
	}
	s.queryHandler = &QueryModelHandler{
		Handler: h,
		Query:   "modeluuid",
	}
	mux := apiserverhttp.NewMux()
	mux.AddHandler("GET", "/query", s.queryHandler)
	mux.AddHandler("GET", "/controller", s.controllerModelHandler)
	s.server = httptest.NewServer(mux)
}

func (s *ModelHandlersSuite) TestControllerUUID(c *tc.C) {
	resp, err := s.server.Client().Get(s.server.URL + "/controller")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(out), tc.Equals, coretesting.ModelTag.Id())
}

func (s *ModelHandlersSuite) TestQuery(c *tc.C) {
	resp, err := s.server.Client().Get(s.server.URL + "/query?modeluuid=" + coretesting.ModelTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(out), tc.Equals, coretesting.ModelTag.Id())
}

func (s *ModelHandlersSuite) TestQueryInvalidModelUUID(c *tc.C) {
	resp, err := s.server.Client().Get(s.server.URL + "/query?modeluuid=zing")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusBadRequest)
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(out), tc.Equals, `invalid model UUID "zing"`+"\n")
}

// TestSetIsControllerModelOnContext verifies that when the is controller model
// flag has been set on a context true is reported when asking the context if
// the request is for the controller model.
func (*ModelHandlersSuite) TestSetIsControllerModelOnContext(c *tc.C) {
	ctx := SetContextIsControllerModel(c.Context())
	c.Check(RequestIsForControllerModel(ctx), tc.IsTrue)
}

// TestIsControllerModelNotSetOnContext verifies that if a context has not had
// the is controller model key set on it false is returned when reporting if
// the request context is for the controller model.
func (*ModelHandlersSuite) TestIsControllerModelNotSetOnContext(c *tc.C) {
	c.Check(RequestIsForControllerModel(c.Context()), tc.IsFalse)
}

// TestIsControllerModelKeyBadValue verifies that should the
// [isControllerModelKey] ever be set on a context with a value that is not a
// bool [RequestIsForControllerModel] returns false.
//
// This is a sanity check to make sure that even when we do the wrong thing we
// adhere to the contract of [RequestIsForControllerModel].
func (*ModelHandlersSuite) TestIsControllerModelKeyBadValue(c *tc.C) {
	ctx := context.WithValue(c.Context(), isControllerModelKey{}, "true")
	c.Check(RequestIsForControllerModel(ctx), tc.IsFalse)
}

// TestControllerModelSignalHandlerIsControllerModel tests that
// [ControllerModelSignalHandler] sets the is controller model flag on the
// context when the request model is equal to that of the controller model.
func (*ModelHandlersSuite) TestControllerModelSignalHandlerIsControllerModel(c *tc.C) {
	ctx := c.Context()
	controllerModelUUID := tc.Must(c, coremodel.NewUUID)
	ctx = SetContextModelUUID(ctx, controllerModelUUID)

	var nextHandlerCalled bool
	var nextHandlerFunc http.HandlerFunc = func(
		_ http.ResponseWriter, r *http.Request,
	) {
		c.Check(RequestIsForControllerModel(r.Context()), tc.IsTrue)
		nextHandlerCalled = true
	}

	handler := ControllerModelSignalHandler{
		ControllerModelUUID: controllerModelUUID,
		Handler:             nextHandlerFunc,
	}

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/foo", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)
	c.Check(nextHandlerCalled, tc.IsTrue)
}

// TestControllerModelSignalHandlerIsNotControllerModel tests that
// [ControllerModelSignalHandler] does not set the is controller model flag on
// requests that are not for the controller model.
func (*ModelHandlersSuite) TestControllerModelSignalHandlerIsNotControllerModel(c *tc.C) {
	ctx := c.Context()
	controllerModelUUID := tc.Must(c, coremodel.NewUUID)
	modelUUID := tc.Must(c, coremodel.NewUUID)
	ctx = SetContextModelUUID(ctx, modelUUID) // Not the controller model uuid

	var nextHandlerCalled bool
	var nextHandlerFunc http.HandlerFunc = func(
		_ http.ResponseWriter, r *http.Request,
	) {
		c.Check(RequestIsForControllerModel(r.Context()), tc.IsFalse)
		nextHandlerCalled = true
	}

	handler := ControllerModelSignalHandler{
		ControllerModelUUID: controllerModelUUID,
		Handler:             nextHandlerFunc,
	}

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/foo", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)
	c.Check(nextHandlerCalled, tc.IsTrue)
}
