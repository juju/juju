// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"os"
	"path/filepath"

	charm "github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/schema"
	environschema "gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/facades/client/charms/interfaces"
	"github.com/juju/juju/apiserver/facades/client/charms/services"
	"github.com/juju/juju/charmhub"
	corearch "github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/state"
	statestorage "github.com/juju/juju/state/storage"
	jujuversion "github.com/juju/juju/version"
)

const controllerCharmURL = "ch:juju-controller"

type controllerCharmArgs struct {
	cons         constraints.Value
	charmPath    string
	channel      charm.Channel
	isCAAS       bool
	unitPassword string
	sshProxyPort int
}

type controllerCharmDeployer struct {
	controllerCharmArgs

	// The values below are populated during the deployment process.

	// machine is only populated for IAAS deployments and contains
	// info about the machine where the controller is deployed.
	machine           *state.Machine
	arch              string
	base              corebase.Base
	controllerAddress string
	source            string
	controllerUnit    *state.Unit
	dataDir           string
}

func (c *BootstrapCommand) deployControllerCharm(st *state.State, args controllerCharmArgs) error {
	arch := corearch.DefaultArchitecture
	base := jujuversion.DefaultSupportedLTSBase()
	if args.cons.HasArch() {
		arch = *args.cons.Arch
	}

	ccd := controllerCharmDeployer{
		controllerCharmArgs: args,
		arch:                arch,
		base:                base,
		dataDir:             c.DataDir(),
	}

	if err := ccd.deploy(st); err != nil {
		return err
	}

	logger.Debugf("Successfully deployed %s Juju controller charm", ccd.source)

	return nil
}

func (ccd *controllerCharmDeployer) deploy(st *state.State) error {
	if err := ccd.getControllerDetails(st); err != nil {
		return err
	}

	if err := ccd.deployCharm(st); err != nil {
		return err
	}

	if err := ccd.finishUnitSetup(st); err != nil {
		return err
	}

	if err := ccd.openProxySSHPort(st); err != nil {
		return err
	}

	logger.Debugf("Successfully deployed %s Juju controller charm", ccd.source)

	return nil
}

func (ccd *controllerCharmDeployer) deployCharm(st *state.State) error {
	// First try using a local charm specified at bootstrap time.
	ccd.source = "local"
	curl, origin, err := populateLocalControllerCharm(st, ccd.dataDir, ccd.arch, ccd.base)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Annotate(err, "deploying local controller charm")
	}
	// If no local charm, use the one from charmhub.
	if err != nil {
		ccd.source = "store"
		if curl, origin, err = populateStoreControllerCharm(st, ccd.charmPath, ccd.channel, ccd.arch, ccd.base); err != nil {
			return errors.Annotate(err, "deploying charmhub controller charm")
		}
	}
	// Always for the controller charm to use the same base as the controller.
	// This avoids the situation where we cannot deploy a slightly stale
	// controller charm onto a newer machine at bootstrap.
	origin.Platform.OS = ccd.base.OS
	origin.Platform.Channel = ccd.base.Channel.String()

	// Once the charm is added, set up the controller application.
	ccd.controllerUnit, err = addControllerApplication(
		st, curl, *origin, ccd.cons, ccd.controllerAddress)
	if err != nil {
		return errors.Annotate(err, "cannot add controller application")
	}
	return nil
}

func (ccd *controllerCharmDeployer) getControllerDetails(st *state.State) error {
	// Force CAAS unit's providerId on CAAS.
	// TODO(caas): this assumes k8s is the provider
	if ccd.isCAAS {
		s, err := st.CloudService(st.ControllerUUID())
		if err != nil {
			return errors.Trace(err)
		}
		hp := network.SpaceAddressesWithPort(s.Addresses(), 0)
		addr := hp.AllMatchingScope(network.ScopeMatchCloudLocal)
		if len(addr) > 0 {
			ccd.controllerAddress = addr[0]
		}
		logger.Debugf("CAAS controller address %v", ccd.controllerAddress)
	} else {
		var err error
		ccd.machine, err = st.Machine(agent.BootstrapControllerId)
		if err != nil {
			return errors.Trace(err)
		}
		ccd.base, err = corebase.ParseBase(ccd.machine.Base().OS, ccd.machine.Base().Channel)
		if err != nil {
			return errors.Trace(err)
		}
		pa, err := ccd.machine.PublicAddress()
		if err != nil && !network.IsNoAddressError(err) {
			return errors.Trace(err)
		}
		if err == nil {
			ccd.controllerAddress = pa.Value
		}
	}
	return nil
}

func (ccd *controllerCharmDeployer) finishUnitSetup(st *state.State) error {
	if ccd.isCAAS {
		providerID := fmt.Sprintf("controller-%d", ccd.controllerUnit.UnitTag().Number())
		op := ccd.controllerUnit.UpdateOperation(state.UnitUpdateProperties{
			ProviderId: &providerID,
		})
		err := st.ApplyOperation(op)
		if err != nil {
			return errors.Annotate(err, "cannot update controller unit")
		}
		// For k8s, we need to set the charm unit agent password.
		return ccd.controllerUnit.SetPassword(ccd.unitPassword)
	} else {
		return ccd.controllerUnit.AssignToMachine(ccd.machine)
	}
}

func (c *controllerCharmDeployer) openProxySSHPort(st *state.State) error {
	pcp, err := c.controllerUnit.OpenedPortRanges()
	if err != nil {
		return errors.Trace(err)
	}
	pcp.Open("", network.PortRange{
		FromPort: c.sshProxyPort,
		ToPort:   c.sshProxyPort,
		Protocol: "tcp",
	})
	if err = st.ApplyOperation(pcp.Changes()); err != nil {
		return errors.Trace(err)
	}
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
func populateStoreControllerCharm(st *state.State, charmPath string, channel charm.Channel, arch string, base corebase.Base) (*charm.URL, *corecharm.Origin, error) {
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
	// supported bases.
	//
	// We prefer the latest LTS base, if the current base is not one,
	// charmRepo.ResolveWithPreferredChannel, will return an origin with the
	// latest LTS based on data provided by charmhub in the revision-not-found
	// error response.
	//
	// The controller charm doesn't have any base specific code.
	curl, origin, _, err = charmRepo.ResolveWithPreferredChannel(curl.Name, origin)
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

func (st *stateShim) PrepareCharmUpload(curl string) (services.UploadedCharm, error) {
	return st.State.PrepareCharmUpload(curl)
}

func (st *stateShim) UpdateUploadedCharm(info state.CharmInfo) (services.UploadedCharm, error) {
	return st.State.UpdateUploadedCharm(info)
}

// populateLocalControllerCharm downloads and stores a local controller charm archive.
func populateLocalControllerCharm(st *state.State, dataDir, arch string, base corebase.Base) (*charm.URL, *corecharm.Origin, error) {
	controllerCharmPath := filepath.Join(dataDir, "charms", bootstrap.ControllerCharmArchive)
	_, err := os.Stat(controllerCharmPath)
	if os.IsNotExist(err) {
		return nil, nil, errors.NotFoundf(controllerCharmPath)
	}
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	curl, err := addLocalControllerCharm(st, controllerCharmPath)
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
func addLocalControllerCharm(st *state.State, charmFileName string) (*charm.URL, error) {
	archive, err := charm.ReadCharmArchive(charmFileName)
	if err != nil {
		return nil, errors.Errorf("invalid charm archive: %v", err)
	}

	name := archive.Meta().Name
	if name != bootstrap.ControllerCharmName {
		return nil, errors.Errorf("unexpected controller charm name %q", name)
	}

	// Reserve a charm URL for it in state.
	curl := &charm.URL{
		Schema:   charm.Local.String(),
		Name:     name,
		Revision: archive.Revision(),
	}
	curl, err = st.PrepareLocalCharmUpload(curl.String())
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Now we need to repackage it with the reserved URL, upload it to
	// provider storage and update the state.
	err = apiserver.RepackageAndUploadCharm(st, archive, curl.String(), archive.Revision())
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
	ch, err := st.Charm(curl.String())
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

	// Expose all endpoints of the controller app
	mappedExposeParams := map[string]state.ExposedEndpoint{
		"": {
			ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR},
		},
	}
	if err := app.MergeExposeSettings(mappedExposeParams); err != nil {
		return nil, errors.Trace(err)
	}

	return st.Unit(app.Name() + "/0")
}
