// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"regexp"
	"sort"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/environs"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// State represents a type for interacting with the underlying state.
type State interface {
	ApplicationState
	CharmState
	StorageState
	UnitState
	MigrationState
}

const (
	// applicationSnippet is a non-compiled regexp that can be composed with
	// other snippets to form a valid application regexp.
	applicationSnippet = "(?:[a-z][a-z0-9]*(?:-[a-z0-9]*[a-z][a-z0-9]*)*)"
)

var (
	validApplication = regexp.MustCompile("^" + applicationSnippet + "$")
)

// Service provides the API for working with applications.
type Service struct {
	st            State
	leaderEnsurer leadership.Ensurer
	logger        logger.Logger
	clock         clock.Clock

	storageRegistryGetter corestorage.ModelStorageRegistryGetter
	charmStore            CharmStore
	statusHistory         StatusHistory
}

// NewService returns a new service reference wrapping the input state.
func NewService(
	st State,
	leaderEnsurer leadership.Ensurer,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	charmStore CharmStore,
	statusHistory StatusHistory,
	clock clock.Clock,
	logger logger.Logger,
) *Service {
	return &Service{
		st:                    st,
		leaderEnsurer:         leaderEnsurer,
		logger:                logger,
		clock:                 clock,
		storageRegistryGetter: storageRegistryGetter,
		charmStore:            charmStore,
		statusHistory:         statusHistory,
	}
}

// AgentVersionGetter is responsible for retrieving the target
// agent version for the current model.
type AgentVersionGetter interface {
	// GetModelTargetAgentVersion returns the agent version
	// for the current model.
	GetModelTargetAgentVersion(context.Context) (semversion.Number, error)
}

// Provider defines the interface for interacting with the underlying model
// provider.
type Provider interface {
	environs.ConstraintsChecker
}

// SupportedFeatureProvider defines the interface for interacting with the
// a model provider that satisfies the SupportedFeatureEnumerator interface.
type SupportedFeatureProvider interface {
	environs.SupportedFeatureEnumerator
}

// CAASApplicationProvider contains methods from the caas.Broker interface
// used by the application provider service.
type CAASApplicationProvider interface {
	Application(string, caas.DeploymentType) caas.Application
}

// WatcherFactory instances return watchers for a given namespace and UUID.
type WatcherFactory interface {
	// NewUUIDsWatcher returns a watcher that emits the UUIDs for changes to the
	// input table name that match the input mask.
	NewUUIDsWatcher(
		namespace string, changeMask changestream.ChangeType,
	) (watcher.StringsWatcher, error)

	NewNotifyWatcher(
		filterOption eventsource.FilterOption,
		filterOptions ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)

	NewNotifyMapperWatcher(
		mapper eventsource.Mapper,
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)

	// NewNamespaceMapperWatcher returns a new watcher that receives changes
	// from the input base watcher's db/queue. Change-log events will be emitted
	// only if the filter accepts them, and dispatching the notifications via
	// the Changes channel, once the mapper has processed them. Filtering of
	// values is done first by the filter, and then by the mapper. Based on the
	// mapper's logic a subset of them (or none) may be emitted. A filter option
	// is required, though additional filter options can be provided.
	NewNamespaceMapperWatcher(
		initialStateQuery eventsource.NamespaceQuery,
		mapper eventsource.Mapper,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)
}

// WatchableService provides the API for working with applications and the
// ability to create watchers.
type WatchableService struct {
	*ProviderService
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new service reference wrapping the input state.
func NewWatchableService(
	st State,
	leaderEnsurer leadership.Ensurer,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	modelID coremodel.UUID,
	watcherFactory WatcherFactory,
	agentVersionGetter AgentVersionGetter,
	provider providertracker.ProviderGetter[Provider],
	supportedFeatureProvider providertracker.ProviderGetter[SupportedFeatureProvider],
	caasApplicationProvider providertracker.ProviderGetter[CAASApplicationProvider],
	charmStore CharmStore,
	statusHistory StatusHistory,
	clock clock.Clock,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		ProviderService: NewProviderService(
			st,
			leaderEnsurer,
			storageRegistryGetter,
			modelID,
			agentVersionGetter,
			provider,
			supportedFeatureProvider,
			caasApplicationProvider,
			charmStore,
			statusHistory,
			clock,
			logger,
		),
		watcherFactory: watcherFactory,
	}
}

// WatchApplicationUnitLife returns a watcher that observes changes to the life of any units if an application.
func (s *WatchableService) WatchApplicationUnitLife(appName string) (watcher.StringsWatcher, error) {
	lifeGetter := func(ctx context.Context, ids []string) (map[string]life.Life, error) {
		unitUUIDs, err := transform.SliceOrErr(ids, coreunit.ParseID)
		if err != nil {
			return nil, err
		}
		lives, err := s.st.GetApplicationUnitLife(ctx, appName, unitUUIDs...)
		if err != nil {
			return nil, err
		}
		result := make(map[string]life.Life, len(lives))
		for unitUUID, life := range lives {
			result[unitUUID.String()] = life
		}
		return result, nil
	}
	lifeMapper := domain.LifeStringsWatcherMapperFunc(s.logger, lifeGetter)

	table, query := s.st.InitialWatchStatementUnitLife(appName)
	return s.watcherFactory.NewNamespaceMapperWatcher(
		query, lifeMapper,
		eventsource.NamespaceFilter(table, changestream.All),
	)
}

// WatchApplicationScale returns a watcher that observes changes to an application's scale.
func (s *WatchableService) WatchApplicationScale(ctx context.Context, appName string) (watcher.NotifyWatcher, error) {
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	scaleState, err := s.st.GetApplicationScaleState(ctx, appID)
	if err != nil {
		return nil, errors.Errorf("getting scaling state for %q: %w", appName, err)
	}
	currentScale := scaleState.Scale

	mask := changestream.Changed
	mapper := func(ctx context.Context, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
		newScaleState, err := s.st.GetApplicationScaleState(ctx, appID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		newScale := newScaleState.Scale
		// Only dispatch if the scale has changed.
		if newScale != currentScale {
			currentScale = newScale
			return changes, nil
		}
		return nil, nil
	}
	return s.watcherFactory.NewNotifyMapperWatcher(
		mapper,
		eventsource.PredicateFilter(
			s.st.NamespaceForWatchApplicationScale(),
			mask,
			eventsource.EqualsPredicate(appID.String()),
		),
	)
}

// WatchApplicationsWithPendingCharms returns a watcher that observes changes to
// applications that have pending charms.
func (s *WatchableService) WatchApplicationsWithPendingCharms(ctx context.Context) (watcher.StringsWatcher, error) {
	table, query := s.st.InitialWatchStatementApplicationsWithPendingCharms()
	return s.watcherFactory.NewNamespaceMapperWatcher(
		query,
		func(ctx context.Context, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
			return s.watchApplicationsWithPendingCharmsMapper(ctx, changes)
		},
		eventsource.NamespaceFilter(table, changestream.Changed),
	)
}

// watchApplicationsWithPendingCharmsMapper removes any applications that do not
// have pending charms.
func (s *WatchableService) watchApplicationsWithPendingCharmsMapper(ctx context.Context, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
	// Preserve the ordering of the changes, as this is a strings watcher
	// and we want to return the changes in the order they were received.

	appChanges := make(map[coreapplication.ID][]indexedChanged)
	uuids := make([]coreapplication.ID, 0)
	for i, change := range changes {
		appID, err := coreapplication.ParseID(change.Changed())
		if err != nil {
			return nil, err
		}

		if _, ok := appChanges[appID]; !ok {
			uuids = append(uuids, appID)
		}

		appChanges[appID] = append(appChanges[appID], indexedChanged{
			change: change,
			index:  i,
		})
	}

	// Get all the applications with pending charms using the uuids.
	apps, err := s.GetApplicationsWithPendingCharmsFromUUIDs(ctx, uuids)
	if err != nil {
		return nil, err
	}

	// If any applications have no pending charms, then return no changes.
	if len(apps) == 0 {
		return nil, nil
	}

	// Grab all the changes for the applications with pending charms,
	// ensuring they're indexed so we can sort them later.
	var indexed []indexedChanged
	for _, appID := range apps {
		events, ok := appChanges[appID]
		if !ok {
			s.logger.Errorf(ctx, "application %q has pending charms but no change events", appID)
			continue
		}

		indexed = append(indexed, events...)
	}

	// Sort the index so they're preserved
	sort.Slice(indexed, func(i, j int) bool {
		return indexed[i].index < indexed[j].index
	})

	// Grab the changes in the order they were received.
	var results []changestream.ChangeEvent
	for _, result := range indexed {
		results = append(results, result.change)
	}

	return results, nil
}

type indexedChanged struct {
	change changestream.ChangeEvent
	index  int
}

// WatchApplication watches for changes to the specified application in the
// application table.
// If the application does not exist an error satisfying
// [applicationerrors.NotFound] will be returned.
func (s *WatchableService) WatchApplication(ctx context.Context, name string) (watcher.NotifyWatcher, error) {
	uuid, err := s.GetApplicationIDByName(ctx, name)
	if err != nil {
		return nil, errors.Errorf("getting ID of application %s: %w", name, err)
	}
	return s.watcherFactory.NewNotifyWatcher(
		eventsource.PredicateFilter(
			s.st.NamespaceForWatchApplication(),
			changestream.All,
			eventsource.EqualsPredicate(uuid.String()),
		),
	)
}

// WatchApplicationConfig watches for changes to the specified application's
// config.
// This notifies on any changes to the application's config, which is driven
// of the application config hash. It is up to the caller to determine if the
// config value they're interested in has changed.
//
// If the application does not exist an error satisfying
// [applicationerrors.NotFound] will be returned.
func (s *WatchableService) WatchApplicationConfig(ctx context.Context, name string) (watcher.NotifyWatcher, error) {
	uuid, err := s.GetApplicationIDByName(ctx, name)
	if err != nil {
		return nil, errors.Errorf("getting ID of application %s: %w", name, err)
	}

	return s.watcherFactory.NewNotifyWatcher(
		eventsource.PredicateFilter(
			s.st.NamespaceForWatchApplicationConfig(),
			changestream.All,
			eventsource.EqualsPredicate(uuid.String()),
		),
	)
}

// WatchApplicationConfigHash watches for changes to the specified application's
// config hash.
// This notifies on any changes to the application's config hash, which is
// driven of the application config hash. It is up to the caller to determine
// if the config value they're interested in has changed. This watcher is
// the backing for the uniter's remote state. We should be attempting to
// remove this in the future.
//
// If the application does not exist an error satisfying
// [applicationerrors.NotFound] will be returned.
func (s *WatchableService) WatchApplicationConfigHash(ctx context.Context, name string) (watcher.StringsWatcher, error) {
	appID, err := s.GetApplicationIDByName(ctx, name)
	if err != nil {
		return nil, errors.Errorf("getting ID of application %s: %w", name, err)
	}

	// sha256 is the current config hash for the application. This will
	// be filled in by the initial query. If it's empty after the initial
	// query, then a new config hash will be generated on the first change.
	var sha256 string

	table, query := s.st.InitialWatchStatementApplicationConfigHash(name)
	return s.watcherFactory.NewNamespaceMapperWatcher(
		func(ctx context.Context, txn database.TxnRunner) ([]string, error) {
			initialResults, err := query(ctx, txn)
			if err != nil {
				return nil, errors.Capture(err)
			}

			if num := len(initialResults); num > 1 {
				return nil, errors.Errorf("too many config hashes for application %q", name)
			} else if num == 1 {
				sha256 = initialResults[0]
			}

			return initialResults, nil
		},
		func(ctx context.Context, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
			// If there are no changes, return no changes.
			if len(changes) == 0 {
				return nil, nil
			}

			currentSHA256, err := s.st.GetApplicationConfigHash(ctx, appID)
			if err != nil {
				return nil, errors.Capture(err)
			}
			// If the hash hasn't changed, return no changes. The first sha256
			// might be empty, so if that's the case the currentSHA256 will not
			// be empty. Either way we'll only return changes if the hash has
			// changed.
			if currentSHA256 == sha256 {
				return nil, nil
			}
			sha256 = currentSHA256

			// There can be only one.
			// Select the last change event, which will be naturally ordered
			// by the grouping of the query (CREATE, UPDATE, DELETE).
			change := changes[len(changes)-1]

			return []changestream.ChangeEvent{
				newMaskedChangeIDEvent(change, sha256),
			}, nil
		},
		eventsource.NamespaceFilter(table, changestream.All),
	)
}

// WatchUnitAddressesHash watches for changes to the specified unit's
// addresses hash, as well as changes to the endpoint bindings for the spaces
// the addresses belong to.
//
// If the unit does not exist an error satisfying [applicationerrors.UnitNotFound]
// will be returned.
func (s *WatchableService) WatchUnitAddressesHash(ctx context.Context, unitName coreunit.Name) (watcher.StringsWatcher, error) {
	appUUID, err := s.st.GetApplicationIDByUnitName(ctx, unitName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// currentHash is the current hash. This will be filled in by the initial
	// query.
	// If it's empty after the initial query, then a new address hash will be
	// generated on the first change.
	var currentHash string

	// Retrieve the net node uuid that corresponds to the cloud service and if
	// there isn't one, then the unit's net node.
	netNodeUUID, err := s.st.GetNetNodeUUIDByUnitName(ctx, unitName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ipAddressTable, appEndpointTable, query := s.st.InitialWatchStatementUnitAddressesHash(appUUID, netNodeUUID)
	return s.watcherFactory.NewNamespaceMapperWatcher(
		func(ctx context.Context, txn database.TxnRunner) ([]string, error) {
			initialResults, err := query(ctx, txn)
			if err != nil {
				return nil, errors.Capture(err)
			}

			if num := len(initialResults); num > 1 {
				return nil, errors.Errorf("too many address hashes for unit %q", unitName)
			} else if num == 1 {
				currentHash = initialResults[0]
			}

			return initialResults, nil
		},
		func(ctx context.Context, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
			// If there are no changes, return no changes.
			if len(changes) == 0 {
				return nil, nil
			}

			newHash, err := s.st.GetAddressesHash(ctx, appUUID, netNodeUUID)
			if err != nil {
				return nil, errors.Capture(err)
			}
			// If the hash hasn't changed, return no changes. The first hash
			// might be empty, so if that's the case the new hash will not
			// be empty. Either way we'll only return changes if the hash has
			// changed.
			if newHash == currentHash {
				return nil, nil
			}
			currentHash = newHash

			// There can be only one.
			// Select the last change event, which will be naturally ordered
			// by the grouping of the query (CREATE, UPDATE, DELETE).
			change := changes[len(changes)-1]

			return []changestream.ChangeEvent{
				newMaskedChangeIDEvent(change, currentHash),
			}, nil
		},
		eventsource.NamespaceFilter(ipAddressTable, changestream.All),
		eventsource.NamespaceFilter(appEndpointTable, changestream.All),
	)
}

// WatchApplicationExposed watches for changes to the specified application's
// exposed endpoints.
// This notifies on any changes to the application's exposed endpoints. It is up
// to the caller to determine if the exposed endpoints they're interested in has
// changed.
//
// If the application does not exist an error satisfying
// [applicationerrors.NotFound] will be returned.
func (s *WatchableService) WatchApplicationExposed(ctx context.Context, name string) (watcher.NotifyWatcher, error) {
	uuid, err := s.GetApplicationIDByName(ctx, name)
	if err != nil {
		return nil, errors.Errorf("getting ID of application %s: %w", name, err)
	}

	exposedToSpaces, exposedToCIDRs := s.st.NamespaceForWatchApplicationExposed()
	return s.watcherFactory.NewNotifyWatcher(
		eventsource.PredicateFilter(
			exposedToSpaces,
			changestream.All,
			eventsource.EqualsPredicate(uuid.String()),
		),
		eventsource.PredicateFilter(
			exposedToCIDRs,
			changestream.All,
			eventsource.EqualsPredicate(uuid.String()),
		),
	)
}

// WatchUnitForUniterChanged watches for some specific changes to the unit with
// the given name. The watcher will emit a notification when there is a change to
// the unit's inherent properties, it's subordinates or it's resolved mode.
//
// If the unit does not exist an error satisfying [applicationerrors.UnitNotFound]
// will be returned.
//
// These tables are selected since they provide coverage for the events the uniter
// watches for using the Watch agent facade method.
//
// TODO(jack-w-shaw): This watcher only exists to maintain backwards compatibility
// with the uniter agent facade. Specifically, version 20 of the facade implements
// a Watch endpoint, which can watches for _any_ change to the unit doc in Mongo.
// Once we no longer need to support facade 20, we can drop this method.
func (s *WatchableService) WatchUnitForLegacyUniter(ctx context.Context, unitName coreunit.Name) (watcher.NotifyWatcher, error) {
	uuid, err := s.GetUnitUUID(ctx, unitName)
	if err != nil {
		return nil, errors.Errorf("getting ID of unit %s: %w", unitName, err)
	}

	unitNamespace, principalNamespace, resolvedNamespace := s.st.NamespaceForWatchUnitForLegacyUniter()
	return s.watcherFactory.NewNotifyWatcher(
		eventsource.PredicateFilter(
			unitNamespace,
			changestream.All,
			eventsource.EqualsPredicate(uuid.String()),
		),
		eventsource.PredicateFilter(
			principalNamespace,
			changestream.All,
			eventsource.EqualsPredicate(uuid.String()),
		),
		eventsource.PredicateFilter(
			resolvedNamespace,
			changestream.All,
			eventsource.EqualsPredicate(uuid.String()),
		),
	)
}

type maskedChangeIDEvent struct {
	changestream.ChangeEvent
	id string
}

func newMaskedChangeIDEvent(change changestream.ChangeEvent, id string) changestream.ChangeEvent {
	return maskedChangeIDEvent{
		ChangeEvent: change,
		id:          id,
	}
}

func (m maskedChangeIDEvent) Changed() string {
	return m.id
}

// isValidApplicationName returns whether name is a valid application name.
func isValidApplicationName(name string) bool {
	return validApplication.MatchString(name)
}

// isValidReferenceName returns whether name is a valid reference name.
// This ensures that the reference name is both a valid application name
// and a valid charm name.
func isValidReferenceName(name string) bool {
	return isValidApplicationName(name) && isValidCharmName(name)
}

// addDefaultStorageDirectives fills in default values, replacing any empty/missing values
// in the specified directives.
func addDefaultStorageDirectives(
	ctx context.Context,
	state State,
	modelType coremodel.ModelType,
	allDirectives map[string]storage.Directive,
	storage map[string]internalcharm.Storage,
) (map[string]storage.Directive, error) {
	defaults, err := state.StorageDefaults(ctx)
	if err != nil {
		return nil, errors.Errorf("getting storage defaults: %w", err)
	}
	return domainstorage.StorageDirectivesWithDefaults(storage, modelType, defaults, allDirectives)
}

func validateStorageDirectives(
	ctx context.Context,
	state State,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	modelType coremodel.ModelType,
	allDirectives map[string]storage.Directive,
	meta *internalcharm.Meta,
) error {
	registry, err := storageRegistryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	validator, err := domainstorage.NewStorageDirectivesValidator(modelType, registry, state)
	if err != nil {
		return errors.Capture(err)
	}
	err = validator.ValidateStorageDirectivesAgainstCharm(ctx, allDirectives, meta)
	if err != nil {
		return errors.Capture(err)
	}
	// Ensure all stores have directives specified. Defaults should have
	// been set by this point, if the user didn't specify any.
	for name, charmStorage := range meta.Storage {
		if _, ok := allDirectives[name]; !ok && charmStorage.CountMin > 0 {
			return errors.Errorf("%w for store %q", applicationerrors.MissingStorageDirective, name)
		}
	}
	return nil
}

func encodeChannelAndPlatform(origin corecharm.Origin) (*deployment.Channel, deployment.Platform, error) {
	channel, err := encodeChannel(origin.Channel)
	if err != nil {
		return nil, deployment.Platform{}, errors.Capture(err)
	}

	platform, err := encodePlatform(origin.Platform)
	if err != nil {
		return nil, deployment.Platform{}, errors.Capture(err)
	}

	return channel, platform, nil

}

func encodeCharmSource(source corecharm.Source) (charm.CharmSource, error) {
	switch source {
	case corecharm.Local:
		return charm.LocalSource, nil
	case corecharm.CharmHub:
		return charm.CharmHubSource, nil
	default:
		return "", errors.Errorf("unknown source %q, expected local or charmhub: %w", source, applicationerrors.CharmSourceNotValid)
	}
}

func encodeChannel(ch *internalcharm.Channel) (*deployment.Channel, error) {
	// Empty channels (not nil), with empty strings for track, risk and branch,
	// will be normalized to "stable", so aren't officially empty.
	// We need to handle that case correctly.
	if ch == nil {
		return nil, nil
	}

	// Always ensure to normalize the channel before encoding it, so that
	// all channels saved to the database are in a consistent format.
	normalize := ch.Normalize()

	risk, err := encodeChannelRisk(normalize.Risk)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return &deployment.Channel{
		Track:  normalize.Track,
		Risk:   risk,
		Branch: normalize.Branch,
	}, nil
}

func encodeChannelRisk(risk internalcharm.Risk) (deployment.ChannelRisk, error) {
	switch risk {
	case internalcharm.Stable:
		return deployment.RiskStable, nil
	case internalcharm.Candidate:
		return deployment.RiskCandidate, nil
	case internalcharm.Beta:
		return deployment.RiskBeta, nil
	case internalcharm.Edge:
		return deployment.RiskEdge, nil
	default:
		return "", errors.Errorf("unknown risk %q, expected stable, candidate, beta or edge", risk)
	}
}

func encodePlatform(platform corecharm.Platform) (deployment.Platform, error) {
	ostype, err := encodeOSType(platform.OS)
	if err != nil {
		return deployment.Platform{}, errors.Capture(err)
	}

	return deployment.Platform{
		Channel:      platform.Channel,
		OSType:       ostype,
		Architecture: encodeArchitecture(platform.Architecture),
	}, nil
}

func encodeOSType(os string) (deployment.OSType, error) {
	switch ostype.OSTypeForName(os) {
	case ostype.Ubuntu:
		return deployment.Ubuntu, nil
	default:
		return 0, errors.Errorf("unknown os type %q, expected ubuntu", os)
	}
}

func encodeArchitecture(a string) architecture.Architecture {
	switch a {
	case arch.AMD64:
		return architecture.AMD64
	case arch.ARM64:
		return architecture.ARM64
	case arch.PPC64EL:
		return architecture.PPC64EL
	case arch.S390X:
		return architecture.S390X
	case arch.RISCV64:
		return architecture.RISCV64
	default:
		return architecture.Unknown
	}
}

func ptr[T any](v T) *T {
	return &v
}
