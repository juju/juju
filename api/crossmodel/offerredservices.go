// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/model/crossmodel"
)

// offeredServicesAPI allows access to a services exported from the environment.
type offeredServicesAPI struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// OfferedServiceAPI provides access to service offerings from this environment.
type OfferedServiceAPI interface {

	// OfferedServices returns the offered services for the specified urls.
	// The results and any errors are keyed by URL.
	OfferedServices(serviceUrls ...string) (map[string]crossmodel.OfferedService, map[string]error, error)

	// WatchOfferedServices starts a watcher for changes to the offered
	// services from this environment.
	WatchOfferedServices() (watcher.StringsWatcher, error)
}

// NewOfferedServices creates a new client for accessing an offered services API.
func NewOfferedServices(st base.APICallCloser) OfferedServiceAPI {
	frontend, backend := base.NewClientFacade(st, "OfferedServices")
	return &offeredServicesAPI{ClientFacade: frontend, facade: backend}
}

// OfferedServices returns the offered services for the specified urls.
// The results and any errors are keyed by URL.
func (s *offeredServicesAPI) OfferedServices(serviceUrls ...string) (map[string]crossmodel.OfferedService, map[string]error, error) {
	if len(serviceUrls) == 0 {
		return nil, nil, errors.New("no service urls specified")
	}
	var queryParams params.OfferedServiceQueryParams
	queryParams.ServiceUrls = make([]string, len(serviceUrls))
	for i, url := range serviceUrls {
		queryParams.ServiceUrls[i] = url
	}
	results := new(params.OfferedServiceResults)
	if err := s.facade.FacadeCall("OfferedServices", queryParams, results); err != nil {
		return nil, nil, errors.Trace(err)
	}
	if len(results.Results) != len(serviceUrls) {
		return nil, nil, errors.Errorf("expected %d results, got %d", len(serviceUrls), len(results.Results))
	}
	offers := make(map[string]crossmodel.OfferedService)
	offerErrors := make(map[string]error)
	for i, result := range results.Results {
		serviceUrl := serviceUrls[i]
		if result.Error != nil {
			if result.Error.ErrorCode() == params.CodeNotFound {
				offerErrors[serviceUrl] = errors.NotFoundf("offered service at %q", serviceUrls[i])
			} else {
				offerErrors[serviceUrl] = errors.Annotatef(result.Error, "error retrieving offer at %q", serviceUrls[i])
			}
			continue
		}
		offers[result.Result.ServiceURL] = MakeOfferedServiceFromParams(result.Result)
	}
	return offers, offerErrors, nil
}

// MakeOfferedServiceFromParams creates an OfferedService from api parameters.
func MakeOfferedServiceFromParams(offer params.OfferedService) crossmodel.OfferedService {
	eps := make(map[string]string, len(offer.Endpoints))
	for k, v := range offer.Endpoints {
		eps[k] = v
	}
	return crossmodel.OfferedService{
		ServiceURL:  offer.ServiceURL,
		ServiceName: offer.ServiceName,
		CharmName:   offer.CharmName,
		Description: offer.Description,
		Registered:  offer.Registered,
		Endpoints:   eps,
	}
}

// WatchOfferedServices starts a watcher for changes to the offered
// services from this environment.
func (s *offeredServicesAPI) WatchOfferedServices() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := s.facade.FacadeCall("WatchOfferedServices", nil, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	w := watcher.NewStringsWatcher(s.facade.RawAPICaller(), result)
	return w, nil
}
