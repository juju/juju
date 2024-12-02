// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/providertracker"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/environs"
	internalcharm "github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// State represents a type for interacting with the underlying state.
type State interface {
	ApplicationState
	CharmState
	ResourceState
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
	st     State
	logger logger.Logger
	clock  clock.Clock

	storageRegistryGetter corestorage.ModelStorageRegistryGetter
	resourceStoreGetter   ResourceStoreGetter
	secretDeleter         DeleteSecretState
	charmStore            CharmStore
}

// NewService returns a new service reference wrapping the input state.
func NewService(
	st State,
	deleteSecretState DeleteSecretState,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	charmStore CharmStore,
	resourceStoreGetter ResourceStoreGetter,
	clock clock.Clock,
	logger logger.Logger,
) *Service {
	// Note:
	// The store for container image resources is really a DQLite table today.
	// Using AddStore is a compromise to avoid injecting one service into
	// another, as would happen if NewResourceStoreFactory had a second
	// argument to provide a containerImageResourceStore.
	resourceStoreGetter.AddStore(
		charmresource.TypeContainerImage,
		nil,
	)
	return &Service{
		st:                    st,
		logger:                logger,
		clock:                 clock,
		storageRegistryGetter: storageRegistryGetter,
		resourceStoreGetter:   resourceStoreGetter,
		secretDeleter:         deleteSecretState,
		charmStore:            charmStore,
	}
}

// AgentVersionGetter is responsible for retrieving the agent version for a
// given model.
type AgentVersionGetter interface {
	// GetModelTargetAgentVersion returns the agent version for the specified
	// model.
	GetModelTargetAgentVersion(context.Context, coremodel.UUID) (version.Number, error)
}

// Provider defines the interface for interacting with the underlying model
// provider.
type Provider interface {
	environs.SupportedFeatureEnumerator
}

// ProviderService defines a service for interacting with the underlying
// model state.
type ProviderService struct {
	*Service

	modelID            coremodel.UUID
	agentVersionGetter AgentVersionGetter
	provider           providertracker.ProviderGetter[Provider]
}

// NewProviderService returns a new Service for interacting with a models state.
func NewProviderService(
	st State,
	deleteSecretState DeleteSecretState,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	resourceStoreGetter ResourceStoreGetter,
	modelID coremodel.UUID,
	agentVersionGetter AgentVersionGetter,
	provider providertracker.ProviderGetter[Provider],
	charmStore CharmStore,
	clock clock.Clock,
	logger logger.Logger,
) *ProviderService {
	return &ProviderService{
		Service: NewService(
			st,
			deleteSecretState,
			storageRegistryGetter,
			charmStore,
			resourceStoreGetter,
			clock,
			logger,
		),
		modelID:            modelID,
		agentVersionGetter: agentVersionGetter,
		provider:           provider,
	}
}

// GetSupportedFeatures returns the set of features that the model makes
// available for charms to use.
// If the agent version cannot be found, an error satisfying
// [modelerrors.NotFound] will be returned.
func (s *ProviderService) GetSupportedFeatures(ctx context.Context) (assumes.FeatureSet, error) {
	agentVersion, err := s.agentVersionGetter.GetModelTargetAgentVersion(ctx, s.modelID)
	if err != nil {
		return assumes.FeatureSet{}, err
	}

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{
		Name:        "juju",
		Description: assumes.UserFriendlyFeatureDescriptions["juju"],
		Version:     &agentVersion,
	})

	provider, err := s.provider(ctx)
	if errors.Is(err, errors.NotSupported) {
		return fs, nil
	} else if err != nil {
		return fs, err
	}

	envFs, err := provider.SupportedFeatures()
	if err != nil {
		return fs, fmt.Errorf("enumerating features supported by environment: %w", err)
	}

	fs.Merge(envFs)

	return fs, nil
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
	deleteSecretState DeleteSecretState,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	resourceStoreGetter ResourceStoreGetter,
	modelID coremodel.UUID,
	watcherFactory WatcherFactory,
	agentVersionGetter AgentVersionGetter,
	provider providertracker.ProviderGetter[Provider],
	charmStore CharmStore,
	clock clock.Clock,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		ProviderService: NewProviderService(
			st,
			deleteSecretState,
			storageRegistryGetter,
			resourceStoreGetter,
			modelID,
			agentVersionGetter,
			provider,
			charmStore,
			clock,
			logger,
		),
		watcherFactory: watcherFactory,
	}
}

// WatchApplicationUnitLife returns a watcher that observes changes to the life of any units if an application.
func (s *WatchableService) WatchApplicationUnitLife(appName string) (watcher.StringsWatcher, error) {
	lifeGetter := func(ctx context.Context, db database.TxnRunner, ids []string) (map[string]life.Life, error) {
		unitUUIDs, err := transform.SliceOrErr(ids, coreunit.ParseID)
		if err != nil {
			return nil, err
		}
		unitLifes, err := s.st.GetApplicationUnitLife(ctx, appName, unitUUIDs...)
		if err != nil {
			return nil, err
		}
		result := make(map[string]life.Life, len(unitLifes))
		for unitUUID, life := range unitLifes {
			result[unitUUID.String()] = life
		}
		return result, nil
	}
	lifeMapper := domain.LifeStringsWatcherMapperFunc(s.logger, lifeGetter)

	table, query := s.st.InitialWatchStatementUnitLife(appName)
	return s.watcherFactory.NewNamespaceMapperWatcher(table, changestream.All, query, lifeMapper)
}

// WatchApplicationScale returns a watcher that observes changes to an application's scale.
func (s *WatchableService) WatchApplicationScale(ctx context.Context, appName string) (watcher.NotifyWatcher, error) {
	appID, currentScale, err := s.getApplicationScaleAndID(ctx, appName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	mask := changestream.Create | changestream.Update
	mapper := func(ctx context.Context, db database.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
		newScale, err := s.GetApplicationScale(ctx, appName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// Only dispatch if the scale has changed.
		if newScale != currentScale {
			currentScale = newScale
			return changes, nil
		}
		return nil, nil
	}
	return s.watcherFactory.NewValueMapperWatcher("application_scale", appID.String(), mask, mapper)
}

// WatchApplicationsWithPendingCharms returns a watcher that observes changes to
// applications that have pending charms.
func (s *WatchableService) WatchApplicationsWithPendingCharms(ctx context.Context) (watcher.StringsWatcher, error) {

	table, query := s.st.InitialWatchStatementApplicationsWithPendingCharms()
	return s.watcherFactory.NewNamespaceMapperWatcher(
		table,
		changestream.Create,
		query,
		func(ctx context.Context, _ database.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
			return s.watchApplicationsWithPendingCharmsMapper(ctx, changes)
		},
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
			s.logger.Errorf("application %q has pending charms but no change events", appID)
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

// MigrationService provides the API for migrating applications.
type MigrationService struct {
	st                    State
	storageRegistryGetter corestorage.ModelStorageRegistryGetter
	clock                 clock.Clock
	logger                logger.Logger
}

// NewMigrationService returns a new service reference wrapping the input state.
func NewMigrationService(
	st State,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	clock clock.Clock,
	logger logger.Logger,
) *MigrationService {
	return &MigrationService{
		st:                    st,
		storageRegistryGetter: storageRegistryGetter,
		clock:                 clock,
		logger:                logger,
	}
}

// GetCharmID returns a charm ID by name. It returns an error if the charm
// can not be found by the name.
// This can also be used as a cheap way to see if a charm exists without
// needing to load the charm metadata.
// Returns [applicationerrors.CharmNameNotValid] if the name is not valid, and
// [applicationerrors.CharmNotFound] if the charm is not found.
func (s *MigrationService) GetCharmID(ctx context.Context, args charm.GetCharmArgs) (corecharm.ID, error) {
	if !isValidCharmName(args.Name) {
		return "", applicationerrors.CharmNameNotValid
	}

	// Validate the source, it can only be charmhub or local.
	if args.Source != charm.CharmHubSource && args.Source != charm.LocalSource {
		return "", applicationerrors.CharmSourceNotValid
	}

	if rev := args.Revision; rev != nil && *rev >= 0 {
		return s.st.GetCharmID(ctx, args.Name, *rev, args.Source)
	}

	return "", applicationerrors.CharmNotFound
}

// GetCharm returns the charm using the charm ID.
// Calling this method will return all the data associated with the charm.
// It is not expected to call this method for all calls, instead use the move
// focused and specific methods. That's because this method is very expensive
// to call. This is implemented for the cases where all the charm data is
// needed; model migration, charm export, etc.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *MigrationService) GetCharm(ctx context.Context, id corecharm.ID) (internalcharm.Charm, charm.CharmLocator, error) {
	if err := id.Validate(); err != nil {
		return nil, charm.CharmLocator{}, fmt.Errorf("charm id: %w", err)
	}

	ch, _, err := s.st.GetCharm(ctx, id)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Trace(err)
	}

	// The charm needs to be decoded into the internalcharm.Charm type.

	metadata, err := decodeMetadata(ch.Metadata)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Trace(err)
	}

	manifest, err := decodeManifest(ch.Manifest)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Trace(err)
	}

	actions, err := decodeActions(ch.Actions)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Trace(err)
	}

	config, err := decodeConfig(ch.Config)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Trace(err)
	}

	lxdProfile, err := decodeLXDProfile(ch.LXDProfile)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Trace(err)
	}

	locator := charm.CharmLocator{
		Name:         ch.ReferenceName,
		Revision:     ch.Revision,
		Source:       ch.Source,
		Architecture: ch.Architecture,
	}

	return internalcharm.NewCharmBase(
		&metadata,
		&manifest,
		&config,
		&actions,
		&lxdProfile,
	), locator, nil
}

// ImportApplication imports the specified application and units if required,
// returning an error satisfying [applicationerrors.ApplicationAlreadyExists]
// if the application already exists.
func (s *MigrationService) ImportApplication(
	ctx context.Context, appName string,
	charm internalcharm.Charm, origin corecharm.Origin, args AddApplicationArgs,
	units ...ImportUnitArg,
) error {
	if err := validateCreateApplicationParams(appName, args.ReferenceName, charm, origin, args.DownloadInfo); err != nil {
		return errors.Annotatef(err, "invalid application args")
	}

	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return errors.Annotatef(err, "getting model type")
	}
	appArg, err := makeCreateApplicationArgs(ctx, s.st, s.storageRegistryGetter, modelType, charm, origin, args)
	if err != nil {
		return errors.Annotatef(err, "creating application args")
	}
	appArg.Scale = len(units)

	unitArgs := make([]application.InsertUnitArg, len(units))
	for i, u := range units {
		arg := application.InsertUnitArg{
			UnitName: u.UnitName,
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: application.UnitAgentStatusInfo{
					StatusID:   application.MarshallUnitAgentStatus(u.AgentStatus.Status),
					StatusInfo: s.makeUnitStatus(u.AgentStatus),
				},
				WorkloadStatus: application.UnitWorkloadStatusInfo{
					StatusID:   application.MarshallUnitWorkloadStatus(u.WorkloadStatus.Status),
					StatusInfo: s.makeUnitStatus(u.WorkloadStatus),
				},
			},
		}
		if u.CloudContainer != nil {
			arg.CloudContainer = makeCloudContainerArg(u.UnitName, *u.CloudContainer)
		}
		if u.PasswordHash != nil {
			arg.Password = &application.PasswordInfo{
				PasswordHash:  *u.PasswordHash,
				HashAlgorithm: application.HashAlgorithmSHA256,
			}
		}
		unitArgs[i] = arg
	}

	err = s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.CreateApplication(ctx, appName, appArg)
		if err != nil {
			return errors.Annotatef(err, "creating application %q", appName)
		}
		for _, arg := range unitArgs {
			if err := s.st.InsertUnit(ctx, appID, arg); err != nil {
				return errors.Annotatef(err, "inserting unit %q", arg.UnitName)
			}
		}
		return nil
	})
	return err
}

func (s *MigrationService) makeUnitStatus(in StatusParams) application.StatusInfo {
	si := application.StatusInfo{
		Message: in.Message,
		Since:   s.clock.Now(),
	}
	if in.Since != nil {
		si.Since = *in.Since
	}
	if len(in.Data) > 0 {
		si.Data = make(map[string]string)
		for k, v := range in.Data {
			if v == nil {
				continue
			}
			si.Data[k] = fmt.Sprintf("%v", v)
		}
	}
	return si
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
		return nil, errors.Annotate(err, "getting storage defaults")
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
		return errors.Trace(err)
	}

	validator, err := domainstorage.NewStorageDirectivesValidator(modelType, registry, state)
	if err != nil {
		return errors.Trace(err)
	}
	err = validator.ValidateStorageDirectivesAgainstCharm(ctx, allDirectives, meta)
	if err != nil {
		return errors.Trace(err)
	}
	// Ensure all stores have directives specified. Defaults should have
	// been set by this point, if the user didn't specify any.
	for name, charmStorage := range meta.Storage {
		if _, ok := allDirectives[name]; !ok && charmStorage.CountMin > 0 {
			return fmt.Errorf("%w for store %q", applicationerrors.MissingStorageDirective, name)
		}
	}
	return nil
}

func encodeChannelAndPlatform(origin corecharm.Origin) (*application.Channel, application.Platform, error) {
	channel, err := encodeChannel(origin.Channel)
	if err != nil {
		return nil, application.Platform{}, errors.Trace(err)
	}

	platform, err := encodePlatform(origin.Platform)
	if err != nil {
		return nil, application.Platform{}, errors.Trace(err)
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
		return "", internalerrors.Errorf("unknown source %q, expected local or charmhub: %w", source, applicationerrors.CharmSourceNotValid)
	}
}

func encodeChannel(ch *internalcharm.Channel) (*application.Channel, error) {
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
		return nil, errors.Trace(err)
	}

	return &application.Channel{
		Track:  normalize.Track,
		Risk:   risk,
		Branch: normalize.Branch,
	}, nil
}

func encodeChannelRisk(risk internalcharm.Risk) (application.ChannelRisk, error) {
	switch risk {
	case internalcharm.Stable:
		return application.RiskStable, nil
	case internalcharm.Candidate:
		return application.RiskCandidate, nil
	case internalcharm.Beta:
		return application.RiskBeta, nil
	case internalcharm.Edge:
		return application.RiskEdge, nil
	default:
		return "", errors.Errorf("unknown risk %q, expected stable, candidate, beta or edge", risk)
	}
}

func encodePlatform(platform corecharm.Platform) (application.Platform, error) {
	ostype, err := encodeOSType(platform.OS)
	if err != nil {
		return application.Platform{}, errors.Trace(err)
	}

	arch := encodeArchitecture(platform.Architecture)
	if err != nil {
		return application.Platform{}, errors.Trace(err)
	}

	return application.Platform{
		Channel:      platform.Channel,
		OSType:       ostype,
		Architecture: arch,
	}, nil
}

func encodeOSType(os string) (application.OSType, error) {
	switch ostype.OSTypeForName(os) {
	case ostype.Ubuntu:
		return application.Ubuntu, nil
	default:
		return 0, errors.Errorf("unknown os type %q, expected ubuntu", os)
	}
}

func encodeArchitecture(a string) application.Architecture {
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
		return architecture.RISV64
	default:
		return architecture.Unknown
	}
}
