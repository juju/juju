// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

var (
	loginDeviceAPICall = func(ctx context.Context, caller base.APICaller, request interface{}, response interface{}) error {
		return caller.APICall(ctx, "Admin", 4, "", "LoginDevice", request, response)
	}
	getDeviceSessionTokenAPICall = func(ctx context.Context, caller base.APICaller, request interface{}, response interface{}) error {
		return caller.APICall(ctx, "Admin", 4, "", "GetDeviceSessionToken", request, response)
	}
	loginWithSessionTokenAPICall = func(ctx context.Context, caller base.APICaller, request interface{}, response interface{}) error {
		return caller.APICall(ctx, "Admin", 4, "", "LoginWithSessionToken", request, response)
	}
)

// NewSessionTokenLoginProvider returns a LoginProvider implementation that
// authenticates the entity with the session token.
func NewSessionTokenLoginProvider(
	token string,
	printOutputFunc func(string, ...any) error,
	updateAccountDetailsFunc func(string) error,
) *sessionTokenLoginProvider {
	return &sessionTokenLoginProvider{
		sessionToken:             token,
		printOutputFunc:          printOutputFunc,
		updateAccountDetailsFunc: updateAccountDetailsFunc,
	}
}

type sessionTokenLoginProvider struct {
	sessionToken string
	// printOutpuFunc is used by the login provider to print the user code
	// and verification URL.
	printOutputFunc func(string, ...any) error
	// updateAccountDetailsFunc function is used to update the session
	// token for the account details.
	updateAccountDetailsFunc func(string) error
}

// Login implements the LoginProvider.Login method.
//
// It authenticates as the entity using the specified session token.
// Subsequent requests on the state will act as that entity.
func (p *sessionTokenLoginProvider) Login(ctx context.Context, caller base.APICaller) (*LoginResultParams, error) {
	// First we try to log in using the session token we have.
	result, err := p.login(ctx, caller)
	if err == nil {
		return result, nil
	}

	if params.ErrCode(err) == params.CodeUnauthorized {
		// if we fail with an "unauthorized" error, we initiate a
		// new device login.
		if err := p.initiateDeviceLogin(ctx, caller); err != nil {
			return nil, errors.Trace(err)
		}
		// and retry the login using the obtained session token.
		return p.login(ctx, caller)
	}
	return nil, errors.Trace(err)
}

func (p *sessionTokenLoginProvider) initiateDeviceLogin(ctx context.Context, caller base.APICaller) error {
	if p.printOutputFunc == nil {
		return errors.New("cannot present login details")
	}

	type loginRequest struct{}

	var deviceResult struct {
		UserCode        string `json:"user-code"`
		VerificationURI string `json:"verification-uri"`
	}

	// The first call we make is to initiate the device login oauth2 flow. This will
	// return a user code and the verification URL - verification URL will point to the
	// configured IdP. These two will be presented to the user. User will have to
	// open a browser, visit the verification URL, enter the user code and log in.
	err := loginDeviceAPICall(ctx, caller, &loginRequest{}, &deviceResult)
	if err != nil {
		return errors.Trace(err)
	}

	// We print the verification URL and the user code.
	err = p.printOutputFunc("Please visit %s and enter code %s to log in.", deviceResult.VerificationURI, deviceResult.UserCode)
	if err != nil {
		return errors.Trace(err)
	}

	type loginResponse struct {
		SessionToken string `json:"session-token"`
	}
	var sessionTokenResult loginResponse
	// Then we make a blocking call to get the session token.
	err = getDeviceSessionTokenAPICall(ctx, caller, &loginRequest{}, &sessionTokenResult)
	if err != nil {
		return errors.Trace(err)
	}

	p.sessionToken = sessionTokenResult.SessionToken

	return p.updateAccountDetailsFunc(sessionTokenResult.SessionToken)
}

func (p *sessionTokenLoginProvider) login(ctx context.Context, caller base.APICaller) (*LoginResultParams, error) {
	var result params.LoginResult
	request := struct {
		SessionToken string `json:"session-token"`
	}{
		SessionToken: p.sessionToken,
	}

	err := loginWithSessionTokenAPICall(ctx, caller, request, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var controllerAccess string
	var modelAccess string
	var tag names.Tag
	if result.UserInfo != nil {
		tag, err = names.ParseTag(result.UserInfo.Identity)
		if err != nil {
			return nil, errors.Trace(err)
		}
		controllerAccess = result.UserInfo.ControllerAccess
		modelAccess = result.UserInfo.ModelAccess
	}
	servers := params.ToMachineHostsPorts(result.Servers)
	serverVersion, err := version.Parse(result.ServerVersion)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &LoginResultParams{
		tag:              tag,
		modelTag:         result.ModelTag,
		controllerTag:    result.ControllerTag,
		servers:          servers,
		publicDNSName:    result.PublicDNSName,
		facades:          result.Facades,
		modelAccess:      modelAccess,
		controllerAccess: controllerAccess,
		serverVersion:    serverVersion,
	}, nil
}
