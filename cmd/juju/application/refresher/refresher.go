// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package refresher

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/charm/v8"
	"github.com/juju/charmrepo/v6"
	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"

	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/utils"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/series"
)

// ErrExhausted reveals if a refresher was exhausted in it's task. If so, then
// it is expected to attempt another refresher.
var ErrExhausted = errors.Errorf("exhausted")

// ErrAlreadyUpToDate indicates a charm is already up-to-date.
var ErrAlreadyUpToDate = errors.Errorf("already up-to-date")

// RefresherDependencies are required for any deployer to be run.
type RefresherDependencies struct {
	Authorizer    store.MacaroonGetter
	CharmAdder    store.CharmAdder
	CharmResolver CharmResolver
}

// RefresherConfig is the data required to choose a refresher and then run the
// PrepareAndUpgrade.
type RefresherConfig struct {
	ApplicationName string
	CharmURL        *charm.URL
	CharmOrigin     corecharm.Origin
	CharmRef        string
	Channel         charm.Channel
	DeployedBase    series.Base
	Force           bool
	ForceSeries     bool
	Switch          bool
	Logger          CommandLogger
}

// RefresherFn defines a function alias to create a Refresher from a given
// function.
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
		d.maybeCharmHub(deps.CharmAdder, deps.CharmResolver),
	}
	return d
}

// Run executes over a series of refreshers using a given config. It will
// execute each refresher if it's allowed, otherwise it will move on to the
// next one.
// If a refresher returns that it's exhausted (no other action to perform in
// a given task) then it will move on to the next refresher.
// If no refresher matches the config or if each one is exhausted then it will
// state that it was unable to refresh.
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
		}
		// err might be ErrAlreadyUpToDate but we still want
		// to return the charmID for updating any resources.
		return charmID, err
	}
	return nil, errors.Errorf("unable to refresh %q", cfg.CharmRef)
}

func (d *factory) maybeReadLocal(charmAdder store.CharmAdder, charmRepo CharmRepository) func(RefresherConfig) (Refresher, error) {
	return func(cfg RefresherConfig) (Refresher, error) {
		return &localCharmRefresher{
			charmAdder:   charmAdder,
			charmOrigin:  cfg.CharmOrigin,
			charmRepo:    charmRepo,
			charmURL:     cfg.CharmURL,
			charmRef:     cfg.CharmRef,
			deployedBase: cfg.DeployedBase,
			force:        cfg.Force,
			forceSeries:  cfg.ForceSeries,
		}, nil
	}
}

func (d *factory) maybeCharmStore(authorizer store.MacaroonGetter, charmAdder store.CharmAdder, charmResolver CharmResolver) func(RefresherConfig) (Refresher, error) {
	return func(cfg RefresherConfig) (Refresher, error) {
		return &charmStoreRefresher{
			baseRefresher: baseRefresher{
				charmAdder:      charmAdder,
				charmResolver:   charmResolver,
				resolveOriginFn: stdOriginResolver,
				charmURL:        cfg.CharmURL,
				charmOrigin:     cfg.CharmOrigin,
				charmRef:        cfg.CharmRef,
				channel:         cfg.Channel,
				deployedBase:    cfg.DeployedBase,
				switchCharm:     cfg.Switch,
				force:           cfg.Force,
				forceSeries:     cfg.ForceSeries,
				logger:          cfg.Logger,
			},
			authorizer: authorizer,
		}, nil
	}
}

func (d *factory) maybeCharmHub(charmAdder store.CharmAdder, charmResolver CharmResolver) func(RefresherConfig) (Refresher, error) {
	return func(cfg RefresherConfig) (Refresher, error) {
		originResolver := charmHubOriginResolver
		if cfg.Switch {
			// When switching, use the stdOriginResolver as it can
			// emit the correct origin for upgrading from cs -> ch.
			originResolver = stdOriginResolver
		}

		return &charmHubRefresher{
			baseRefresher: baseRefresher{
				charmAdder:      charmAdder,
				charmResolver:   charmResolver,
				resolveOriginFn: originResolver,
				charmURL:        cfg.CharmURL,
				charmOrigin:     cfg.CharmOrigin,
				charmRef:        cfg.CharmRef,
				channel:         cfg.Channel,
				deployedBase:    cfg.DeployedBase,
				switchCharm:     cfg.Switch,
				force:           cfg.Force,
				forceSeries:     cfg.ForceSeries,
				logger:          cfg.Logger,
			},
		}, nil
	}
}

type localCharmRefresher struct {
	charmAdder   store.CharmAdder
	charmRepo    CharmRepository
	charmOrigin  corecharm.Origin
	charmURL     *charm.URL
	charmRef     string
	deployedBase series.Base
	force        bool
	forceSeries  bool
}

// Allowed will attempt to check if a local charm is allowed to be refreshed.
// Currently this is always true.
func (d *localCharmRefresher) Allowed(_ RefresherConfig) (bool, error) {
	// We should always return true here, because of the current design.
	return true, nil
}

// Refresh a given local charm.
// Bundles are not supported as there is no physical representation in Juju.
func (d *localCharmRefresher) Refresh() (*CharmID, error) {
	var deployedSeries string
	if d.deployedBase.Name != "" {
		var err error
		deployedSeries, err = series.GetSeriesFromBase(d.deployedBase)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	ch, newURL, err := d.charmRepo.NewCharmAtPathForceSeries(d.charmRef, deployedSeries, d.forceSeries)
	if err == nil {
		newName := ch.Meta().Name
		if newName != d.charmURL.Name {
			return nil, errors.Errorf("cannot refresh %q to %q", d.charmURL.Name, newName)
		}
		addedURL, err := d.charmAdder.AddLocalCharm(newURL, ch, d.force)
		if err != nil {
			return nil, errors.Trace(err)
		}

		newOrigin := d.charmOrigin
		newOrigin.Source = corecharm.Local
		newOrigin.Channel = nil
		newOrigin.Hash = ""
		newOrigin.ID = ""
		newOrigin.Revision = &addedURL.Revision
		return &CharmID{
			URL:    addedURL,
			Origin: newOrigin,
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

	if IsLocalURL(d.charmRef) {
		// This was clearly meant to refer to a local charm, which we've not
		// been able to find, so return the error
		return nil, errors.Annotatef(err, "%q", d.charmRef)
	}

	// Not a valid local charm, in this case, we should move onto the next
	// refresher.
	return nil, ErrExhausted
}

// IsLocalURL checks if the provided URL refers to a local charm (i.e. it
// begins with one of  `/`  `./`  `../` ).
func IsLocalURL(url string) bool {
	return strings.HasPrefix(url, "/") || strings.HasPrefix(url, "./") ||
		strings.HasPrefix(url, "../")
}

func (d *localCharmRefresher) String() string {
	return fmt.Sprintf("attempting to refresh local charm %q", d.charmRef)
}

// ResolveOriginFunc attempts to resolve a new charm Origin from the given
// arguments, ensuring that we work for multiple stores (charmhub vs charmstore)
type ResolveOriginFunc = func(*charm.URL, corecharm.Origin, charm.Channel) (commoncharm.Origin, error)

type baseRefresher struct {
	charmAdder      store.CharmAdder
	charmResolver   CharmResolver
	resolveOriginFn ResolveOriginFunc
	charmURL        *charm.URL
	charmOrigin     corecharm.Origin
	charmRef        string
	channel         charm.Channel
	deployedBase    series.Base
	switchCharm     bool
	force           bool
	forceSeries     bool
	logger          CommandLogger
}

func (r baseRefresher) ResolveCharm() (*charm.URL, commoncharm.Origin, error) {
	if r.charmOrigin.Channel != nil {
		r.logger.Verbosef("Original channel %q", r.charmOrigin.Channel.String())
	}
	r.logger.Verbosef("Requested channel %q", r.channel.String())

	refURL, err := charm.ParseURL(r.charmRef)
	if err != nil {
		return nil, commoncharm.Origin{}, errors.Trace(err)
	}

	// Take the current origin and apply the user supplied channel, so that
	// when attempting to resolve the new charm URL, we pick up everything
	// that already exists, but we get the destination of where the user wants
	// to get to.
	destOrigin, err := r.resolveOriginFn(refURL, r.charmOrigin, r.channel)
	if err != nil {
		return nil, commoncharm.Origin{}, errors.Trace(err)
	}

	// Charm has been supplied as a URL so we resolve and deploy using the store.
	newURL, origin, supportedSeries, err := r.charmResolver.ResolveCharm(refURL, destOrigin, r.switchCharm)
	if err != nil {
		return nil, commoncharm.Origin{}, errors.Trace(err)
	}

	var deployedSeries string
	if r.deployedBase.Name != "" {
		deployedSeries, err = series.GetSeriesFromBase(r.deployedBase)
		if err != nil {
			return nil, commoncharm.Origin{}, errors.Trace(err)
		}
	}
	_, seriesSupportedErr := charm.SeriesForCharm(deployedSeries, supportedSeries)
	if !r.forceSeries && deployedSeries != "" && newURL.Series == "" && seriesSupportedErr != nil {
		series := []string{"no series"}
		if len(supportedSeries) > 0 {
			series = supportedSeries
		}
		return nil, commoncharm.Origin{}, errors.Errorf(
			"cannot upgrade from single series %q charm to a charm supporting %q. Use --force-series to override.",
			deployedSeries, series,
		)
	}

	// If no explicit revision was set with either SwitchURL
	// or Revision flags, discover the latest.
	if r.charmURL == nil {
		return nil, origin, errors.Errorf("unexpected charm URL")
	}
	var resolveErr error
	if *newURL == *r.charmURL {
		// Charm is uptodate so we return a suitable error.
		// We also want to still return the URL and origin as these
		// are used updating any resources.
		if refURL.Revision != -1 {
			resolveErr = errors.Annotatef(ErrAlreadyUpToDate,
				"charm %q, revision %d", newURL.Name, newURL.Revision)
		} else {
			// No point in trying to upgrade a charm store charm when
			// we just determined that's the latest revision
			// available.
			resolveErr = errors.Annotatef(ErrAlreadyUpToDate,
				"charm %q", newURL.Name)
		}
	}
	r.logger.Verbosef("Using channel %q", origin.CharmChannel().String())
	return newURL, origin, resolveErr
}

// stdOriginResolver attempts to resolve the origin required to resolve a
// charm. It works not only with charmstore charms but it also encapsulates the
// required logic to deduce the appropriate origin for a charmstore to charmhub
// switch.
func stdOriginResolver(curl *charm.URL, origin corecharm.Origin, channel charm.Channel) (commoncharm.Origin, error) {
	result, err := utils.DeduceOrigin(curl, channel, origin.Platform)
	if err != nil {
		return commoncharm.Origin{}, errors.Trace(err)
	}
	return result, nil
}

type charmStoreRefresher struct {
	baseRefresher
	authorizer store.MacaroonGetter
}

// Allowed will attempt to check if the charm store is allowed to refresh.
// Depending on the charm url, will then determine if that's true or not.
func (r *charmStoreRefresher) Allowed(cfg RefresherConfig) (bool, error) {
	path, err := charm.EnsureSchema(cfg.CharmRef, charm.CharmStore)
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
	return true, nil
}

// Refresh a given charm store charm.
// Bundles are not supported as there is no physical representation in Juju.
func (r *charmStoreRefresher) Refresh() (*CharmID, error) {
	newURL, origin, err := r.ResolveCharm()
	if errors.Is(err, ErrAlreadyUpToDate) {
		// The charm itself is uptodate but we may need the
		// URL, origin and macaroon (if there is one)
		// for updating resources.
		csMac, csErr := store.AuthorizeCharmStoreEntity(r.authorizer, newURL)
		if csErr != nil && !strings.Contains(csErr.Error(), "404 NOT FOUND") {
			return nil, errors.Trace(csErr)
		}
		return &CharmID{
			URL:      newURL,
			Origin:   origin.CoreCharmOrigin(),
			Macaroon: csMac,
		}, err
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	if r.deployedBase.Name != "" {
		origin.Series, err = series.GetSeriesFromBase(r.deployedBase)
		if err != nil {
			return nil, errors.Trace(err)
		}
		origin.Base = r.deployedBase
	}

	curl, csMac, _, err := store.AddCharmWithAuthorizationFromURL(r.charmAdder, r.authorizer, newURL, origin, r.force)
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
	return corecharm.NewCharmAtPathForceSeries(path, series, force)
}

// charmHubOriginResolver attempts to resolve the origin required to resolve
// a charm. It does this by updating the incoming origin with the user requested
// channel, so we can correctly resolve the charm.
func charmHubOriginResolver(_ *charm.URL, origin corecharm.Origin, channel charm.Channel) (commoncharm.Origin, error) {
	if channel.Track == "" {
		if origin.Channel != nil {
			origin.Channel.Risk = channel.Risk
		}
		return commoncharm.CoreCharmOrigin(origin)
	}
	normalizedC := channel.Normalize()
	origin.Channel = &normalizedC
	return commoncharm.CoreCharmOrigin(origin)
}

type charmHubRefresher struct {
	baseRefresher
}

// Allowed will attempt to check if the charm store is allowed to refresh.
// Depending on the charm url, will then determine if that's true or not.
func (r *charmHubRefresher) Allowed(cfg RefresherConfig) (bool, error) {
	path, err := charm.EnsureSchema(cfg.CharmRef, charm.CharmHub)
	if err != nil {
		return false, errors.Trace(err)
	}

	curl, err := charm.ParseURL(path)
	if err != nil {
		return false, errors.Trace(err)
	}

	if !charm.CharmHub.Matches(curl.Schema) {
		return false, nil
	}

	if !cfg.Switch {
		return true, nil
	}

	if err := r.charmAdder.CheckCharmPlacement(cfg.ApplicationName, curl); err != nil && !errors.IsNotSupported(err) {
		// If force is used then ignore the error, the user seems to know
		// what they're doing.
		if !cfg.Force {
			return false, errors.Trace(err)
		}
		r.logger.Warningf("Charm placement check failed, using --force may break deployment")
	}

	return true, nil
}

// Refresh a given charm hub charm.
// Bundles are not supported as there is no physical representation in Juju.
func (r *charmHubRefresher) Refresh() (*CharmID, error) {
	newURL, origin, err := r.ResolveCharm()
	if errors.Is(err, ErrAlreadyUpToDate) {
		// The charm itself is up-to-date but we may need the
		// URL and origin for updating resources.
		return &CharmID{
			URL:    newURL,
			Origin: origin.CoreCharmOrigin(),
		}, err
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	if r.deployedBase.Name != "" {
		origin.Series, err = series.GetSeriesFromBase(r.deployedBase)
		if err != nil {
			return nil, errors.Trace(err)
		}
		origin.Base = r.deployedBase
	}

	curl, actualOrigin, err := store.AddCharmFromURL(r.charmAdder, newURL, origin, r.force)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &CharmID{
		URL:    curl,
		Origin: actualOrigin.CoreCharmOrigin(),
	}, nil
}

func (r *charmHubRefresher) String() string {
	return fmt.Sprintf("attempting to refresh charm hub charm %q", r.charmRef)
}
