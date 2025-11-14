// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/names/v5"
	"github.com/juju/utils/v3"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	coremacaroon "github.com/juju/juju/core/macaroon"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
	jujuversion "github.com/juju/juju/version"
)

// NewLegacyLoginProvider returns a LoginProvider implementation that
// authenticates the entity with the given name and password or macaroons. The nonce
// should be empty unless logging in as a machine agent.
func NewLegacyLoginProvider(
	tag names.Tag,
	password string,
	nonce string,
	macaroons []macaroon.Slice,
	cookieURL *url.URL,
) *legacyLoginProvider {
	return &legacyLoginProvider{
		tag:       tag,
		password:  password,
		nonce:     nonce,
		macaroons: macaroons,
		cookieURL: cookieURL,
	}
}

// legacyLoginProvider provides the default juju login provider that
// authenticates the entity with the given name and password or macaroons. The
// nonce should be empty unless logging in as a machine agent.
type legacyLoginProvider struct {
	tag          names.Tag
	password     string
	nonce        string
	macaroons    []macaroon.Slice
	bakeryClient base.MacaroonDischarger
	cookieURL    *url.URL
}

// AuthHeader implements the [LoginProvider.AuthHeader] method.
// Returns an HTTP header with basic auth if a user tag is provided.
// The header will also include any macaroons as cookies.
func (p *legacyLoginProvider) AuthHeader() (http.Header, error) {
	var requestHeader http.Header
	if p.tag != nil {
		// Note that password may be empty here; we still
		// want to pass the tag along. An empty password
		// indicates that we're using macaroon authentication.
		requestHeader = jujuhttp.BasicAuthHeader(p.tag.String(), p.password)
	} else {
		requestHeader = make(http.Header)
	}
	if p.nonce != "" {
		requestHeader.Set(params.MachineNonceHeader, p.nonce)
	}
	// Add any cookies because they will not be sent to websocket
	// connections by default.
	err := p.addCookiesToHeader(requestHeader)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return requestHeader, nil
}

// addCookiesToHeader adds any macaroons associated with the
// API host to the given header. This is necessary because
// otherwise cookies are not sent to websocket endpoints.
func (p *legacyLoginProvider) addCookiesToHeader(h http.Header) error {
	// Note: The go-macaroon-bakery accepts macaroons either as encoded header
	// values or as cookies. Here we opt to add them as cookies.
	// See https://github.com/go-macaroon-bakery/macaroon-bakery/blob/v3/httpbakery/client.go#L683
	// and apiserver/stateauthenticator/auth.go LoginRequest

	// net/http only allows adding cookies to a request,
	// but when it sends a request to a non-http endpoint,
	// it doesn't add the cookies, so make a request, starting
	// with the given header, add the cookies to use, then
	// throw away the request but keep the header.
	req := &http.Request{
		Header: h,
	}
	var cookies []*http.Cookie
	if p.bakeryClient != nil {
		cookies = p.bakeryClient.CookieJar().Cookies(p.cookieURL)
		for _, c := range cookies {
			req.AddCookie(c)
		}
	}
	if len(cookies) == 0 && len(p.macaroons) > 0 {
		// These macaroons must have been added directly rather than
		// obtained from a request. Add them. (For example in the
		// logtransfer connection for a migration.)
		// See https://bugs.launchpad.net/juju/+bug/1650451
		for _, macaroon := range p.macaroons {
			cookie, err := httpbakery.NewCookie(coremacaroon.MacaroonNamespace, macaroon)
			if err != nil {
				return errors.Trace(err)
			}
			req.AddCookie(cookie)
		}
	}
	h.Set(httpbakery.BakeryProtocolHeader, fmt.Sprint(bakery.LatestVersion))
	return nil
}

// Login implements the LoginProvider.Login method.
//
// It authenticates as the entity with the given name and password
// or macaroons. Subsequent requests on the state will act as that entity.
func (p *legacyLoginProvider) Login(ctx context.Context, caller base.APICaller) (*LoginResultParams, error) {
	var authTag string
	if p.tag != nil {
		authTag = p.tag.String()
	}
	// Store the bakery client for later use in AuthHeader()
	// when we want to authenticate HTTP requests.
	p.bakeryClient = caller.BakeryClient()

	request := &params.LoginRequest{
		AuthTag:       authTag,
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
			httpbakery.MacaroonsForURL(p.bakeryClient.CookieJar(), p.cookieURL)...,
		)
	}
	var result params.LoginResult
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
		request.Macaroons = httpbakery.MacaroonsForURL(p.bakeryClient.CookieJar(), p.cookieURL)
		result = params.LoginResult{} // zero result
		err = caller.APICall("Admin", 3, "", "Login", request, &result)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if result.DischargeRequired != nil {
			return nil, errors.Errorf("login with discharged macaroons failed: %s", result.DischargeRequiredReason)
		}
	}
	loginResult, err := NewLoginResultParams(result)
	if err != nil {
		return loginResult, err
	}
	// Edge case for username/password login. Ensure the result has a tag set.
	// Currently no tag is returned when performing a login as a machine rather than a user.
	// Ideally the server would respond with the tag used as part of the request.
	loginResult.EnsureTag(p.tag)
	return loginResult, nil
}
