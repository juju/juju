// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"net/url"
	"os"
	"runtime/debug"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/names/v5"
	"github.com/juju/utils/v3"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
	jujuversion "github.com/juju/juju/version"
)

// NewUserpassLoginProvider returns a LoginProvider implementation that
// authenticates the entity with the given name and password or macaroons. The nonce
// should be empty unless logging in as a machine agent.
func NewUserpassLoginProvider(
	tag names.Tag,
	password string,
	nonce string,
	macaroons []macaroon.Slice,
	bakeryClient *httpbakery.Client,
	cookieURL *url.URL,
) *userpassLoginProvider {
	return &userpassLoginProvider{
		tag:          tag,
		password:     password,
		nonce:        nonce,
		macaroons:    macaroons,
		bakeryClient: bakeryClient,
		cookieURL:    cookieURL,
	}
}

// userpassLoginProvider provides the default juju login provider that
// authenticates the entity with the given name and password or macaroons. The
// nonce should be empty unless logging in as a machine agent.
type userpassLoginProvider struct {
	tag          names.Tag
	password     string
	nonce        string
	macaroons    []macaroon.Slice
	bakeryClient *httpbakery.Client
	cookieURL    *url.URL
}

// Login implements the LoginProvider.Login method.
//
// It authenticates as the entity with the given name and password
// or macaroons. Subsequent requests on the state will act as that entity.
func (p *userpassLoginProvider) Login(ctx context.Context, caller base.APICaller) (*LoginResultParams, error) {
	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag:       tagToString(p.tag),
		Credentials:   p.password,
		Nonce:         p.nonce,
		Macaroons:     p.macaroons,
		BakeryVersion: bakery.LatestVersion,
		CLIArgs:       utils.CommandString(os.Args...),
		ClientVersion: jujuversion.Current.String(),
	}
	// If we are in developer mode, add the stack location as user data to the
	// login request. This will allow the apiserver to connect connection ids
	// to the particular place that initiated the connection.
	if featureflag.Enabled(feature.DeveloperMode) {
		request.UserData = string(debug.Stack())
	}

	if p.password == "" {
		// Add any macaroons from the cookie jar that might work for
		// authenticating the login request.
		request.Macaroons = append(request.Macaroons,
			httpbakery.MacaroonsForURL(p.bakeryClient.Jar, p.cookieURL)...,
		)
	}
	err := caller.APICall("Admin", 3, "", "Login", request, &result)
	if err != nil {
		if !params.IsRedirect(err) {
			return nil, errors.Trace(err)
		}

		if rpcErr, ok := errors.Cause(err).(*rpc.RequestError); ok {
			var redirInfo params.RedirectErrorInfo
			err := rpcErr.UnmarshalInfo(&redirInfo)
			if err == nil && redirInfo.CACert != "" && len(redirInfo.Servers) != 0 {
				var controllerTag names.ControllerTag
				if redirInfo.ControllerTag != "" {
					if controllerTag, err = names.ParseControllerTag(redirInfo.ControllerTag); err != nil {
						return nil, errors.Trace(err)
					}
				}

				return nil, &RedirectError{
					Servers:         params.ToMachineHostsPorts(redirInfo.Servers),
					CACert:          redirInfo.CACert,
					ControllerTag:   controllerTag,
					ControllerAlias: redirInfo.ControllerAlias,
					FollowRedirect:  false, // user-action required
				}
			}
		}

		// We've been asked to redirect. Find out the redirection info.
		// If the rpc packet allowed us to return arbitrary information in
		// an error, we'd probably put this information in the Login response,
		// but we can't do that currently.
		var resp params.RedirectInfoResult
		if err := caller.APICall("Admin", 3, "", "RedirectInfo", nil, &resp); err != nil {
			return nil, errors.Annotatef(err, "cannot get redirect addresses")
		}
		return nil, &RedirectError{
			Servers:        params.ToMachineHostsPorts(resp.Servers),
			CACert:         resp.CACert,
			FollowRedirect: true, // JAAS-type redirect
		}
	}
	if result.DischargeRequired != nil || result.BakeryDischargeRequired != nil {
		// The result contains a discharge-required
		// macaroon. We discharge it and retry
		// the login request with the original macaroon
		// and its discharges.
		if result.DischargeRequiredReason == "" {
			result.DischargeRequiredReason = "no reason given for discharge requirement"
		}
		// Prefer the newer bakery.v2 macaroon.
		dcMac := result.BakeryDischargeRequired
		if dcMac == nil {
			dcMac, err = bakery.NewLegacyMacaroon(result.DischargeRequired)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		if err := p.bakeryClient.HandleError(ctx, p.cookieURL, &httpbakery.Error{
			Message: result.DischargeRequiredReason,
			Code:    httpbakery.ErrDischargeRequired,
			Info: &httpbakery.ErrorInfo{
				Macaroon:     dcMac,
				MacaroonPath: "/",
			},
		}); err != nil {
			cause := errors.Cause(err)
			if httpbakery.IsInteractionError(cause) {
				// Just inform the user of the reason for the
				// failure, e.g. because the username/password
				// they presented was invalid.
				err = cause.(*httpbakery.InteractionError).Reason
			}
			return nil, errors.Trace(err)
		}
		// Add the macaroons that have been saved by HandleError to our login request.
		request.Macaroons = httpbakery.MacaroonsForURL(p.bakeryClient.Jar, p.cookieURL)
		result = params.LoginResult{} // zero result
		err = caller.APICall("Admin", 3, "", "Login", request, &result)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if result.DischargeRequired != nil {
			return nil, errors.Errorf("login with discharged macaroons failed: %s", result.DischargeRequiredReason)
		}
	}

	return NewLoginResultParams(result)
}
