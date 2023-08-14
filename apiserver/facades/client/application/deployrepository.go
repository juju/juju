// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"sync"

	"github.com/juju/charm/v10"
	jujuclock "github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/kr/pretty"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/client/charms/services"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/bootstrap"
	environsconfig "github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	jujuversion "github.com/juju/juju/version"
)

var deployRepoLogger = logger.Child("deployfromrepository")

// DeployFromRepositoryValidator defines an deploy config validator.
type DeployFromRepositoryValidator interface {
	ValidateArg(params.DeployFromRepositoryArg) (deployTemplate, []error)
}

// DeployFromRepository defines an interface for deploying a charm
// from a repository.
type DeployFromRepository interface {
	DeployFromRepository(arg params.DeployFromRepositoryArg) (params.DeployFromRepositoryInfo, []*params.PendingResourceUpload, []error)
}

// DeployFromRepositoryState defines a common set of functions for retrieving state
// objects.
type DeployFromRepositoryState interface {
	AddApplication(state.AddApplicationArgs) (Application, error)
	AddCharmMetadata(info state.CharmInfo) (Charm, error)
	ControllerConfig() (controller.Config, error)
	Machine(string) (Machine, error)
	ModelConstraints() (constraints.Value, error)

	services.StateBackend

	network.SpaceLookup
	DefaultEndpointBindingSpace() (string, error)
	Space(id string) (*state.Space, error)
}

// DeployFromRepositoryAPI provides the deploy from repository
// API facade for any given version. It is expected that any API
// parameter changes should be performed before entering the API.
type DeployFromRepositoryAPI struct {
	state     DeployFromRepositoryState
	validator DeployFromRepositoryValidator

	stateCharm func(Charm) *state.Charm
}

// NewDeployFromRepositoryAPI creates a new DeployFromRepositoryAPI.
func NewDeployFromRepositoryAPI(state DeployFromRepositoryState, validator DeployFromRepositoryValidator) DeployFromRepository {
	api := &DeployFromRepositoryAPI{
		state:      state,
		validator:  validator,
		stateCharm: CharmToStateCharm,
	}
	return api
}

func (api *DeployFromRepositoryAPI) DeployFromRepository(arg params.DeployFromRepositoryArg) (params.DeployFromRepositoryInfo, []*params.PendingResourceUpload, []error) {
	deployRepoLogger.Tracef("deployOneFromRepository(%s)", pretty.Sprint(arg))
	// Validate the args.
	dt, errs := api.validator.ValidateArg(arg)

	if len(errs) > 0 {
		return params.DeployFromRepositoryInfo{}, nil, errs
	}

	info := params.DeployFromRepositoryInfo{
		Architecture: dt.origin.Platform.Architecture,
		Base: params.Base{
			Name:    dt.origin.Platform.OS,
			Channel: dt.origin.Platform.Channel,
		},
		Channel:          dt.origin.Channel.String(),
		EffectiveChannel: nil,
		Name:             dt.applicationName,
		Revision:         dt.charmURL.Revision,
	}
	if dt.dryRun {
		return info, nil, nil
	}
	// Queue async charm download.
	// AddCharmMetadata returns no error if the charm
	// has already been queue'd or downloaded.
	ch, err := api.state.AddCharmMetadata(state.CharmInfo{
		Charm: dt.charm,
		ID:    dt.charmURL,
	})
	if err != nil {
		return params.DeployFromRepositoryInfo{}, nil, []error{errors.Trace(err)}
	}

	stOrigin, err := StateCharmOrigin(dt.origin)
	if err != nil {
		return params.DeployFromRepositoryInfo{}, nil, []error{errors.Trace(err)}
	}
	_, err = api.state.AddApplication(state.AddApplicationArgs{
		Name:              dt.applicationName,
		Charm:             api.stateCharm(ch),
		CharmOrigin:       stOrigin,
		Storage:           nil,
		Devices:           nil,
		AttachStorage:     nil,
		EndpointBindings:  dt.endpoints,
		ApplicationConfig: dt.applicationConfig,
		CharmConfig:       dt.charmSettings,
		NumUnits:          dt.numUnits,
		Placement:         dt.placement,
		Constraints:       dt.constraints,
		Resources:         dt.resources,
	})
	if err != nil {
		return params.DeployFromRepositoryInfo{}, nil, []error{errors.Trace(err)}
	}

	// Last step, add pending resources.
	pendingResourceUploads, errs := addPendingResources()

	return info, pendingResourceUploads, errs
}

// addPendingResource adds a pending resource doc for all resources to be
// added when deploying the charm. PendingResourceUpload is only returned
// for local resources which will require the client to upload the
// resource once DeployFromRepository returns. All resources will be
// processed. Errors are not terminal.
// TODO: determine necessary args.
func addPendingResources() ([]*params.PendingResourceUpload, []error) {
	return nil, nil
}

type deployTemplate struct {
	applicationConfig *config.Config
	applicationName   string
	charm             charm.Charm
	charmSettings     charm.Settings
	charmURL          *charm.URL
	constraints       constraints.Value
	endpoints         map[string]string
	dryRun            bool
	force             bool
	numUnits          int
	origin            corecharm.Origin
	placement         []*instance.Placement
	resources         map[string]string
	storage           map[string]state.StorageConstraints
}

func makeDeployFromRepositoryValidator(st DeployFromRepositoryState, m Model, charmhubHTTPClient facade.HTTPClient) DeployFromRepositoryValidator {
	v := &deployFromRepositoryValidator{
		charmhubHTTPClient: charmhubHTTPClient,
		model:              m,
		state:              st,
		newRepoFactory: func(cfg services.CharmRepoFactoryConfig) corecharm.RepositoryFactory {
			return services.NewCharmRepoFactory(cfg)
		},
		newStateBindings: func(st state.EndpointBinding, givenMap map[string]string) (Bindings, error) {
			return state.NewBindings(st, givenMap)
		},
	}
	if m.Type() == state.ModelTypeCAAS {
		return &caasDeployFromRepositoryValidator{
			validator: v,
		}
	}
	return &iaasDeployFromRepositoryValidator{
		validator: v,
	}
}

type deployFromRepositoryValidator struct {
	model Model
	state DeployFromRepositoryState

	mu          sync.Mutex
	repoFactory corecharm.RepositoryFactory
	// For testing using mocks.
	newRepoFactory     func(services.CharmRepoFactoryConfig) corecharm.RepositoryFactory
	charmhubHTTPClient facade.HTTPClient

	// For testing using mocks.
	newStateBindings func(st state.EndpointBinding, givenMap map[string]string) (Bindings, error)
}

// validateDeployFromRepositoryArgs does validation of all provided
// arguments. Returned is a deployTemplate which contains validated
// data necessary to deploy the application.
// Where possible, errors will be grouped and returned as a list.
func (v *deployFromRepositoryValidator) validate(arg params.DeployFromRepositoryArg) (deployTemplate, []error) {
	errs := make([]error, 0)

	if err := checkMachinePlacement(v.state, arg.ApplicationName, arg.Placement); err != nil {
		errs = append(errs, err)
	}

	initialCurl, requestedOrigin, usedModelDefaultBase, err := v.createOrigin(arg)
	if err != nil {
		errs = append(errs, err)
		return deployTemplate{}, errs
	}
	deployRepoLogger.Tracef("from createOrigin: %s, %s", initialCurl, pretty.Sprint(requestedOrigin))
	// TODO:
	// The logic in resolveCharm and getCharm can be improved as there is some
	// duplication. We call ResolveCharmWithPreferredChannel, then pick a
	// series, then call GetEssentialMetadata, which again calls ResolveCharmWithPreferredChannel
	// then a refresh request.

	charmURL, resolvedOrigin, err := v.resolveCharm(initialCurl, requestedOrigin, arg.Force, usedModelDefaultBase, arg.Cons)
	if err != nil {
		errs = append(errs, err)
		return deployTemplate{}, errs
	}
	deployRepoLogger.Tracef("from resolveCharm: %s, %s", charmURL, pretty.Sprint(resolvedOrigin))
	// Are we deploying a charm? if not, fail fast here.
	// TODO: add a ErrorNotACharm or the like for the juju client.

	// get the charm data to validate against, either a previously deployed
	// charm or the essential metadata from a charm to be async downloaded.
	resolvedOrigin, resolvedCharm, err := v.getCharm(charmURL, resolvedOrigin)
	if err != nil {
		errs = append(errs, err)
		return deployTemplate{}, errs
	}
	deployRepoLogger.Tracef("from getCharm: %s", charmURL, pretty.Sprint(resolvedOrigin))

	if resolvedCharm.Meta().Name == bootstrap.ControllerCharmName {
		errs = append(errs, errors.NotSupportedf("manual deploy of the controller charm"))
	}
	if resolvedCharm.Meta().Subordinate {
		if arg.NumUnits != nil && *arg.NumUnits != 0 {
			errs = append(errs, fmt.Errorf("subordinate application must be deployed without units"))
		}
		if !constraints.IsEmpty(&arg.Cons) {
			errs = append(errs, fmt.Errorf("subordinate application must be deployed without constraints"))
		}
	}
	if err := jujuversion.CheckJujuMinVersion(resolvedCharm.Meta().MinJujuVersion, jujuversion.Current); err != nil {
		errs = append(errs, err)
	}

	appNameForConfig := arg.CharmName
	if arg.ApplicationName != "" {
		appNameForConfig = arg.ApplicationName
	}
	appConfig, settings, err := v.appCharmSettings(appNameForConfig, arg.Trust, resolvedCharm.Config(), arg.ConfigYAML)
	if err != nil {
		errs = append(errs, err)
	}

	appName := resolvedCharm.Meta().Name
	if arg.ApplicationName != "" {
		appName = arg.ApplicationName
	}

	// Enforce "assumes" requirements if the feature flag is enabled.
	if err := assertCharmAssumptions(resolvedCharm.Meta().Assumes, v.model, v.state.ControllerConfig); err != nil {
		if !errors.Is(err, errors.NotSupported) || !arg.Force {
			errs = append(errs, err)
		}
		deployRepoLogger.Warningf("proceeding with deployment of application %q even though the charm feature requirements could not be met as --force was specified", appName)
	}

	var numUnits int
	if arg.NumUnits != nil {
		numUnits = *arg.NumUnits
	}

	// Validate the other args.
	dt := deployTemplate{
		applicationConfig: appConfig,
		applicationName:   appName,
		charm:             resolvedCharm,
		charmSettings:     settings,
		charmURL:          charmURL,
		dryRun:            arg.DryRun,
		force:             arg.Force,
		numUnits:          numUnits,
		origin:            resolvedOrigin,
		placement:         arg.Placement,
		storage:           stateStorageConstraints(arg.Storage),
	}
	if arg.NumUnits != nil {
		dt.numUnits = *arg.NumUnits
	} else {
		// The juju client defaults num units to 1. Ensure that a
		// charm deployed by any client has at least one if no
		// number provided.
		dt.numUnits = 1
	}
	if len(arg.EndpointBindings) > 0 {
		bindings, err := v.newStateBindings(v.state, arg.EndpointBindings)
		if err != nil {
			errs = append(errs, err)
		}
		dt.endpoints = bindings.Map()
	}

	if !resolvedCharm.Meta().Subordinate {
		dt.constraints = arg.Cons
	}
	deployRepoLogger.Tracef("validateDeployFromRepositoryArgs returning: %s", pretty.Sprint(dt))
	return dt, errs
}

type caasDeployFromRepositoryValidator struct {
	validator *deployFromRepositoryValidator
}

func (v caasDeployFromRepositoryValidator) ValidateArg(arg params.DeployFromRepositoryArg) (deployTemplate, []error) {
	// TODO: NumUnits
	// TODO: Storage
	dt, errs := v.validator.validate(arg)
	if corecharm.IsKubernetes(dt.charm) && charm.MetaFormat(dt.charm) == charm.FormatV1 {
		deployRepoLogger.Debugf("DEPRECATED: %q is a podspec charm, which will be removed in a future release", arg.CharmName)
	}
	return dt, errs
}

type iaasDeployFromRepositoryValidator struct {
	validator *deployFromRepositoryValidator
}

func (v *iaasDeployFromRepositoryValidator) ValidateArg(arg params.DeployFromRepositoryArg) (deployTemplate, []error) {
	// TODO: Storage
	return v.validator.validate(arg)
}

func (v *deployFromRepositoryValidator) createOrigin(arg params.DeployFromRepositoryArg) (*charm.URL, corecharm.Origin, bool, error) {
	path, err := charm.EnsureSchema(arg.CharmName, charm.CharmHub)
	if err != nil {
		return nil, corecharm.Origin{}, false, err
	}
	curl, err := charm.ParseURL(path)
	if err != nil {
		return nil, corecharm.Origin{}, false, err
	}
	if arg.Revision != nil {
		curl = curl.WithRevision(*arg.Revision)
	}
	if !charm.CharmHub.Matches(curl.Schema) {
		return nil, corecharm.Origin{}, false, errors.Errorf("unknown schema for charm URL %q", curl.String())
	}
	channelStr := corecharm.DefaultChannelString
	if arg.Channel != nil && *arg.Channel != "" {
		channelStr = *arg.Channel
	}
	channel, err := charm.ParseChannelNormalize(channelStr)
	if err != nil {
		return nil, corecharm.Origin{}, false, err
	}

	plat, usedModelDefaultBase, err := v.deducePlatform(arg)
	if err != nil {
		return nil, corecharm.Origin{}, false, err
	}

	origin := corecharm.Origin{
		Channel:  &channel,
		Platform: plat,
		Revision: arg.Revision,
		Source:   corecharm.CharmHub,
	}
	return curl, origin, usedModelDefaultBase, nil
}

// deducePlatform returns a platform for initial resolveCharm call.
// At minimum, it must contain an architecture.
// Platform is determined by the args: architecture constraint and
// provided base.
// - Check placement to determine known machine platform. If diffs from
// other provided data return error.
// - If no base provided, use model default base.
// - If no model default base, will be determined later.
// - If no architecture provided, use model default. Fallback
// to DefaultArchitecture.
func (v *deployFromRepositoryValidator) deducePlatform(arg params.DeployFromRepositoryArg) (corecharm.Platform, bool, error) {
	argArch := arg.Cons.Arch
	argBase := arg.Base
	var usedModelDefaultBase bool
	var usedModelDefaultArch bool

	// Try argBase with provided argArch and argBase first.
	platform := corecharm.Platform{}
	if argArch != nil {
		platform.Architecture = *argArch
	}
	// Fallback to model defaults if set. DefaultArchitecture otherwise.
	if platform.Architecture == "" {
		mConst, err := v.state.ModelConstraints()
		if err != nil {
			return corecharm.Platform{}, usedModelDefaultBase, err
		}
		if mConst.Arch != nil {
			platform.Architecture = *mConst.Arch
		} else {
			platform.Architecture = arch.DefaultArchitecture
			usedModelDefaultArch = true
		}
	}
	if argBase != nil {
		base, err := corebase.ParseBase(argBase.Name, argBase.Channel)
		if err != nil {
			return corecharm.Platform{}, usedModelDefaultBase, err
		}
		platform.OS = base.OS
		// platform channels don't model the concept of a risk
		// so ensure that only the track is included
		platform.Channel = base.Channel.Track
	}

	// Initial validation of platform from known data.
	_, err := corecharm.ParsePlatform(platform.String())
	if err != nil && !errors.Is(err, errors.BadRequest) {
		return corecharm.Platform{}, usedModelDefaultBase, err
	}

	// Match against platforms from placement
	placementPlatform, placementsMatch, err := v.platformFromPlacement(arg.Placement)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return corecharm.Platform{}, usedModelDefaultBase, err
	}
	if err == nil && !placementsMatch {
		return corecharm.Platform{}, usedModelDefaultBase, errors.BadRequestf("bases of existing placement machines do not match")
	}

	// No platform args, and one platform from placement, use that.
	if placementsMatch && usedModelDefaultArch && argBase == nil {
		return placementPlatform, usedModelDefaultBase, nil
	}
	if platform.Channel == "" {
		mCfg, err := v.model.Config()
		if err != nil {
			return corecharm.Platform{}, usedModelDefaultBase, err
		}
		if db, ok := mCfg.DefaultBase(); ok {
			defaultBase, err := corebase.ParseBaseFromString(db)
			if err != nil {
				return corecharm.Platform{}, usedModelDefaultBase, err
			}
			platform.OS = defaultBase.OS
			// platform channels don't model the concept of a risk
			// so ensure that only the track is included
			platform.Channel = defaultBase.Channel.Track
			usedModelDefaultBase = true
		}
	}
	return platform, usedModelDefaultBase, nil
}

func (v *deployFromRepositoryValidator) platformFromPlacement(placements []*instance.Placement) (corecharm.Platform, bool, error) {
	if len(placements) == 0 {
		return corecharm.Platform{}, false, errors.NotFoundf("placements")
	}
	machines := make([]Machine, 0)
	// Find which machines in placement actually exist.
	for _, placement := range placements {
		m, err := v.state.Machine(placement.Directive)
		if errors.Is(err, errors.NotFound) {
			continue
		}
		if err != nil {
			return corecharm.Platform{}, false, err
		}
		machines = append(machines, m)
	}
	if len(machines) == 0 {
		return corecharm.Platform{}, false, errors.NotFoundf("machines in placements")
	}

	// Gather platforms for existing machines
	var platform corecharm.Platform
	platStrings := set.NewStrings()
	for _, machine := range machines {
		b := machine.Base()
		a, err := machine.HardwareCharacteristics()
		if err != nil {
			return corecharm.Platform{}, false, err
		}
		platString := fmt.Sprintf("%s/%s/%s", *a.Arch, b.OS, b.Channel)
		p, err := corecharm.ParsePlatformNormalize(platString)
		if err != nil {
			return corecharm.Platform{}, false, err
		}
		platform = p
		platStrings.Add(p.String())
	}

	return platform, platStrings.Size() == 1, nil
}

func (v *deployFromRepositoryValidator) resolveCharm(curl *charm.URL, requestedOrigin corecharm.Origin, force, usedModelDefaultBase bool, cons constraints.Value) (*charm.URL, corecharm.Origin, error) {
	repo, err := v.getCharmRepository(requestedOrigin.Source)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}

	resultURL, resolvedOrigin, supportedSeries, resolveErr := repo.ResolveWithPreferredChannel(curl, requestedOrigin)
	if charm.IsUnsupportedSeriesError(resolveErr) {
		if !force {
			msg := fmt.Sprintf("%v. Use --force to deploy the charm anyway.", resolveErr)
			if usedModelDefaultBase {
				msg += " Used the default-series."
			}
			return nil, corecharm.Origin{}, errors.Errorf(msg)
		}
	} else if resolveErr != nil {
		return nil, corecharm.Origin{}, errors.Trace(resolveErr)
	}
	modelCons, err := v.state.ModelConstraints()
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}

	// The charmhub API can return "all" for architecture as it's not a real
	// arch we don't know how to correctly model it. "all " doesn't mean use the
	// default arch, it means use any arch which isn't quite the same. So if we
	// do get "all" we should see if there is a clean way to resolve it.
	if resolvedOrigin.Platform.Architecture == "all" {
		resolvedOrigin.Platform.Architecture = constraints.ArchOrDefault(modelCons, nil)
	}

	var seriesFlag string
	if requestedOrigin.Platform.OS != "" {
		var err error
		seriesFlag, err = corebase.GetSeriesFromChannel(requestedOrigin.Platform.OS, requestedOrigin.Platform.Channel)
		if err != nil {
			return nil, corecharm.Origin{}, errors.Trace(err)
		}
	}

	modelCfg, err := v.model.Config()
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}

	imageStream := modelCfg.ImageStream()

	workloadSeries, err := corebase.WorkloadSeries(jujuclock.WallClock.Now(), seriesFlag, imageStream)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}

	selector := corecharm.SeriesSelector{
		SeriesFlag:          seriesFlag,
		SupportedSeries:     supportedSeries,
		SupportedJujuSeries: workloadSeries,
		Force:               force,
		Conf:                modelCfg,
		FromBundle:          false,
		Logger:              deployRepoLogger,
		UsingImageID:        cons.HasImageID() || modelCons.HasImageID(),
	}
	err = selector.Validate()
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}

	// Get the series to use.
	series, err := selector.CharmSeries()
	deployRepoLogger.Tracef("Using series %q from %v to deploy %v", series, supportedSeries, curl)
	if charm.IsUnsupportedSeriesError(err) {
		msg := fmt.Sprintf("%v. Use --force to deploy the charm anyway.", err)
		if usedModelDefaultBase {
			msg += " Used the default-series."
		}
		return nil, corecharm.Origin{}, errors.Errorf(msg)
	}

	var base corebase.Base
	if series == corebase.Kubernetes.String() {
		base = corebase.LegacyKubernetesBase()
	} else {
		base, err = corebase.GetBaseFromSeries(series)
		if err != nil {
			return nil, corecharm.Origin{}, errors.Trace(err)
		}
	}
	resolvedOrigin.Platform.OS = base.OS
	resolvedOrigin.Platform.Channel = base.Channel.String()

	return resultURL, resolvedOrigin, nil
}

// getCharm returns the charm being deployed. Either it already has been
// used once and we get the data from state. Or we get the essential metadata.
func (v *deployFromRepositoryValidator) getCharm(charmURL *charm.URL, resolvedOrigin corecharm.Origin) (corecharm.Origin, charm.Charm, error) {
	repo, err := v.getCharmRepository(corecharm.CharmHub)
	if err != nil {
		return resolvedOrigin, nil, err
	}

	// Check if a charm doc already exists for this charm URL. If so, the
	// charm has already been queued for download so this is a no-op. We
	// still need to resolve and return back a suitable origin as charmhub
	// may refer to the same blob using the same revision in different
	// channels.
	//
	// We need to use GetDownloadURL instead of ResolveWithPreferredChannel
	// to ensure that the resolved origin has the ID/Hash fields correctly
	// populated.
	// TODO: Handle already deployed charm.
	//deployedCharm, err := api.backend.Charm(charmURL)
	//if err == nil {
	//	_, resolvedOrigin, err = repo.GetDownloadURL(charmURL, resolvedOrigin)
	//	if err != nil {
	//	}
	//}

	// Fetch the essential metadata that we require to deploy the charm
	// without downloading the full archive. The remaining metadata will
	// be populated once the charm gets downloaded.
	essentialMeta, err := repo.GetEssentialMetadata(corecharm.MetadataRequest{
		CharmURL: charmURL,
		Origin:   resolvedOrigin,
	})
	if err != nil {
		return resolvedOrigin, nil, errors.Annotatef(err, "retrieving essential metadata for charm %q", charmURL)
	}
	metaRes := essentialMeta[0]
	resolvedCharm := corecharm.NewCharmInfoAdapter(metaRes)
	return metaRes.ResolvedOrigin, resolvedCharm, nil
}

func (v *deployFromRepositoryValidator) appCharmSettings(appName string, trust bool, chCfg *charm.Config, configYAML string) (*config.Config, charm.Settings, error) {
	if !trust && configYAML == "" {
		return nil, nil, nil
	}
	// Cheat with trust. Trust is passed to DeployFromRepository as a flag, however
	// it's handled internally to juju as an application config. As DFR only
	// has charm config via yaml, stick trust into the config via map to enable
	// reuse of current parseCharmSettings as used with the old deploy and
	// setConfig.
	// At deploy time, there's no need to include "trust=false" as missing is the same thing.
	var cfg map[string]string
	if trust {
		cfg = map[string]string{"trust": "true"}
	}
	appConfig, _, charmSettings, _, err := parseCharmSettings(v.model.Type(), chCfg, appName, cfg, configYAML, environsconfig.NoDefaults)
	return appConfig, charmSettings, err
}

func (v *deployFromRepositoryValidator) getCharmRepository(src corecharm.Source) (corecharm.Repository, error) {
	// The following is only required for testing, as we generate api new http
	// client here for production.
	v.mu.Lock()
	if v.repoFactory != nil {
		defer v.mu.Unlock()
		return v.repoFactory.GetCharmRepository(src)
	}
	v.mu.Unlock()

	repoFactory := v.newRepoFactory(services.CharmRepoFactoryConfig{
		Logger:             deployRepoLogger,
		CharmhubHTTPClient: v.charmhubHTTPClient,
		StateBackend:       v.state,
		ModelBackend:       v.model,
	})

	return repoFactory.GetCharmRepository(src)
}
