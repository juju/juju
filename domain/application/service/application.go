// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	coredatabase "github.com/juju/juju/core/database"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/application"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	uniterrors "github.com/juju/juju/domain/unit/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/storage"
)

// ApplicationState describes retrieval and persistence methods for
// applications.
type ApplicationState interface {
	// ExecuteTxnOperation starts a txn and looks up ID of the specified application.
	// It invokes the supplied callback with the app ID and a set of operations which
	// can be called and will run in the single transaction. It returns an error
	// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist
	// and an error satisfying [applicationerrors.ApplicationIsDead] if the application is dead
	// and deadOk is false.
	ExecuteTxnOperation(ctx context.Context, appName string, deadOk bool, _ application.StateTxOperationFunc) error

	// StorageDefaults returns the default storage sources for a model.
	StorageDefaults(ctx context.Context) (domainstorage.StorageDefaults, error)

	// GetStoragePoolByName returns the storage pool with the specified name, returning an error
	// satisfying [storageerrors.PoolNotFoundError] if it doesn't exist.
	GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePoolDetails, error)

	// CreateApplication creates an application, whilst inserting a charm into the
	// database, returning an error satisfying [applicationerrors.ApplicationAle\readyExists]
	// if the application already exists.
	CreateApplication(context.Context, string, application.AddApplicationArg, ...application.UpsertUnitArg) (coreapplication.ID, error)

	// ApplicationScaleState looks up the scale state of the specified application, returning an error
	// satisfying [applicationerrors.ApplicationNotFound] if the application is not found.
	ApplicationScaleState(context.Context, string) (application.ScaleState, error)

	// UpsertCloudService updates the cloud service for the specified application, returning an error
	// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
	UpsertCloudService(ctx context.Context, appName, providerID string, sAddrs network.SpaceAddresses) error

	// DeleteApplication deletes the specified application, returning an error
	// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
	// If the application still has units, as error satisfying [applicationerrors.ApplicationHasUnits]
	// is returned.
	DeleteApplication(context.Context, string) error

	// GetApplicationID returns the ID for the named application, returning an error
	// satisfying [applicationerrors.ApplicationNotFound] if the application is not found.
	GetApplicationID(context.Context, string) (coreapplication.ID, error)

	// AddUnits adds the specified units to the application, returning an error
	// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
	AddUnits(ctx context.Context, applicationName string, args ...application.UpsertUnitArg) error

	InitialWatchStatementUnitLife(appName string) (string, eventsource.NamespaceQuery)

	GetUnitLife(ctx context.Context, unitIDs []string) (map[string]life.Life, error)
}

// ApplicationService provides the API for working with applications.
type ApplicationService struct {
	st     ApplicationState
	logger logger.Logger

	registry  storage.ProviderRegistry
	modelType coremodel.ModelType
}

// NewApplicationService returns a new service reference wrapping the input state.
func NewApplicationService(st ApplicationState, registry storage.ProviderRegistry, logger logger.Logger) *ApplicationService {
	// Some uses of application service don't need to supply a storage registry, eg cleaner facade.
	// In such cases it'd wasteful to create one as an environ instance would be needed.
	if registry == nil {
		registry = storage.NotImplementedProviderRegistry{}
	}
	return &ApplicationService{
		st:       st,
		logger:   logger,
		registry: registry,
		// TODO(storage) - pass in model info getter
		modelType: coremodel.IAAS,
	}
}

// CreateApplication creates the specified application and units if required,
// returning an error satisfying [applicationerrors.ApplicationAle\readyExists]
// if the application already exists.
func (s *ApplicationService) CreateApplication(
	ctx context.Context,
	name string,
	charm charm.Charm,
	origin corecharm.Origin,
	args AddApplicationArgs,
	units ...AddUnitArg,
) (coreapplication.ID, error) {
	// Validate that we have a valid charm and name.
	meta := charm.Meta()
	if meta == nil {
		return "", applicationerrors.CharmMetadataNotValid
	} else if name == "" && meta.Name == "" {
		return "", applicationerrors.ApplicationNameNotValid
	} else if meta.Name == "" {
		return "", applicationerrors.CharmNameNotValid
	}

	if err := origin.Validate(); err != nil {
		return "", fmt.Errorf("%w: %v", applicationerrors.CharmOriginNotValid, err)
	}

	// We know that the charm name is valid, so we can use it as the application
	// name if that is not provided.
	if name == "" {
		name = meta.Name
	}

	// TODO (stickupkid): These should be done either in the application
	// state in one transaction, or be operating on the domain/charm types.
	//TODO(storage) - insert storage directive for app
	cons := make(map[string]storage.Directive)
	for n, sc := range args.Storage {
		cons[n] = sc
	}
	if err := s.addDefaultStorageDirectives(ctx, s.modelType, cons, meta); err != nil {
		return "", errors.Annotate(err, "adding default storage directives")
	}
	if err := s.validateStorageDirectives(ctx, s.modelType, cons, charm); err != nil {
		return "", errors.Annotate(err, "invalid storage directives")
	}

	// When encoding the charm, this will also validate the charm metadata,
	// when parsing it.
	ch, _, err := encodeCharm(charm)
	if err != nil {
		return "", fmt.Errorf("encode charm: %w", err)
	}

	var channel *domaincharm.Channel
	if origin.Channel != nil {
		normalisedC := origin.Channel.Normalize()
		channel = &domaincharm.Channel{
			Track:  normalisedC.Track,
			Risk:   domaincharm.ChannelRisk(normalisedC.Risk),
			Branch: normalisedC.Branch,
		}
	}
	appArg := application.AddApplicationArg{
		Charm:   ch,
		Channel: channel,
		Platform: application.Platform{
			Channel:        origin.Platform.Channel,
			OSTypeID:       application.MarshallOSType(ostype.OSTypeForName(origin.Platform.OS)),
			ArchitectureID: application.MarshallArchitecture(origin.Platform.Architecture),
		},
	}

	unitArgs := make([]application.UpsertUnitArg, len(units))
	for i, u := range units {
		unitArgs[i] = makeUpsertUnitArgs(u)
	}

	id, err := s.st.CreateApplication(ctx, name, appArg, unitArgs...)
	if err != nil {
		return "", errors.Annotatef(err, "creating application %q", name)
	}
	return id, nil
}

func makeUpsertUnitArgs(in AddUnitArg) application.UpsertUnitArg {
	result := application.UpsertUnitArg{
		UnitName:     in.UnitName,
		PasswordHash: in.PasswordHash,
	}
	if in.CloudContainer != nil {
		result.CloudContainer = &application.CloudContainer{
			ProviderId: in.CloudContainer.ProviderId,
			Ports:      in.CloudContainer.Ports,
		}
		if in.CloudContainer.Address != nil {
			result.CloudContainer.Address = &application.Address{
				Value:       in.CloudContainer.Address.Value,
				AddressType: string(in.CloudContainer.Address.AddressType()),
				Scope:       string(in.CloudContainer.Address.Scope),
				SpaceID:     in.CloudContainer.Address.SpaceID,
				Origin:      string(network.OriginProvider),
			}
			if in.CloudContainer.AddressOrigin != nil {
				result.CloudContainer.Address.Origin = string(*in.CloudContainer.AddressOrigin)
			}
		}
	}
	return result
}

// AddUnits adds the specified units to the application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
func (s *ApplicationService) AddUnits(ctx context.Context, name string, units ...AddUnitArg) error {
	args := make([]application.UpsertUnitArg, len(units))
	for i, u := range units {
		args[i] = application.UpsertUnitArg{
			UnitName: u.UnitName,
		}
	}
	err := s.st.AddUnits(ctx, name, args...)
	return errors.Annotatef(err, "adding units to application %q", name)
}

// RegisterCAASUnit creates or updates the specified application unit in a caas model,
// returning an error satisfying [applicationerrors.ApplicationNotFoundError]
// if the application doesn't exist. If the application life is Dead, an error
// satisfying [applicationerrors.ApplicationIsDead] is returned.
func (s *ApplicationService) RegisterCAASUnit(ctx context.Context, appName string, args RegisterCAASUnitParams) error {
	if args.PasswordHash == nil {
		return errors.NotValidf("password hash")
	}
	if args.ProviderId == nil {
		return errors.NotValidf("provider id")
	}
	if !args.OrderedScale {
		return errors.NewNotImplemented(nil, "registering CAAS units not supported without ordered unit IDs")
	}
	if args.UnitName == "" {
		return errors.NotValidf("missing unit name")
	}

	p := AddUnitArg{
		UnitName:     &args.UnitName,
		PasswordHash: args.PasswordHash,
		CloudContainer: &CloudContainerParams{
			ProviderId: args.ProviderId,
			Ports:      args.Ports,
		},
	}
	if args.Address != nil {
		addr := network.NewSpaceAddress(*args.Address, network.WithScope(network.ScopeMachineLocal))
		// k8s doesn't support spaces yet.
		addr.SpaceID = network.AlphaSpaceId
		p.CloudContainer.Address = &addr
		origin := network.OriginProvider
		p.CloudContainer.AddressOrigin = &origin
	}
	// We need to do a bunch of business logic in the one transaction so pass in a closure that is
	// given the transaction to use.
	unitArg := makeUpsertUnitArgs(p)
	err := s.st.ExecuteTxnOperation(ctx, appName, false, func(ctx context.Context, stateOps application.StateTxOperations, appID coreapplication.ID) error {
		unitLife, err := stateOps.UnitLife(ctx, args.UnitName)
		if errors.Is(err, uniterrors.NotFound) {
			return s.insertCAASUnit(ctx, stateOps, appID, args.OrderedId, unitArg)
		}
		if unitLife == life.Dead {
			return fmt.Errorf("dead unit %q already exists%w", args.UnitName, errors.Hide(applicationerrors.ApplicationIsDead))
		}
		return stateOps.UpsertUnit(ctx, appID, unitArg)
	})
	return errors.Annotatef(err, "saving caas unit %q", args.UnitName)
}

func (s *ApplicationService) insertCAASUnit(
	ctx context.Context, stateOps application.StateTxOperations, appID coreapplication.ID, orderedID int, args application.UpsertUnitArg,
) error {
	appScale, err := stateOps.ApplicationScaleState(ctx, appID)
	if err != nil {
		return errors.Annotatef(err, "getting application scale state for app %q", appID)
	}
	if orderedID >= appScale.Scale ||
		(appScale.Scaling && orderedID >= appScale.ScaleTarget) {
		return fmt.Errorf("unrequired unit %s is not assigned%w", *args.UnitName, errors.Hide(uniterrors.NotAssigned))
	}
	return stateOps.UpsertUnit(ctx, appID, args)
}

// DeleteApplication deletes the specified application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
// If the application still has units, as error satisfying [applicationerrors.ApplicationHasUnits]
// is returned.
func (s *ApplicationService) DeleteApplication(ctx context.Context, name string) error {
	err := s.st.DeleteApplication(ctx, name)
	return errors.Annotatef(err, "deleting application %q", name)
}

// UpdateApplicationCharm sets a new charm for the application, validating that aspects such
// as storage are still viable with the new charm.
func (s *ApplicationService) UpdateApplicationCharm(ctx context.Context, name string, params UpdateCharmParams) error {
	//TODO(storage) - update charm and storage directive for app
	return nil
}

// addDefaultStorageDirectives fills in default values, replacing any empty/missing values
// in the specified directives.
func (s *ApplicationService) addDefaultStorageDirectives(ctx context.Context, modelType coremodel.ModelType, allDirectives map[string]storage.Directive, charmMeta *charm.Meta) error {
	defaults, err := s.st.StorageDefaults(ctx)
	if err != nil {
		return errors.Annotate(err, "getting storage defaults")
	}
	return domainstorage.StorageDirectivesWithDefaults(charmMeta.Storage, modelType, defaults, allDirectives)
}

// WatchableApplicationService provides the API for working with applications and the
// ability to create watchers.
type WatchableApplicationService struct {
	ApplicationService
	watcherFactory WatcherFactory
}

// NewWatchableApplicationService returns a new service reference wrapping the input state.
func NewWatchableApplicationService(st ApplicationState, watcherFactory WatcherFactory, registry storage.ProviderRegistry, logger logger.Logger) *WatchableApplicationService {
	return &WatchableApplicationService{
		ApplicationService: ApplicationService{
			st:       st,
			registry: registry,
			logger:   logger,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchApplicationScale returns a watcher that observes changes to an application's scale.
func (s *WatchableApplicationService) WatchApplicationScale(ctx context.Context, appName string) (watcher.NotifyWatcher, error) {
	appID, err := s.st.GetApplicationID(ctx, appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	currentScale, err := s.GetScale(ctx, appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	mask := changestream.Create | changestream.Update
	mapper := func(ctx context.Context, db coredatabase.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
		newScale, err := s.GetScale(ctx, appName)
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

type LifeGetter func(ctx context.Context, db coredatabase.TxnRunner, ids []string) (map[string]corelife.Value, error)

// lifeStringsWatcher is a watcher that watches for changes
// to the life of a particular set of entities.
type lifeStringsWatcher struct {
	logger logger.Logger

	// life holds the most recent known life states of interesting entities.
	life map[string]corelife.Value

	lifeGetter LifeGetter
}

func NewLifeStringsWatcher(
	logger logger.Logger,
	lifeGetter LifeGetter,
) eventsource.Mapper {
	w := &lifeStringsWatcher{
		logger:     logger,
		lifeGetter: lifeGetter,
		life:       map[string]corelife.Value{},
	}
	return w.mapper
}

func (w *lifeStringsWatcher) mapper(ctx context.Context, db coredatabase.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
	w.logger.Criticalf("WWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWW")
	events := make(map[string]changestream.ChangeEvent, len(changes))
	for _, change := range changes {
		events[change.Changed()] = change
	}

	w.logger.Criticalf("GOT MSGS: %+v", events)

	ids := set.NewStrings()
	// Separate ids into those thought to exist and those known to be removed.
	latest := make(map[string]corelife.Value)
	for _, change := range changes {
		if change.Type() == changestream.Delete {
			latest[change.Changed()] = corelife.Dead
			continue
		}
		ids.Add(change.Changed())
	}

	// Collect life states from ids thought to exist. Any that don't actually
	// exist are ignored (we'll hear about them in the next set of updates --
	// all that's actually happened in that situation is that the watcher
	// events have lagged a little behind reality).
	newValues, err := w.lifeGetter(ctx, db, ids.Values())
	if err != nil {
		return nil, errors.Trace(err)
	}

	for id, l := range newValues {
		latest[id] = l
	}
	// Add to ids any whose life state is known to have changed.
	for id, newLife := range latest {
		gone := newLife == corelife.Dead
		oldLife, known := w.life[id]
		switch {
		case known && gone:
			delete(w.life, id)
		case !known && !gone:
			w.life[id] = newLife
		case known && newLife != oldLife:
			w.life[id] = newLife
		default:
			delete(events, id)
		}
	}
	var result []changestream.ChangeEvent
	for _, e := range events {
		result = append(result, e)
	}
	return result, nil
}

// WatchApplicationUnitLife returns a watcher that observes changes to the life of any units if an application.
func (s *WatchableApplicationService) WatchApplicationUnitLife(ctx context.Context, appName string) (watcher.StringsWatcher, error) {
	lifeGetter := func(ctx context.Context, db coredatabase.TxnRunner, ids []string) (map[string]corelife.Value, error) {
		unitLife, err := s.st.GetUnitLife(ctx, ids)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result := make(map[string]corelife.Value)
		for id, l := range unitLife {
			switch l {
			case life.Alive:
				result[id] = corelife.Alive
			case life.Dying:
				result[id] = corelife.Dying
			case life.Dead:
				result[id] = corelife.Dead
			}

		}
		return result, err
	}
	lifeW := NewLifeStringsWatcher(s.logger, lifeGetter)
	table, query := s.st.InitialWatchStatementUnitLife(appName)

	return s.watcherFactory.NewNamespaceMapperWatcher(table, changestream.All, query, lifeW)
}

func (s *ApplicationService) validateStorageDirectives(ctx context.Context, modelType coremodel.ModelType, allDirectives map[string]storage.Directive, charm charm.Charm) error {
	validator, err := domainstorage.NewStorageDirectivesValidator(modelType, s.registry, s.st)
	if err != nil {
		return errors.Trace(err)
	}
	err = validator.ValidateStorageDirectivesAgainstCharm(ctx, allDirectives, charm)
	if err != nil {
		return errors.Trace(err)
	}
	// Ensure all stores have directives specified. Defaults should have
	// been set by this point, if the user didn't specify any.
	for name, charmStorage := range charm.Meta().Storage {
		if _, ok := allDirectives[name]; !ok && charmStorage.CountMin > 0 {
			return fmt.Errorf("%w for store %q", applicationerrors.MissingStorageDirective, name)
		}
	}
	return nil
}

// UpdateCloudService updates the cloud service for the specified application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
func (s *ApplicationService) UpdateCloudService(ctx context.Context, appName, providerID string, sAddrs network.SpaceAddresses) error {
	return s.st.UpsertCloudService(ctx, appName, providerID, sAddrs)
}

// Broker provides access to the k8s cluster to guery the scale
// of a specified application.
type Broker interface {
	Application(string, caas.DeploymentType) caas.Application
}

// CAASUnitTerminating should be called by the CAASUnitTerminationWorker when
// the agent receives a signal to exit. UnitTerminating will return how
// the agent should shutdown.
// We pass in a CAAS broker to get app details from the k8s cluster - we will probably
// make it a service attribute once more use cases emerge.
func (s *ApplicationService) CAASUnitTerminating(ctx context.Context, appName string, unitNum int, broker Broker) (bool, error) {
	// TODO(sidecar): handle deployment other than statefulset
	deploymentType := caas.DeploymentStateful
	restart := true

	switch deploymentType {
	case caas.DeploymentStateful:
		caasApp := broker.Application(appName, caas.DeploymentStateful)
		appState, err := caasApp.State()
		if err != nil {
			return false, errors.Trace(err)
		}
		scaleInfo, err := s.st.ApplicationScaleState(ctx, appName)
		if err != nil {
			return false, errors.Trace(err)
		}
		if unitNum >= scaleInfo.Scale || unitNum >= appState.DesiredReplicas {
			restart = false
		}
	case caas.DeploymentStateless, caas.DeploymentDaemon:
		// Both handled the same way.
		restart = true
	default:
		return false, errors.NotSupportedf("unknown deployment type")
	}
	return restart, nil
}

// SetScale sets the application's desired scale value, returning an error
// satisfying [applicationerrors.ApplicationNotFound] if the application is not found.
// This is used on CAAS models.
func (s *ApplicationService) SetScale(ctx context.Context, appName string, scale int, force bool) error {
	if scale < 0 {
		return fmt.Errorf("application scale %d not valid%w", scale, errors.Hide(applicationerrors.ScaleChangeInvalid))
	}
	err := s.st.ExecuteTxnOperation(ctx, appName, false, func(ctx context.Context, stateOps application.StateTxOperations, appID coreapplication.ID) error {
		appScale, err := stateOps.ApplicationScaleState(ctx, appID)
		if err != nil {
			return errors.Annotatef(err, "getting application scale state for app %q", appID)
		}
		s.logger.Tracef(
			"SetScale DesiredScaleProtected %v, DesiredScale %v -> %v",
			appScale.DesiredScaleProtected, appScale.Scale, scale,
		)
		if appScale.DesiredScaleProtected && !force && scale != appScale.Scale {
			return fmt.Errorf(
				"%w: SetScale(%d) without force while desired scale %d is not applied yet",
				applicationerrors.ScaleChangeInvalid, scale, appScale.Scale)
		}
		return stateOps.SetDesiredApplicationScale(ctx, appID, scale, force)
	})
	return errors.Annotatef(err, "setting scale for application %q", appName)
}

// GetScale returns the desired scale of an application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
// This is used on CAAS models.
func (s *ApplicationService) GetScale(ctx context.Context, appName string) (int, error) {
	result, err := s.st.ApplicationScaleState(ctx, appName)
	if err != nil {
		return -1, errors.Annotatef(err, "getting scaling state for %q", appName)
	}
	return result.Scale, nil
}

// ChangeScale alters the existing scale by the provided change amount, returning the new amount.
// It returns an error satisfying [applicationerrors.ApplicationNotFoundError] if the application
// doesn't exist.
// This is used on CAAS models.
func (s *ApplicationService) ChangeScale(ctx context.Context, appName string, scaleChange int) (int, error) {
	var newScale int
	err := s.st.ExecuteTxnOperation(ctx, appName, false, func(ctx context.Context, stateOps application.StateTxOperations, appID coreapplication.ID) error {
		currentScaleState, err := stateOps.ApplicationScaleState(ctx, appID)
		if err != nil {
			return errors.Annotatef(err, "getting current scale state for %q", appName)
		}

		newScale = currentScaleState.Scale + scaleChange
		s.logger.Tracef("ChangeScale DesiredScale %v, scaleChange %v, newScale %v", currentScaleState.Scale, scaleChange, newScale)
		if newScale < 0 {
			newScale = currentScaleState.Scale
			return fmt.Errorf(
				"%w: cannot remove more units than currently exist", applicationerrors.ScaleChangeInvalid)
		}
		err = stateOps.SetDesiredApplicationScale(ctx, appID, newScale, true)
		return errors.Annotatef(err, "changing scaling state for %q", appName)
	})
	return newScale, errors.Annotatef(err, "changing scale for %q", appName)
}

// SetScalingState updates the scale state of an application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
// This is used on CAAS models.
func (s *ApplicationService) SetScalingState(ctx context.Context, appName string, scaleTarget int, scaling bool) error {
	err := s.st.ExecuteTxnOperation(ctx, appName, true, func(ctx context.Context, stateOps application.StateTxOperations, appID coreapplication.ID) error {
		appLife, err := stateOps.ApplicationLife(ctx, appID)
		if err != nil {
			return errors.Annotatef(err, "getting life for %q", appName)
		}
		currentScaleState, err := stateOps.ApplicationScaleState(ctx, appID)
		if err != nil {
			return errors.Annotatef(err, "getting current scale state for %q", appName)
		}

		var scale *int
		if scaling {
			switch appLife {
			case life.Alive:
				// if starting a scale, ensure we are scaling to the same target.
				if !currentScaleState.Scaling && currentScaleState.Scale != scaleTarget {
					return applicationerrors.ScalingStateInconsistent
				}
			case life.Dying, life.Dead:
				// force scale to the scale target when dying/dead.
				scale = &scaleTarget
			}
		}
		err = stateOps.SetApplicationScalingState(ctx, appID, scale, scaleTarget, scaling)
		return errors.Annotatef(err, "updating scaling state for %q", appName)
	})
	return errors.Annotatef(err, "setting scale for %q", appName)

}

// GetScalingState returns the scale state of an application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
// This is used on CAAS models.
func (s *ApplicationService) GetScalingState(ctx context.Context, name string) (ScalingState, error) {
	result, err := s.st.ApplicationScaleState(ctx, name)
	if err != nil {
		return ScalingState{}, errors.Annotatef(err, "getting scaling state for %q", name)
	}
	return ScalingState{
		ScaleTarget: result.ScaleTarget,
		Scaling:     result.Scaling,
	}, nil
}
