// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package refresher

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/charm/v11"
	jujuclock "github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/utils"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
)

// ErrExhausted reveals if a refresher was exhausted in it's task. If so, then
// it is expected to attempt another refresher.
var ErrExhausted = errors.Errorf("exhausted")

// ErrAlreadyUpToDate indicates a charm is already up-to-date.
var ErrAlreadyUpToDate = errors.Errorf("already up-to-date")

// RefresherDependencies are required for any deployer to be run.
type RefresherDependencies struct {
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
	Force           bool
	ForceBase       bool
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
			charmAdder:  charmAdder,
			charmOrigin: cfg.CharmOrigin,
			charmRepo:   charmRepo,
			charmURL:    cfg.CharmURL,
			charmRef:    cfg.CharmRef,
			force:       cfg.Force,
			forceBase:   cfg.ForceBase,
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
				switchCharm:     cfg.Switch,
				force:           cfg.Force,
				forceBase:       cfg.ForceBase,
				logger:          cfg.Logger,
			},
		}, nil
	}
}

type localCharmRefresher struct {
	charmAdder  store.CharmAdder
	charmRepo   CharmRepository
	charmOrigin corecharm.Origin
	charmURL    *charm.URL
	charmRef    string
	force       bool
	forceBase   bool
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
	deployedBase, err := corebase.ParseBase(d.charmOrigin.Platform.OS, d.charmOrigin.Platform.Channel)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ch, newURL, err := d.charmRepo.NewCharmAtPathForceBase(d.charmRef, deployedBase, d.forceBase)
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
	if errors.Is(err, errors.NotFound) {
		return nil, errors.Errorf("no charm found at %q", d.charmRef)
	}
	// If we get a "not exists" or invalid path error then we attempt to interpret
	// the supplied charm reference as a URL below, otherwise we return the error.
	if err != os.ErrNotExist && !corecharm.IsInvalidPathError(err) {
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
// arguments
type ResolveOriginFunc = func(*charm.URL, corecharm.Origin, charm.Channel) (commoncharm.Origin, error)

type baseRefresher struct {
	charmAdder      store.CharmAdder
	charmResolver   CharmResolver
	resolveOriginFn ResolveOriginFunc
	charmURL        *charm.URL
	charmOrigin     corecharm.Origin
	charmRef        string
	channel         charm.Channel
	switchCharm     bool
	force           bool
	forceBase       bool
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
	newURL, origin, supportedBases, err := r.charmResolver.ResolveCharm(refURL, destOrigin, r.switchCharm)
	if err != nil {
		return nil, commoncharm.Origin{}, errors.Trace(err)
	}
	deployedBase, err := corebase.ParseBase(r.charmOrigin.Platform.OS, r.charmOrigin.Platform.Channel)
	if err != nil {
		return nil, commoncharm.Origin{}, errors.Trace(err)
	}
	_, baseSupportedErr := corecharm.BaseForCharm(deployedBase, supportedBases)
	if !r.forceBase && !deployedBase.Empty() && newURL.Series == "" && baseSupportedErr != nil {
		bases := []string{"no bases"}
		if len(supportedBases) > 0 {
			bases = transform.Slice(supportedBases, func(in corebase.Base) string { return in.DisplayString() })
		}
		return nil, commoncharm.Origin{}, errors.Errorf(
			"cannot upgrade from single base %q charm to a charm supporting %q. Use --force-series to override.",
			deployedBase.DisplayString(), bases,
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
// charm.
func stdOriginResolver(curl *charm.URL, origin corecharm.Origin, channel charm.Channel) (commoncharm.Origin, error) {
	result, err := utils.DeduceOrigin(curl, channel, origin.Platform)
	if err != nil {
		return commoncharm.Origin{}, errors.Trace(err)
	}
	return result, nil
}

type defaultCharmRepo struct{}

func (defaultCharmRepo) NewCharmAtPathForceBase(path string, base corebase.Base, force bool) (charm.Charm, *charm.URL, error) {
	return corecharm.NewCharmAtPathForceBase(path, base, force)
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

// Allowed will attempt to check if the charm is allowed to refresh.
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
	return fmt.Sprintf("attempting to refresh Charmhub charm %q", r.charmRef)
}
