// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"
	"net/http"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/internal/crossmodel"
	"github.com/juju/juju/core/logger"
	internalerrors "github.com/juju/juju/internal/errors"
)

const (
	localOfferAccessLocationPath = "/offeraccess"
)

// OfferAuthContext provides access to the offer authentication context.
type OfferAuthContext interface {
	// OfferThirdPartyKey returns the key pair used to sign third party
	// caveats for offer macaroons.
	OfferThirdPartyKey() *bakery.KeyPair

	// CheckOfferAccessCaveat validates the offer access caveat and
	// returns the details encoded within it.
	CheckOfferAccessCaveat(ctx context.Context, caveat string) (crossmodel.OfferAccessDetails, error)

	// CheckLocalAccessRequest validates the local access request
	// encoded in the offer access details and returns the first party
	// caveats that should be added to the discharge macaroon.
	CheckLocalAccessRequest(ctx context.Context, details crossmodel.OfferAccessDetails) ([]checkers.Caveat, error)
}

// AddOfferAuthHandlers adds the HTTP handlers used for application offer
// macaroon authentication.
func AddOfferAuthHandlers(authContext OfferAuthContext, mux *apiserverhttp.Mux) error {
	appOfferDischargeMux := http.NewServeMux()

	appOfferHandler := &localOfferAuthHandler{authContext: authContext}
	discharger := httpbakery.NewDischarger(httpbakery.DischargerParams{
		Key:     authContext.OfferThirdPartyKey(),
		Checker: httpbakery.ThirdPartyCaveatCheckerFunc(appOfferHandler.checkThirdPartyCaveat),
	})
	discharger.AddMuxHandlers(appOfferDischargeMux, localOfferAccessLocationPath)

	if err := mux.AddHandler("POST", localOfferAccessLocationPath+"/discharge", appOfferDischargeMux); err != nil && !errors.Is(err, errors.AlreadyExists) {
		return internalerrors.Errorf("adding discharge handler: %w", err)
	}
	if err := mux.AddHandler("GET", localOfferAccessLocationPath+"/publickey", appOfferDischargeMux); err != nil && !errors.Is(err, errors.AlreadyExists) {
		return internalerrors.Errorf("adding public key handler: %w", err)
	}

	return nil
}

type localOfferAuthHandler struct {
	authContext OfferAuthContext
	logger      logger.Logger
}

func (h *localOfferAuthHandler) checkThirdPartyCaveat(ctx context.Context, req *http.Request, cavInfo *bakery.ThirdPartyCaveatInfo, _ *httpbakery.DischargeToken) ([]checkers.Caveat, error) {
	h.logger.Debugf(ctx, "check offer third party caveat %q", cavInfo.Condition)

	details, err := h.authContext.CheckOfferAccessCaveat(ctx, string(cavInfo.Condition))
	if err != nil {
		return nil, errors.Trace(err)
	}

	firstPartyCaveats, err := h.authContext.CheckLocalAccessRequest(ctx, details)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return firstPartyCaveats, nil
}
