// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/bakeryutil"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/core/macaroon"
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

func newOfferAuthcontext(pool *state.StatePool) (*crossmodel.AuthContext, error) {
	// Create a bakery service for discharging third-party caveats for
	// local offer access authentication. This service does not persist keys;
	// its macaroons should be very short-lived.
	st, err := pool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	location := "juju model " + st.ModelUUID()
	checker := checkers.New(macaroon.MacaroonNamespace)

	// Create a bakery service for local offer access authentication. This service
	// persists keys into MongoDB in a TTL collection.
	store, err := st.NewBakeryStorage()
	if err != nil {
		return nil, errors.Trace(err)
	}
	bakeryConfig := st.NewBakeryConfig()
	key, err := bakeryConfig.GetOffersThirdPartyKey()
	if err != nil {
		return nil, errors.Trace(err)
	}
	locator := bakeryutil.BakeryThirdPartyLocator{PublicKey: key.Public}
	localOfferBakery := bakery.New(
		bakery.BakeryParams{
			Checker:       checker,
			RootKeyStore:  store,
			Locator:       locator,
			Key:           key,
			OpsAuthorizer: crossmodel.CrossModelAuthorizer{},
			Location:      location,
		},
	)
	localOfferBakeryKey := key
	offerBakery := &bakeryutil.ExpirableStorageBakery{
		localOfferBakery, location, localOfferBakeryKey, store, locator,
	}
	getTestBakery := func(idURL string) (authentication.ExpirableStorageBakery, error) {
		pKey, err := getPublicKey(idURL)
		if err != nil {
			return nil, errors.Trace(err)
		}
		idPK := pKey.Public
		logger.Criticalf("getTestBakery pKey %q", pKey.Public.String())
		key, err := bakeryConfig.GetExternalUsersThirdPartyKey()
		if err != nil {
			return nil, errors.Trace(err)
		}

		pkCache := bakery.NewThirdPartyStore()
		locator := httpbakery.NewThirdPartyLocator(nil, pkCache)
		pkCache.AddInfo(idURL, bakery.ThirdPartyInfo{
			PublicKey: idPK,
			Version:   3,
		})

		store, err := st.NewBakeryStorage()
		if err != nil {
			return nil, errors.Trace(err)
		}
		store = store.ExpireAfter(15 * time.Minute)
		return &bakeryutil.ExpirableStorageBakery{
			Bakery: bakery.New(
				bakery.BakeryParams{
					Checker:       checker,
					RootKeyStore:  store,
					Locator:       locator,
					Key:           key,
					OpsAuthorizer: crossmodel.CrossModelAuthorizer{},
					Location:      location,
				},
			),
			Location: location,
			Key:      key,
			Store:    store,
			Locator:  locator,
		}, nil
	}
	authCtx, err := crossmodel.NewAuthContext(
		crossmodel.GetBackend(st), key, offerBakery, getTestBakery,
	)
	if err != nil {
		return nil, err
	}
	return authCtx, nil
}

func getPublicKey(idURL string) (*bakery.KeyPair, error) {
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	thirdPartyInfo, err := httpbakery.ThirdPartyInfoForLocation(context.TODO(), &http.Client{Transport: transport}, idURL)
	logger.Criticalf("CreateMacaroonForJaaS thirdPartyInfo.Version %q, thirdPartyInfo.PublicKey.Key.String() %q", thirdPartyInfo.Version, thirdPartyInfo.PublicKey.Key.String())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &bakery.KeyPair{Public: thirdPartyInfo.PublicKey}, nil
}

func (h *localOfferAuthHandler) checkThirdPartyCaveat(stdctx context.Context, req *http.Request, cavInfo *bakery.ThirdPartyCaveatInfo, _ *httpbakery.DischargeToken) ([]checkers.Caveat, error) {
	logger.Debugf("check offer third party caveat %q", cavInfo.Condition)
	details, err := h.authCtx.CheckOfferAccessCaveat(string(cavInfo.Condition))
	if err != nil {
		return nil, errors.Trace(err)
	}

	firstPartyCaveats, err := h.authCtx.CheckLocalAccessRequest(details)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return firstPartyCaveats, nil
}
