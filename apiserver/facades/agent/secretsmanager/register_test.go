// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"context"
	"errors"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager/mocks"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network"
	coresecrets "github.com/juju/juju/core/secrets"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

var (
	errOpen = errors.New("dial failed")

	sourceUUID     = "89cbf64e-3708-495f-871f-e58a34c41b33"
	controllerUUID = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	srcAddr        = "1.2.3.4:17070"
	dstAddr        = "7.7.7.7:1234"
	srcCACert      = "src-ca-cert"
	dstCACert      = "redirected-ca-cert"
	redirectAlias  = "redirected-alias"
)

type remoteClientGetterSuite struct {
	testhelpers.IsolationSuite
}

func TestRemoteClientGetterSuite(t *testing.T) {
	tc.Run(t, &remoteClientGetterSuite{})
}

// stubConnection is a non-nil connection value; crossmodelsecrets.NewClient
// only wraps the connection it is passed, calling BestFacadeVersion to
// pick the facade version.
type stubConnection struct {
	api.Connection
}

func (stubConnection) BestFacadeVersion(facadeName string) int {
	return 1
}

func (s *remoteClientGetterSuite) newURI() *coresecrets.URI {
	return &coresecrets.URI{SourceUUID: sourceUUID, ID: "aabbccddeeff00112233"}
}

func (s *remoteClientGetterSuite) expectSourceControllerInfo(controllerAPI *mocks.MockControllerAPIInfoGetter) {
	controllerAPI.EXPECT().ControllerAPIInfoForModels(gomock.Any(), gomock.Any()).Return(params.ControllerAPIInfoResults{
		Results: []params.ControllerAPIInfoResult{{
			Addresses: []string{srcAddr},
			CACert:    srcCACert,
		}},
	}, nil)
}

func (s *remoteClientGetterSuite) newRedirectError() *api.RedirectError {
	return &api.RedirectError{
		Servers: []network.MachineHostPorts{
			network.NewMachineHostPorts(1234, "7.7.7.7"),
		},
		CACert:          dstCACert,
		ControllerTag:   names.NewControllerTag(controllerUUID),
		ControllerAlias: redirectAlias,
	}
}

func (s *remoteClientGetterSuite) TestNoRedirect(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	controllerAPI := mocks.NewMockControllerAPIInfoGetter(ctrl)
	externalControllers := mocks.NewMockExternalControllerUpdater(ctrl)
	s.expectSourceControllerInfo(controllerAPI)

	var opens int
	open := func(_ context.Context, info *api.Info) (api.Connection, error) {
		opens++
		c.Check(info.Addrs, tc.DeepEquals, []string{srcAddr})
		c.Check(info.CACert, tc.Equals, srcCACert)
		c.Check(info.ModelTag, tc.Equals, names.NewModelTag(sourceUUID))
		c.Check(info.Tag, tc.Equals, names.NewUserTag(api.AnonymousUsername))
		return stubConnection{}, nil
	}

	getter := newRemoteSecretsClientGetter(controllerAPI, externalControllers, open, loggertesting.WrapCheckLog(c))
	client, err := getter(c.Context(), s.newURI())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(client, tc.NotNil)
	c.Check(opens, tc.Equals, 1)
}

func (s *remoteClientGetterSuite) TestFollowsRedirect(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	controllerAPI := mocks.NewMockControllerAPIInfoGetter(ctrl)
	externalControllers := mocks.NewMockExternalControllerUpdater(ctrl)
	s.expectSourceControllerInfo(controllerAPI)
	externalControllers.EXPECT().UpdateExternalController(gomock.Any(), crossmodel.ControllerInfo{
		ControllerUUID: controllerUUID,
		Alias:          redirectAlias,
		Addrs:          []string{dstAddr},
		CACert:         dstCACert,
		ModelUUIDs:     []string{sourceUUID},
	}).Return(nil)

	redirectErr := s.newRedirectError()
	var opens int
	open := func(_ context.Context, info *api.Info) (api.Connection, error) {
		defer func() { opens++ }()
		if opens == 0 {
			return nil, redirectErr
		}
		// The retry must see the redirected addresses and CA cert.
		c.Check(info.Addrs, tc.DeepEquals, []string{dstAddr})
		c.Check(info.CACert, tc.Equals, dstCACert)
		return stubConnection{}, nil
	}

	getter := newRemoteSecretsClientGetter(controllerAPI, externalControllers, open, loggertesting.WrapCheckLog(c))
	client, err := getter(c.Context(), s.newURI())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(client, tc.NotNil)
	c.Check(opens, tc.Equals, 2)
}

func (s *remoteClientGetterSuite) TestFollowsRedirectPersistFails(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	controllerAPI := mocks.NewMockControllerAPIInfoGetter(ctrl)
	externalControllers := mocks.NewMockExternalControllerUpdater(ctrl)
	s.expectSourceControllerInfo(controllerAPI)
	externalControllers.EXPECT().UpdateExternalController(gomock.Any(), crossmodel.ControllerInfo{
		ControllerUUID: controllerUUID,
		Alias:          redirectAlias,
		Addrs:          []string{dstAddr},
		CACert:         dstCACert,
		ModelUUIDs:     []string{sourceUUID},
	}).Return(errors.New("persist failed"))

	redirectErr := s.newRedirectError()
	var opens int
	open := func(_ context.Context, info *api.Info) (api.Connection, error) {
		defer func() { opens++ }()
		if opens == 0 {
			return nil, redirectErr
		}
		return stubConnection{}, nil
	}

	getter := newRemoteSecretsClientGetter(controllerAPI, externalControllers, open, loggertesting.WrapCheckLog(c))
	client, err := getter(c.Context(), s.newURI())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(client, tc.NotNil)
	c.Check(opens, tc.Equals, 2)
}

func (s *remoteClientGetterSuite) TestFollowsRedirectInvalidControllerTag(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	controllerAPI := mocks.NewMockControllerAPIInfoGetter(ctrl)
	externalControllers := mocks.NewMockExternalControllerUpdater(ctrl)
	s.expectSourceControllerInfo(controllerAPI)

	// A redirect without a controller tag cannot be persisted;
	// UpdateExternalController must not be called.
	redirectErr := &api.RedirectError{
		Servers: []network.MachineHostPorts{
			network.NewMachineHostPorts(1234, "7.7.7.7"),
		},
		CACert: dstCACert,
	}
	var opens int
	open := func(_ context.Context, info *api.Info) (api.Connection, error) {
		defer func() { opens++ }()
		if opens == 0 {
			return nil, redirectErr
		}
		return stubConnection{}, nil
	}

	var logs int
	logger := loggertesting.WrapCheckLog(loggertesting.RecordLog(func(string, ...any) {
		logs++
	}))
	getter := newRemoteSecretsClientGetter(controllerAPI, externalControllers, open, logger)
	client, err := getter(c.Context(), s.newURI())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(client, tc.NotNil)
	c.Check(opens, tc.Equals, 2)
	c.Check(logs, tc.Equals, 0)
}

func (s *remoteClientGetterSuite) TestOpenError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	controllerAPI := mocks.NewMockControllerAPIInfoGetter(ctrl)
	externalControllers := mocks.NewMockExternalControllerUpdater(ctrl)
	s.expectSourceControllerInfo(controllerAPI)

	var opens int
	open := func(_ context.Context, info *api.Info) (api.Connection, error) {
		opens++
		return nil, errOpen
	}

	getter := newRemoteSecretsClientGetter(controllerAPI, externalControllers, open, loggertesting.WrapCheckLog(c))
	client, err := getter(c.Context(), s.newURI())
	c.Assert(err, tc.ErrorIs, errOpen)
	c.Check(client, tc.IsNil)
	c.Check(opens, tc.Equals, 1)
}

func (s *remoteClientGetterSuite) TestControllerAPIInfoError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	controllerAPI := mocks.NewMockControllerAPIInfoGetter(ctrl)
	externalControllers := mocks.NewMockExternalControllerUpdater(ctrl)
	infoErr := errors.New("controller info failed")
	controllerAPI.EXPECT().ControllerAPIInfoForModels(gomock.Any(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewModelTag(sourceUUID).String()}},
	}).Return(params.ControllerAPIInfoResults{}, infoErr)

	var opens int
	open := func(context.Context, *api.Info) (api.Connection, error) {
		opens++
		return stubConnection{}, nil
	}
	getter := newRemoteSecretsClientGetter(controllerAPI, externalControllers, open, loggertesting.WrapCheckLog(c))
	client, err := getter(c.Context(), s.newURI())
	c.Assert(err, tc.ErrorIs, infoErr)
	c.Check(client, tc.IsNil)
	c.Check(opens, tc.Equals, 0)
}

func (s *remoteClientGetterSuite) TestControllerAPIInfoEmptyResults(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	controllerAPI := mocks.NewMockControllerAPIInfoGetter(ctrl)
	externalControllers := mocks.NewMockExternalControllerUpdater(ctrl)
	controllerAPI.EXPECT().ControllerAPIInfoForModels(gomock.Any(), gomock.Any()).Return(params.ControllerAPIInfoResults{}, nil)

	var opens int
	open := func(context.Context, *api.Info) (api.Connection, error) {
		opens++
		return stubConnection{}, nil
	}
	getter := newRemoteSecretsClientGetter(controllerAPI, externalControllers, open, loggertesting.WrapCheckLog(c))
	client, err := getter(c.Context(), s.newURI())
	c.Assert(err, tc.ErrorMatches, `no controller api for model "`+sourceUUID+`"`)
	c.Check(client, tc.IsNil)
	c.Check(opens, tc.Equals, 0)
}

func (s *remoteClientGetterSuite) TestControllerAPIInfoResultError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	controllerAPI := mocks.NewMockControllerAPIInfoGetter(ctrl)
	externalControllers := mocks.NewMockExternalControllerUpdater(ctrl)
	infoErr := &params.Error{Code: params.CodeNotFound, Message: "controller not found"}
	controllerAPI.EXPECT().ControllerAPIInfoForModels(gomock.Any(), gomock.Any()).Return(params.ControllerAPIInfoResults{
		Results: []params.ControllerAPIInfoResult{{Error: infoErr}},
	}, nil)

	var opens int
	open := func(context.Context, *api.Info) (api.Connection, error) {
		opens++
		return stubConnection{}, nil
	}
	getter := newRemoteSecretsClientGetter(controllerAPI, externalControllers, open, loggertesting.WrapCheckLog(c))
	client, err := getter(c.Context(), s.newURI())
	c.Assert(err, tc.ErrorIs, infoErr)
	c.Check(client, tc.IsNil)
	c.Check(opens, tc.Equals, 0)
}
