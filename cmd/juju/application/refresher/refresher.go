// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package refresher

import (
	"fmt"
	"os"

	"github.com/juju/charm/v8"
	"github.com/juju/charmrepo/v6"
	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/featureflag"

	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/utils"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/feature"
)

// ErrExhausted reveals if a refresher was exhausted in it's task. If so, then
// it is expected to attempt another refresher.
var ErrExhausted = errors.Errorf("exhausted")

// RefresherDependencies are required for any deployer to be run.
type RefresherDependencies struct {
	Authorizer    store.MacaroonGetter
	CharmAdder    store.CharmAdder
	CharmResolver CharmResolver
}

// RefresherConfig is the data required to choose an refresher and then run the
// PrepareAndUpgrade.
type RefresherConfig struct {
	ApplicationName string
	CharmURL        *charm.URL
	CharmRef        string
	Channel         corecharm.Channel
	DeployedSeries  string
	Force           bool
	ForceSeries     bool
}

type RefresherFn = func(RefresherConfig) (Refresher, error)

type factory struct {
	refreshers []RefresherFn
	clock      jujuclock.Clock
}

// NewRefresherFactory returns a factory setup with the API and
// function dependencies required by every refresher.
func NewRefresherFactory(deps RefresherDependencies) RefresherFactory {
	d := &factory{
		clock: jujuclock.WallClock,
	}
	d.refreshers = []RefresherFn{
		d.maybeReadLocal(deps.CharmAdder, defaultCharmRepo{}),
		d.maybeCharmStore(deps.Authorizer, deps.CharmAdder, deps.CharmResolver),
	}
	return d
}

// GetRefresher returns the correct refresher to use based on the cfg provided.
func (d *factory) Run(cfg RefresherConfig) (*CharmID, error) {
	for _, fn := range d.refreshers {
		// Failure to correctly setup a refresher will call all of the
		// refreshers to fail.
		refresh, err := fn(cfg)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// If a refresher doesn't allow the config, then continue to the next
		// one.
		if allowed, err := refresh.Allowed(cfg); err != nil {
			return nil, errors.Trace(err)
		} else if !allowed {
			continue
		}

		charmID, err := refresh.Refresh()
		// We've exhausted this refresh task, attempt another one.
		if errors.Cause(err) == ErrExhausted {
			continue
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		// If there isn't an error, then it's expected that we found what we
		// where looking for.
		return charmID, nil
	}
	return nil, errors.Errorf("unable to refresh %q", cfg.CharmRef)
}

func (d *factory) maybeReadLocal(charmAdder store.CharmAdder, charmRepo CharmRepository) func(RefresherConfig) (Refresher, error) {
	return func(cfg RefresherConfig) (Refresher, error) {
		return &localCharmRefresher{
			charmAdder:     charmAdder,
			charmRepo:      charmRepo,
			charmURL:       cfg.CharmURL,
			charmRef:       cfg.CharmRef,
			deployedSeries: cfg.DeployedSeries,
			force:          cfg.Force,
			forceSeries:    cfg.ForceSeries,
		}, nil
	}
}

func (d *factory) maybeCharmStore(authorizer store.MacaroonGetter, charmAdder store.CharmAdder, charmResolver CharmResolver) func(RefresherConfig) (Refresher, error) {
	return func(cfg RefresherConfig) (Refresher, error) {
		return &charmStoreRefresher{
			authorizer:     authorizer,
			charmAdder:     charmAdder,
			charmResolver:  charmResolver,
			charmURL:       cfg.CharmURL,
			charmRef:       cfg.CharmRef,
			channel:        cfg.Channel,
			deployedSeries: cfg.DeployedSeries,
			force:          cfg.Force,
			forceSeries:    cfg.ForceSeries,
		}, nil
	}
}

type localCharmRefresher struct {
	charmAdder     store.CharmAdder
	charmRepo      CharmRepository
	charmURL       *charm.URL
	charmRef       string
	deployedSeries string
	force          bool
	forceSeries    bool
}

func (d *localCharmRefresher) Allowed(cfg RefresherConfig) (bool, error) {
	// We should always return true here, because of the current design.
	return true, nil
}

func (d *localCharmRefresher) Refresh() (*CharmID, error) {
	ch, newURL, err := d.charmRepo.NewCharmAtPathForceSeries(d.charmRef, d.deployedSeries, d.forceSeries)
	if err == nil {
		newName := ch.Meta().Name
		if newName != d.charmURL.Name {
			return nil, errors.Errorf("cannot refresh %q to %q", d.charmURL.Name, newName)
		}
		addedURL, err := d.charmAdder.AddLocalCharm(newURL, ch, d.force)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return &CharmID{
			URL: addedURL,
		}, nil
	}
	if _, ok := err.(*charmrepo.NotFoundError); ok {
		return nil, errors.Errorf("no charm found at %q", d.charmRef)
	}
	// If we get a "not exists" or invalid path error then we attempt to interpret
	// the supplied charm reference as a URL below, otherwise we return the error.
	if err != os.ErrNotExist && !charmrepo.IsInvalidPathError(err) {
		return nil, errors.Trace(err)
	}

	// Not a valid local charm, in this case, we should move onto the next
	// refresher.
	return nil, ErrExhausted
}

func (d *localCharmRefresher) String() string {
	return fmt.Sprintf("attempting to refresh local charm %q", d.charmRef)
}

type charmStoreRefresher struct {
	authorizer     store.MacaroonGetter
	charmAdder     store.CharmAdder
	charmResolver  CharmResolver
	charmURL       *charm.URL
	charmRef       string
	channel        corecharm.Channel
	deployedSeries string
	force          bool
	forceSeries    bool
}

func (r *charmStoreRefresher) Allowed(cfg RefresherConfig) (bool, error) {
	// If we're a charm hub charm reference, then skip the charm store and
	// move onto the next
	if featureflag.Enabled(feature.CharmHubIntegration) {
		path, err := charm.EnsureSchema(cfg.CharmRef)
		if err != nil {
			return false, errors.Trace(err)
		}

		curl, err := charm.ParseURL(path)
		if err != nil {
			return false, errors.Trace(err)
		}

		if charm.CharmHub.Matches(curl.Schema) {
			return false, nil
		}
	}
	return true, nil
}

func (r *charmStoreRefresher) Refresh() (*CharmID, error) {
	refURL, err := charm.ParseURL(r.charmRef)
	if err != nil {
		return nil, errors.Trace(err)
	}

	origin, err := utils.DeduceOrigin(refURL, r.channel)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Charm has been supplied as a URL so we resolve and deploy using the store.
	newURL, origin, supportedSeries, err := r.charmResolver.ResolveCharm(refURL, origin)
	if err != nil {
		return nil, errors.Trace(err)
	}

	_, seriesSupportedErr := charm.SeriesForCharm(r.deployedSeries, supportedSeries)
	if !r.forceSeries && r.deployedSeries != "" && newURL.Series == "" && seriesSupportedErr != nil {
		series := []string{"no series"}
		if len(supportedSeries) > 0 {
			series = supportedSeries
		}
		return nil, errors.Errorf(
			"cannot upgrade from single series %q charm to a charm supporting %q. Use --force-series to override.",
			r.deployedSeries, series,
		)
	}

	// If no explicit revision was set with either SwitchURL
	// or Revision flags, discover the latest.
	if *newURL == *r.charmURL {
		if refURL.Revision != -1 {
			return nil, errors.Errorf("already running specified charm %q", newURL)
		}
		// No point in trying to upgrade a charm store charm when
		// we just determined that's the latest revision
		// available.
		return nil, errors.Errorf("already running latest charm %q", newURL)
	}

	curl, csMac, _, err := store.AddCharmFromURL(r.charmAdder, r.authorizer, newURL, origin, r.force)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &CharmID{
		URL:      curl,
		Origin:   origin.CoreCharmOrigin(),
		Macaroon: csMac,
	}, nil
}

func (r *charmStoreRefresher) String() string {
	return fmt.Sprintf("attempting to refresh charm store charm %q", r.charmRef)
}

type defaultCharmRepo struct{}

func (defaultCharmRepo) NewCharmAtPathForceSeries(path, series string, force bool) (charm.Charm, *charm.URL, error) {
	return charmrepo.NewCharmAtPathForceSeries(path, series, force)
}
