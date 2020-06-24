// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sort"
	"sync"

	"github.com/juju/pubsub"

	"github.com/juju/juju/core/status"
)

const (
	// An application charm URL has changed.
	applicationCharmURLChange = "application-charm-url-change"
	// Application config has changed.
	applicationConfigChange = "application-config-change"
)

func newApplication(model *Model, metrics *ControllerGauges, hub *pubsub.SimpleHub, res *Resident) *Application {
	a := &Application{
		Resident: res,
		model:    model,
		metrics:  metrics,
		hub:      hub,
		mu:       &sync.Mutex{},
	}
	return a
}

// Application represents an application in a model.
type Application struct {
	// Resident identifies the application as a type-agnostic cached entity
	// and tracks resources that it is responsible for cleaning up.
	*Resident

	model   *Model
	metrics *ControllerGauges
	hub     *pubsub.SimpleHub
	mu      *sync.Mutex

	details    ApplicationChange
	configHash string
	hashCache  *hashCache
}

// Note that these property accessors are not lock-protected.
// They are intended for calling from external packages that have retrieved a
// deep copy from the cache.

// CharmURL returns the charm url string for this application.
func (a *Application) CharmURL() string {
	return a.details.CharmURL
}

// Config returns a copy of the current application config.
func (a *Application) Config() map[string]interface{} {
	a.metrics.ApplicationConfigReads.Inc()
	return a.details.Config
}

// Units returns all the units for this application.
func (a *Application) Units() []Unit {
	units := a.model.applicationUnits(a.details.Name)
	sort.Slice(units, func(i, j int) bool {
		return units[i].Name() < units[j].Name()
	})
	return units
}

// ExpectsWorkload only makes sense for CAAS applications.
// If a workload is expected, the charm would have set a podspec.
// Pod specs are never set for IAAS models.
func (a *Application) ExpectsWorkload() bool {
	return a.details.PodSpec != nil
}

// Status returns the application status if it is set, and if not
// it uses the derived status from the units.
func (a *Application) maybeDerivedStatus(getStatus func(*Unit) status.StatusInfo) status.StatusInfo {
	info := a.details.Status
	if info.Status == status.Unset {
		// Get all the unit workload statuses to derive the application one.
		var statuses []status.StatusInfo
		for _, u := range a.Units() {
			statuses = append(statuses, getStatus(&u))
		}
		derived := status.DeriveStatus(statuses)
		if derived.Since == nil {
			derived.Since = info.Since
		}
		return derived
	}
	return info
}

// Status returns the application status if it is set, and if not
// it uses the derived status from the units.
func (a *Application) Status() status.StatusInfo {
	return a.maybeDerivedStatus((*Unit).WorkloadStatus)
}

// DisplayStatus is used from the status code in the apiserver, and is what
// we show to the user. For CAAS models, this is a synthetic status based on
// the application status and the operator status.
func (a *Application) DisplayStatus() status.StatusInfo {
	info := a.maybeDerivedStatus((*Unit).DisplayWorkloadStatus)
	// If there is no status in the OperatorStatus, it is because there
	// is no operator status, which is the normal situation for IAAS applications.
	opInfo := a.details.OperatorStatus
	if opInfo.Status == "" {
		return info
	}
	return status.ApplicationDisplayStatus(info, opInfo, a.ExpectsWorkload())
}

// WatchConfig creates a watcher for the application config.
// Do not use this to watch config on behalf of a unit.
// It is not aware of branch-based config deltas
// and only deals with master settings.
func (a *Application) WatchConfig(keys ...string) *ConfigWatcher {
	a.mu.Lock()
	defer a.mu.Unlock()

	w := newConfigWatcher(keys, a.hashCache, a.hub, a.topic(applicationConfigChange), a.Resident)
	return w
}

// appCharmUrlChange contains an appName and it's charm URL.  To be used
// when publishing for applicationCharmURLChange.
type appCharmUrlChange struct {
	appName string
	chURL   string
}

func (a *Application) setDetails(details ApplicationChange) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// If this is the first receipt of details, set the removal message.
	if a.removalMessage == nil {
		a.removalMessage = RemoveApplication{
			ModelUUID: details.ModelUUID,
			Name:      details.Name,
		}
	}

	a.setStale(false)

	if a.details.CharmURL != details.CharmURL {
		a.hub.Publish(applicationCharmURLChange, appCharmUrlChange{appName: a.details.Name, chURL: details.CharmURL})
	}

	a.details = details
	hashCache, configHash := newHashCache(
		details.Config, a.metrics.ApplicationHashCacheHit, a.metrics.ApplicationHashCacheMiss)

	if configHash != a.configHash {
		a.configHash = configHash
		a.hashCache = hashCache
		a.hashCache.incMisses()
		a.hub.Publish(a.topic(applicationConfigChange), hashCache)
	}
}

// copy returns a copy of the unit, ensuring appropriate deep copying.
func (a *Application) copy() Application {
	ca := *a
	ca.mu = &sync.Mutex{}
	ca.details = ca.details.copy()
	return ca
}

// topic prefixes the input string with the application name.
func (a *Application) topic(suffix string) string {
	return a.details.Name + ":" + suffix
}
