// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/bakeryutil"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/state/bakerystorage"
)

func getLocalOfferBakery(
	location string,
	bakeryConfig bakerystorage.BakeryConfig,
	store bakerystorage.ExpirableStorage,
	checker bakery.FirstPartyCaveatChecker,
) (authentication.ExpirableStorageBakery, error) {
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
	return &bakeryutil.ExpirableStorageBakery{
		Bakery:   localOfferBakery,
		Location: location,
		Key:      key,
		Store:    store,
		Locator:  locator,
	}, nil
}

var (
	// Override for testing.
	DefaultTransport = http.DefaultTransport
)

func getJaaSOfferBakery(
	loginTokenRefreshURL, location string,
	bakeryConfig bakerystorage.BakeryConfig,
	store bakerystorage.ExpirableStorage,
	checker bakery.FirstPartyCaveatChecker,
) (authentication.ExpirableStorageBakery, string, error) {
	refreshURL, err := url.Parse(loginTokenRefreshURL)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	refreshURL.Path = ""
	pkURL, err := url.JoinPath(refreshURL.String(), "macaroons")
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	thirdPartyInfo, err := httpbakery.ThirdPartyInfoForLocation(
		context.TODO(), &http.Client{Transport: DefaultTransport}, pkURL,
	)
	logger.Tracef("got third party info %#v from %q", thirdPartyInfo, pkURL)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	key, err := bakeryConfig.GetExternalUsersThirdPartyKey()
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	pkCache := bakery.NewThirdPartyStore()
	pkCache.AddInfo(pkURL, thirdPartyInfo)
	locator := httpbakery.NewThirdPartyLocator(nil, pkCache)

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
	}, pkURL, nil
}
