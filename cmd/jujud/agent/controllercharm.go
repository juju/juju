// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/facades/client/charms/interfaces"
	"github.com/juju/juju/apiserver/facades/client/charms/services"
	"github.com/juju/juju/charmhub"
	corearch "github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	coreseries "github.com/juju/juju/core/series"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/state"
	statestorage "github.com/juju/juju/state/storage"
	jujuversion "github.com/juju/juju/version"
)

const controllerCharmURL = "ch:juju-controller"

func (c *BootstrapCommand) deployControllerCharm(st *state.State, cons constraints.Value, charmPath string, channel charm.Channel, isCAAS bool, unitPassword string) (resultErr error) {
	arch := corearch.DefaultArchitecture
	base := jujuversion.DefaultSupportedLTSBase()
	if cons.HasArch() {
		arch = *cons.Arch
	}

	var controllerUnit *state.Unit
	controllerAddress := ""
	if isCAAS {
		s, err := st.CloudService(st.ControllerUUID())
		if err != nil {
			return errors.Trace(err)
		}
		hp := network.SpaceAddressesWithPort(s.Addresses(), 0)
		addr := hp.AllMatchingScope(network.ScopeMatchCloudLocal)
		if len(addr) > 0 {
			controllerAddress = addr[0]
		}
		logger.Debugf("CAAS controller address %v", controllerAddress)
		// For k8s, we need to set the charm unit agent password.
		defer func() {
			if resultErr == nil && controllerUnit != nil {
				resultErr = controllerUnit.SetPassword(unitPassword)
			}
		}()
	} else {
		m, err := st.Machine(agent.BootstrapControllerId)
		if err != nil {
			return errors.Trace(err)
		}
		defer func() {
			if resultErr == nil && controllerUnit != nil {
				resultErr = controllerUnit.AssignToMachine(m)
			}
		}()
		base, err = coreseries.ParseBase(m.Base().OS, m.Base().Channel)
		if err != nil {
			return errors.Trace(err)
		}
		pa, err := m.PublicAddress()
		if err != nil && !network.IsNoAddressError(err) {
			return errors.Trace(err)
		}
		if err == nil {
			controllerAddress = pa.Value
		}
	}

	// First try using a local charm specified at bootstrap time.
	source := "local"
	curl, origin, err := populateLocalControllerCharm(st, c.DataDir(), arch, base)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Annotate(err, "deploying local controller charm")
	}
	// If no local charm, use the one from charmhub.
	if err != nil {
		source = "store"
		if curl, origin, err = populateStoreControllerCharm(st, charmPath, channel, arch, base); err != nil {
			return errors.Annotate(err, "deploying charmhub controller charm")
		}
	}
	// Always for the controller charm to use the same base as the controller.
	// This avoids the situation where we cannot deploy a slightly stale
	// controller charm onto a newer machine at bootstrap.
	origin.Platform.OS = base.OS
	origin.Platform.Channel = base.Channel.String()

	// Once the charm is added, set up the controller application.
	controllerUnit, err = addControllerApplication(
		st, curl, *origin, cons, controllerAddress)
	if err != nil {
		return errors.Annotate(err, "cannot add controller application")
	}

	// Force CAAS unit's providerId on CAAS.
	// TODO(caas): this assumes k8s is the provider
	if isCAAS {
		providerID := fmt.Sprintf("controller-%d", controllerUnit.UnitTag().Number())
		op := controllerUnit.UpdateOperation(state.UnitUpdateProperties{
			ProviderId: &providerID,
		})
		err = st.ApplyOperation(op)
		if err != nil {
			return errors.Annotate(err, "cannot update controller unit")
		}
	}

	logger.Debugf("Successfully deployed %s Juju controller charm", source)
	return nil
}

// These are patched for testing.
var (
	newCharmRepo = func(cfg services.CharmRepoFactoryConfig) (corecharm.Repository, error) {
		charmRepoFactory := services.NewCharmRepoFactory(cfg)
		return charmRepoFactory.GetCharmRepository(corecharm.CharmHub)
	}
	newCharmDownloader = func(cfg services.CharmDownloaderConfig) (interfaces.Downloader, error) {
		return services.NewCharmDownloader(cfg)
	}
)

// populateStoreControllerCharm downloads and stores the controller charm from charmhub.
func populateStoreControllerCharm(st *state.State, charmPath string, channel charm.Channel, arch string, base coreseries.Base) (*charm.URL, *corecharm.Origin, error) {
	model, err := st.Model()
	if err != nil {
		return nil, nil, err
	}

	charmhubHTTPClient := charmhub.DefaultHTTPClient(logger)

	stateBackend := &stateShim{st}
	charmRepo, err := newCharmRepo(services.CharmRepoFactoryConfig{
		Logger:             logger,
		CharmhubHTTPClient: charmhubHTTPClient,
		StateBackend:       stateBackend,
		ModelBackend:       model,
	})
	if err != nil {
		return nil, nil, err
	}

	var curl *charm.URL
	if charmPath == "" {
		curl = charm.MustParseURL(controllerCharmURL)
	} else {
		curl = charm.MustParseURL(charmPath)
	}
	if err != nil {
		return nil, nil, err
	}
	origin := corecharm.Origin{
		Source:  corecharm.CharmHub,
		Channel: &channel,
		Platform: corecharm.Platform{
			Architecture: arch,
			OS:           base.OS,
			Channel:      base.Channel.Track,
		},
	}

	// Since we're running on the machine to which the controller charm will be
	// deployed, we know the exact platform to ask for, not need to review the
	// supported series.
	//
	// We prefer the latest LTS series, if the current series is not one,
	// charmRepo.ResolveWithPreferredChannel, will return an origin with the
	// latest LTS based on data provided by charmhub in the revision-not-found
	// error response.
	//
	// The controller charm doesn't have any series specific code.
	curl, origin, _, err = charmRepo.ResolveWithPreferredChannel(curl, origin)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "resolving %q", controllerCharmURL)
	}

	storageFactory := func(modelUUID string) services.Storage {
		return statestorage.NewStorage(model.UUID(), st.MongoSession())
	}
	charmDownloader, err := newCharmDownloader(services.CharmDownloaderConfig{
		Logger:             logger,
		CharmhubHTTPClient: charmhubHTTPClient,
		StorageFactory:     storageFactory,
		StateBackend:       stateBackend,
		ModelBackend:       model,
	})
	if err != nil {
		return nil, nil, err
	}
	resOrigin, err := charmDownloader.DownloadAndStore(curl, origin, false)
	if err != nil {
		return nil, nil, err
	}

	return curl, &resOrigin, nil
}

// stateShim allows us to use a real state instance with the charm services logic.
type stateShim struct {
	*state.State
}

func (st *stateShim) PrepareCharmUpload(curl *charm.URL) (services.UploadedCharm, error) {
	return st.State.PrepareCharmUpload(curl)
}

func (st *stateShim) UpdateUploadedCharm(info state.CharmInfo) (services.UploadedCharm, error) {
	return st.State.UpdateUploadedCharm(info)
}

// populateLocalControllerCharm downloads and stores a local controller charm archive.
func populateLocalControllerCharm(st *state.State, dataDir, arch string, base coreseries.Base) (*charm.URL, *corecharm.Origin, error) {
	controllerCharmPath := filepath.Join(dataDir, "charms", bootstrap.ControllerCharmArchive)
	_, err := os.Stat(controllerCharmPath)
	if os.IsNotExist(err) {
		return nil, nil, errors.NotFoundf(controllerCharmPath)
	}
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	curl, err := addLocalControllerCharm(st, base, controllerCharmPath)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "cannot store controller charm at %q", controllerCharmPath)
	}
	logger.Debugf("Successfully deployed local Juju controller charm")
	origin := corecharm.Origin{
		Source: corecharm.Local,
		Type:   "charm",
		Platform: corecharm.Platform{
			Architecture: arch,
			OS:           base.OS,
			Channel:      base.Channel.String(),
		},
	}
	return curl, &origin, nil
}

// addLocalControllerCharm adds the specified local charm to the controller.
func addLocalControllerCharm(st *state.State, base coreseries.Base, charmFileName string) (*charm.URL, error) {
	archive, err := charm.ReadCharmArchive(charmFileName)
	if err != nil {
		return nil, errors.Errorf("invalid charm archive: %v", err)
	}

	name := archive.Meta().Name
	if name != bootstrap.ControllerCharmName {
		return nil, errors.Errorf("unexpected controller charm name %q", name)
	}

	series, err := coreseries.GetSeriesFromBase(base)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Reserve a charm URL for it in state.
	curl := &charm.URL{
		Schema:   charm.Local.String(),
		Name:     name,
		Revision: archive.Revision(),
		Series:   series,
	}
	curl, err = st.PrepareLocalCharmUpload(curl)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Now we need to repackage it with the reserved URL, upload it to
	// provider storage and update the state.
	err = apiserver.RepackageAndUploadCharm(st, archive, curl)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return curl, nil
}

// addControllerApplication deploys and configures the controller application.
func addControllerApplication(
	st *state.State,
	curl *charm.URL,
	origin corecharm.Origin,
	cons constraints.Value,
	address string,
) (*state.Unit, error) {
	ch, err := st.Charm(curl)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg := charm.Settings{
		"is-juju": true,
	}
	controllerCfg, err := st.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg["identity-provider-url"] = controllerCfg.IdentityURL()
	addr := controllerCfg.PublicDNSAddress()
	if addr == "" {
		addr = address
	}
	if addr != "" {
		cfg["controller-url"] = api.ControllerAPIURL(addr, controllerCfg.APIPort())
	}

	configSchema := environschema.Fields{
		application.TrustConfigOptionName: {
			Type: environschema.Tbool,
		},
	}
	appCfg, err := config.NewConfig(nil, configSchema, schema.Defaults{
		application.TrustConfigOptionName: true,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	stateOrigin, err := application.StateCharmOrigin(origin)
	if err != nil {
		return nil, errors.Trace(err)
	}

	app, err := st.AddApplication(state.AddApplicationArgs{
		Name:              bootstrap.ControllerApplicationName,
		Charm:             ch,
		CharmOrigin:       stateOrigin,
		CharmConfig:       cfg,
		Constraints:       cons,
		ApplicationConfig: appCfg,
		NumUnits:          1,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st.Unit(app.Name() + "/0")
}
