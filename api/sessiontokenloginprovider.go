// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	jujuhttp "github.com/juju/juju/internal/http"
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

// SessionTokenLoginFactory provides a
// level of indirection for generating
// a session token login provider.
type SessionTokenLoginFactory struct{}

// NewLoginProvider provides a real session token login provider.
func (s SessionTokenLoginFactory) NewLoginProvider(token string, output io.Writer, tokenCallback func(token string)) LoginProvider {
	return NewSessionTokenLoginProvider(token, output, tokenCallback)
}

// NewSessionTokenLoginProvider returns a LoginProvider implementation that
// authenticates the entity with the session token.
func NewSessionTokenLoginProvider(
	token string,
	output io.Writer,
	tokenCallback func(token string),
) *sessionTokenLoginProvider {
	return &sessionTokenLoginProvider{
		sessionToken:  token,
		output:        output,
		tokenCallback: tokenCallback,
	}
}

type sessionTokenLoginProvider struct {
	sessionToken string
	// output is used by the login provider to print the user code
	// and verification URL.
	output io.Writer
	// tokenCallback function is used to provide the session
	// token for persisting after successful login.
	tokenCallback func(token string)
}

// AuthHeader implements the [LoginProvider.AuthHeader] method.
// Returns an HTTP header with basic auth set.
// Returns an ErrorLoginFirst error if no token is available.
func (p *sessionTokenLoginProvider) AuthHeader() (http.Header, error) {
	if p.sessionToken == "" {
		return nil, ErrorLoginFirst
	}
	return jujuhttp.BasicAuthHeader("", p.sessionToken), nil
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
	if params.IsCodeSessionTokenInvalid(err) {
		// if we fail because of an invalid session token, we initiate a
		// new device login.
		if err := p.initiateDeviceLogin(ctx, caller); err != nil {
			return nil, errors.Trace(err)
		}
		// and retry the login using the obtained session token.
		return p.login(ctx, caller)
	}
	return nil, errors.Trace(err)
}

func (p *sessionTokenLoginProvider) printOutput(format string, params ...any) error {
	if p.output == nil {
		return errors.New("cannot present login details")
	}
	message := fmt.Sprintf(format, params...)
	if len(message) > 0 && message[len(message)-1] != '\n' {
		message += "\n"
	}
	_, err := fmt.Fprint(p.output, message)
	return err
}

func (p *sessionTokenLoginProvider) initiateDeviceLogin(ctx context.Context, caller base.APICaller) error {
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
	err = p.printOutput("Please visit %s and enter code %s to log in.", deviceResult.VerificationURI, deviceResult.UserCode)
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
	p.tokenCallback(sessionTokenResult.SessionToken)

	return nil
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

	return NewLoginResultParams(result)
}
