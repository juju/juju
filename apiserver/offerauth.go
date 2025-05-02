// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/internal/macaroon"
	"github.com/juju/juju/state"
)

const (
	localOfferAccessLocationPath = "/offeraccess"
)

type localOfferAuthHandler struct {
	authCtx *crossmodel.AuthContext
}

func addOfferAuthHandlers(offerAuthCtxt *crossmodel.AuthContext, mux *apiserverhttp.Mux) {
	appOfferHandler := &localOfferAuthHandler{authCtx: offerAuthCtxt}
	appOfferDischargeMux := http.NewServeMux()

	discharger := httpbakery.NewDischarger(
		httpbakery.DischargerParams{
			Key:     offerAuthCtxt.OfferThirdPartyKey(),
			Checker: httpbakery.ThirdPartyCaveatCheckerFunc(appOfferHandler.checkThirdPartyCaveat),
		})
	discharger.AddMuxHandlers(appOfferDischargeMux, localOfferAccessLocationPath)

	_ = mux.AddHandler("POST", localOfferAccessLocationPath+"/discharge", appOfferDischargeMux)
	_ = mux.AddHandler("GET", localOfferAccessLocationPath+"/publickey", appOfferDischargeMux)
}

func newOfferAuthContext(
	ctx context.Context,
	pool *state.StatePool,
	clock clock.Clock,
	accessService AccessService,
	modelInfoService ModelInfoService,
	controllerConfigService ControllerConfigService,
	macaroonService MacaroonService,
) (*crossmodel.AuthContext, error) {
	// Create a bakery service for discharging third-party caveats for
	// local offer access authentication. This service does not persist keys;
	// its macaroons should be very short-lived.
	st, err := pool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelInfo, err := modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("retrieving model info for controller model: %w", err)
	}
	location := "juju model " + modelInfo.UUID.String()
	checker := checkers.New(macaroon.MacaroonNamespace)

	// Create a bakery service for local offer access authentication. This service
	// persists keys into DQLite in a TTL collection.
	store := macaroon.NewExpirableStorage(macaroonService, macaroon.DefaultExpiration, clock)
	key, err := macaroonService.GetOffersThirdPartyKey(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "unable to get controller config")
	}
	modelTag := names.NewModelTag(modelInfo.UUID.String())
	loginTokenRefreshURL := controllerConfig.LoginTokenRefreshURL()
	if loginTokenRefreshURL != "" {
		offerBakery, err := crossmodel.NewJaaSOfferBakery(
			ctx, loginTokenRefreshURL, location, clock, macaroonService, store, checker,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return crossmodel.NewAuthContext(crossmodel.GetBackend(st), accessService, modelTag, key, offerBakery)
	}
	offerBakery, err := crossmodel.NewLocalOfferBakery(location, key, store, checker, clock)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return crossmodel.NewAuthContext(crossmodel.GetBackend(st), accessService, modelTag, key, offerBakery)
}

func (h *localOfferAuthHandler) checkThirdPartyCaveat(stdctx context.Context, req *http.Request, cavInfo *bakery.ThirdPartyCaveatInfo, _ *httpbakery.DischargeToken) ([]checkers.Caveat, error) {
	logger.Debugf(req.Context(), "check offer third party caveat %q", cavInfo.Condition)
	details, err := h.authCtx.CheckOfferAccessCaveat(string(cavInfo.Condition))
	if err != nil {
		return nil, errors.Trace(err)
	}

	firstPartyCaveats, err := h.authCtx.CheckLocalAccessRequest(stdctx, details)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return firstPartyCaveats, nil
}
