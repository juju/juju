// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/watcher"
)

// offeredApplicationsAPI allows access to a services exported from the environment.
type offeredApplicationsAPI struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// OfferedApplicationAPI provides access to application offerings from this environment.
type OfferedApplicationAPI interface {

	// OfferedApplications returns the offered applications for the specified urls.
	// The results and any errors are keyed by URL.
	OfferedApplications(applicationURLs ...string) (map[string]crossmodel.OfferedApplication, map[string]error, error)

	// WatchOfferedApplications starts a watcher for changes to the offered
	// services from this environment.
	WatchOfferedApplications() (watcher.StringsWatcher, error)
}

// NewOfferedApplications creates a new client for accessing an offered applications API.
func NewOfferedApplications(st base.APICallCloser) OfferedApplicationAPI {
	frontend, backend := base.NewClientFacade(st, "OfferedApplications")
	return &offeredApplicationsAPI{ClientFacade: frontend, facade: backend}
}

// OfferedApplications returns the offered applications for the specified urls.
// The results and any errors are keyed by URL.
func (s *offeredApplicationsAPI) OfferedApplications(applicationURLs ...string) (map[string]crossmodel.OfferedApplication, map[string]error, error) {
	if len(applicationURLs) == 0 {
		return nil, nil, errors.New("no application URLs specified")
	}
	var queryParams params.ApplicationURLs
	queryParams.ApplicationURLs = make([]string, len(applicationURLs))
	for i, url := range applicationURLs {
		queryParams.ApplicationURLs[i] = url
	}
	results := new(params.OfferedApplicationResults)
	if err := s.facade.FacadeCall("OfferedApplications", queryParams, results); err != nil {
		return nil, nil, errors.Trace(err)
	}
	if len(results.Results) != len(applicationURLs) {
		return nil, nil, errors.Errorf("expected %d results, got %d", len(applicationURLs), len(results.Results))
	}
	offers := make(map[string]crossmodel.OfferedApplication)
	offerErrors := make(map[string]error)
	for i, result := range results.Results {
		applicationURL := applicationURLs[i]
		if result.Error != nil {
			if result.Error.ErrorCode() == params.CodeNotFound {
				offerErrors[applicationURL] = errors.NotFoundf("offered application at %q", applicationURLs[i])
			} else {
				offerErrors[applicationURL] = errors.Annotatef(result.Error, "error retrieving offer at %q", applicationURLs[i])
			}
			continue
		}
		offers[result.Result.ApplicationURL] = MakeOfferedApplicationFromParams(result.Result)
	}
	return offers, offerErrors, nil
}

// MakeOfferedApplicationFromParams creates an OfferedApplication from api parameters.
func MakeOfferedApplicationFromParams(offer params.OfferedApplication) crossmodel.OfferedApplication {
	eps := make(map[string]string, len(offer.Endpoints))
	for k, v := range offer.Endpoints {
		eps[k] = v
	}
	return crossmodel.OfferedApplication{
		ApplicationURL:  offer.ApplicationURL,
		ApplicationName: offer.ApplicationName,
		CharmName:       offer.CharmName,
		Description:     offer.Description,
		Registered:      offer.Registered,
		Endpoints:       eps,
	}
}

// WatchOfferedApplications starts a watcher for changes to the offered
// services from this environment.
func (s *offeredApplicationsAPI) WatchOfferedApplications() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := s.facade.FacadeCall("WatchOfferedApplications", nil, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(s.facade.RawAPICaller(), result)
	return w, nil
}
