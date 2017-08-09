// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"

	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"

	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/state"
)

const (
	localOfferAccessLocationPath = "/offeraccess"
)

type localOfferAuthHandler struct {
	authCtx *crossmodel.AuthContext
}

func newOfferAuthcontext(pool *state.StatePool) (*crossmodel.AuthContext, error) {
	// Create a bakery service for discharging third-party caveats for
	// local offer access authentication. This service does not persist keys;
	// its macaroons should be very short-lived.
	st := pool.SystemState()
	localOfferThirdPartyBakeryService, _, err := newBakeryService(st, nil, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Create a bakery service for local offer access authentication. This service
	// persists keys into MongoDB in a TTL collection.
	store, err := st.NewBakeryStorage()
	if err != nil {
		return nil, errors.Trace(err)
	}
	locator := bakeryServicePublicKeyLocator{localOfferThirdPartyBakeryService}
	localUserBakeryService, localUserBakeryServiceKey, err := newBakeryService(
		st, store, locator,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	localOfferBakeryService := &expirableStorageBakeryService{
		localUserBakeryService, localUserBakeryServiceKey, store, locator,
	}
	authCtx, err := crossmodel.NewAuthContext(crossmodel.GetStatePool(pool), localOfferThirdPartyBakeryService, localOfferBakeryService)
	if err != nil {
		return nil, err
	}
	return authCtx, nil
}

func (h *localOfferAuthHandler) checkThirdPartyCaveat(req *http.Request, cavId, cav string) ([]checkers.Caveat, error) {
	ctx := &macaroonOfferAuthContext{h.authCtx, req}
	return ctx.CheckThirdPartyCaveat(cavId, cav)
}

type macaroonOfferAuthContext struct {
	*crossmodel.AuthContext
	req *http.Request
}

// CheckThirdPartyCaveat is part of the bakery.ThirdPartyChecker interface.
func (ctx *macaroonOfferAuthContext) CheckThirdPartyCaveat(cavId, cav string) ([]checkers.Caveat, error) {
	logger.Debugf("check third party caveat %v: %v", cavId, cav)
	details, err := ctx.CheckOfferAccessCaveat(cav)
	if err != nil {
		return nil, errors.Trace(err)
	}

	firstPartyCaveats, err := ctx.CheckLocalAccessRequest(details)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return firstPartyCaveats, nil
}
