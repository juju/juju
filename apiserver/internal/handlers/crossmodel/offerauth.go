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
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/logger"
	internalerrors "github.com/juju/juju/internal/errors"
)

const (
	localOfferAccessLocationPath = "/offeraccess"
)

// CrossModelAuthContextProvider provides access to offer authentication contexts.
type CrossModelAuthContextProvider interface {
	// NewCrossModelAuthContext creates a new OfferAuthContext for the
	// given request host.
	NewCrossModelAuthContext(requestHost string) (facade.CrossModelAuthContext, error)
}

// AddOfferAuthHandlers adds the HTTP handlers used for application offer
// macaroon authentication.
func AddOfferAuthHandlers(authContextProvider CrossModelAuthContextProvider, keyPair *bakery.KeyPair, mux *apiserverhttp.Mux, logger logger.Logger) error {
	appOfferDischargeMux := http.NewServeMux()

	appOfferHandler := &localOfferAuthHandler{authContextProvider: authContextProvider, logger: logger}
	discharger := httpbakery.NewDischarger(httpbakery.DischargerParams{
		Key:     keyPair,
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
	authContextProvider CrossModelAuthContextProvider
	logger              logger.Logger
}

func (h *localOfferAuthHandler) checkThirdPartyCaveat(ctx context.Context, req *http.Request, cavInfo *bakery.ThirdPartyCaveatInfo, _ *httpbakery.DischargeToken) ([]checkers.Caveat, error) {
	h.logger.Debugf(ctx, "check offer third party caveat %q", cavInfo.Condition)

	authContext, err := h.authContextProvider.NewCrossModelAuthContext(req.Host)
	if err != nil {
		return nil, errors.Trace(err)
	}

	details, err := authContext.CheckOfferAccessCaveat(ctx, string(cavInfo.Condition))
	if err != nil {
		return nil, errors.Trace(err)
	}

	firstPartyCaveats, err := authContext.CheckLocalAccessRequest(ctx, details)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return firstPartyCaveats, nil
}
