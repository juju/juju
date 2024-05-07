// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationscaler

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/watcher"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

var logger = internallogger.GetLogger("juju.api.applicationscaler")

// NewWatcherFunc exists to let us test Watch properly.
type NewWatcherFunc func(base.APICaller, params.StringsWatchResult) watcher.StringsWatcher

// API makes calls to the ApplicationScaler facade.
type API struct {
	caller     base.FacadeCaller
	newWatcher NewWatcherFunc
}

// NewAPI returns a new API using the supplied caller.
func NewAPI(caller base.APICaller, newWatcher NewWatcherFunc, options ...Option) *API {
	return &API{
		caller:     base.NewFacadeCaller(caller, "ApplicationScaler", options...),
		newWatcher: newWatcher,
	}
}

// Watch returns a StringsWatcher that delivers the names of applications
// that may need to be rescaled.
func (api *API) Watch() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := api.caller.FacadeCall(context.TODO(), "Watch", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, errors.Trace(result.Error)
	}
	w := api.newWatcher(api.caller.RawAPICaller(), result)
	return w, nil
}

// Rescale requests that all supplied application names be rescaled to
// their minimum configured sizes. It returns the first error it
// encounters.
func (api *API) Rescale(applications []string) error {
	args := params.Entities{
		Entities: make([]params.Entity, len(applications)),
	}
	for i, application := range applications {
		if !names.IsValidApplication(application) {
			return errors.NotValidf("application name %q", application)
		}
		tag := names.NewApplicationTag(application)
		args.Entities[i].Tag = tag.String()
	}
	var results params.ErrorResults
	err := api.caller.FacadeCall(context.TODO(), "Rescale", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	for _, result := range results.Results {
		if result.Error != nil {
			if err == nil {
				err = result.Error
			} else {
				logger.Errorf("additional rescale error: %v", err)
			}
		}
	}
	return errors.Trace(err)
}
