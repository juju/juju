// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This worker is responsible for watching the life cycle of CAAS pod-spec
// applications and creating their operator pods (or removing them). Unlike
// the caasapplicationprovisioner worker, this worker does not create a new
// child worker for every application being monitored.
//
// Note that the separate caasapplicationprovisioner worker handles CAAS
// sidecar applications.

package caasoperatorprovisioner

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/juju/charm/v11"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/agent"
	charmscommon "github.com/juju/juju/api/common/charms"
	apicaasprovisioner "github.com/juju/juju/api/controller/caasoperatorprovisioner"
	"github.com/juju/juju/caas"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
)

// Logger is here to stop the desire of creating a package level Logger.
// Don't do this, instead use the one passed as manifold config.
type logger interface{}

var _ logger = struct{}{}

// CAASProvisionerFacade exposes CAAS provisioning functionality to a worker.
type CAASProvisionerFacade interface {
	OperatorProvisioningInfo(string) (apicaasprovisioner.OperatorProvisioningInfo, error)
	WatchApplications() (watcher.StringsWatcher, error)
	SetPasswords([]apicaasprovisioner.ApplicationPassword) (params.ErrorResults, error)
	Life(string) (life.Value, error)
	IssueOperatorCertificate(string) (apicaasprovisioner.OperatorCertificate, error)
	ApplicationCharmInfo(appName string) (*charmscommon.CharmInfo, error)
}

// Config defines the operation of a Worker.
type Config struct {
	Facade          CAASProvisionerFacade
	OperatorManager caas.ApplicationOperatorManager
	ModelTag        names.ModelTag
	AgentConfig     agent.Config
	Clock           clock.Clock
	Logger          Logger
}

// NewProvisionerWorker starts and returns a new CAAS provisioner worker.
func NewProvisionerWorker(config Config) (worker.Worker, error) {
	p := &provisioner{
		provisionerFacade: config.Facade,
		operatorManager:   config.OperatorManager,
		modelTag:          config.ModelTag,
		agentConfig:       config.AgentConfig,
		clock:             config.Clock,
		logger:            config.Logger,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &p.catacomb,
		Work: p.loop,
	})
	return p, err
}

type provisioner struct {
	catacomb          catacomb.Catacomb
	provisionerFacade CAASProvisionerFacade
	operatorManager   caas.ApplicationOperatorManager
	clock             clock.Clock
	logger            Logger

	modelTag    names.ModelTag
	agentConfig agent.Config
}

// Kill is part of the worker.Worker interface.
func (p *provisioner) Kill() {
	p.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (p *provisioner) Wait() error {
	return p.catacomb.Wait()
}

func (p *provisioner) loop() error {
	// TODO(caas) -  this loop should also keep an eye on kubernetes and ensure
	// that the operator stays up, redeploying it if the pod goes
	// away. For some runtimes we *could* rely on the the runtime's
	// features to do this.

	appWatcher, err := p.provisionerFacade.WatchApplications()
	if err != nil {
		return errors.Trace(err)
	}
	if err := p.catacomb.Add(appWatcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-p.catacomb.Dying():
			return p.catacomb.ErrDying()

		// CAAS applications changed so either create or remove pods as appropriate.
		case apps, ok := <-appWatcher.Changes():
			if !ok {
				return errors.New("app watcher closed channel")
			}
			var newApps []string
			for _, app := range apps {
				// Ignore events for v2 charms.
				format, err := p.charmFormat(app)
				if errors.IsNotFound(err) {
					p.logger.Debugf("application %q no longer exists", app)
					continue
				} else if err != nil {
					return errors.Trace(err)
				}
				if format > charm.FormatV1 {
					p.logger.Tracef("application %q is v2, ignoring event", app)
					continue
				}

				// Process events for v1 charms.
				appLife, err := p.provisionerFacade.Life(app)
				if err != nil && !errors.IsNotFound(err) {
					return errors.Trace(err)
				}
				if err != nil || appLife == life.Dead {
					p.logger.Debugf("deleting operator for %q", app)
					if err := p.operatorManager.DeleteOperator(app); err != nil {
						return errors.Annotatef(err, "failed to stop operator for %q", app)
					}
					continue
				}
				if appLife != life.Alive {
					continue
				}
				newApps = append(newApps, app)
			}
			if len(newApps) == 0 {
				continue
			}
			if err := p.ensureOperators(newApps); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (p *provisioner) charmFormat(appName string) (charm.Format, error) {
	charmInfo, err := p.provisionerFacade.ApplicationCharmInfo(appName)
	if err != nil {
		return charm.FormatUnknown, errors.Annotatef(err, "failed to get charm info for application %q", appName)
	}
	return charm.MetaFormat(charmInfo.Charm()), nil
}

func (p *provisioner) waitForOperatorTerminated(app string) error {
	tryAgain := errors.New("try again")
	existsFunc := func() error {
		opState, err := p.operatorManager.OperatorExists(app)
		if err != nil {
			return errors.Trace(err)
		}
		if !opState.Exists {
			return nil
		}
		if opState.Exists && !opState.Terminating {
			return errors.Errorf("operator %q should be terminating but is now running", app)
		}
		return tryAgain
	}
	retryCallArgs := retry.CallArgs{
		Attempts:    60,
		Delay:       3 * time.Second,
		MaxDuration: 3 * time.Minute,
		Clock:       p.clock,
		Func:        existsFunc,
		IsFatalError: func(err error) bool {
			return err != tryAgain
		},
	}
	return errors.Trace(retry.Call(retryCallArgs))
}

// ensureOperators creates operator pods for the specified app names -> api passwords.
func (p *provisioner) ensureOperators(apps []string) error {
	var appPasswords []apicaasprovisioner.ApplicationPassword
	operatorConfig := make([]*caas.OperatorConfig, len(apps))
	for i, app := range apps {
		opState, err := p.operatorManager.OperatorExists(app)
		if err != nil {
			return errors.Annotatef(err, "failed to find operator for %q", app)
		}
		if opState.Exists && opState.Terminating {
			// We can't deploy an app while a previous version is terminating.
			// TODO(caas) - the remove application process should block until app terminated
			// TODO(caas) - consider making this async, but ok for now as it's a corner case
			if err := p.waitForOperatorTerminated(app); err != nil {
				return errors.Annotatef(err, "operator for %q was terminating and there was an error waiting for it to stop", app)
			}
			opState.Exists = false
		}

		op, err := p.operatorManager.Operator(app)
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}

		// If the operator does not exist already, we need to create an initial
		// password for it.
		var password string
		if !opState.Exists {
			if password, err = utils.RandomPassword(); err != nil {
				return errors.Trace(err)
			}
			appPasswords = append(appPasswords, apicaasprovisioner.ApplicationPassword{Name: app, Password: password})
		}

		var prevCfg caas.OperatorConfig
		if op != nil && op.Config != nil {
			prevCfg = *op.Config
		}
		config, err := p.updateOperatorConfig(app, password, prevCfg)
		if err != nil {
			return errors.Annotatef(err, "failed to generate operator config for %q", app)
		}
		operatorConfig[i] = config
	}
	// If we did create any passwords for new operators, first they need
	// to be saved so the agent can login when it starts up.
	if len(appPasswords) > 0 {
		errorResults, err := p.provisionerFacade.SetPasswords(appPasswords)
		if err != nil {
			return errors.Annotate(err, "failed to set application api passwords")
		}
		if err := errorResults.Combine(); err != nil {
			return errors.Annotate(err, "failed to set application api passwords")
		}
	}

	// Now that any new config/passwords are done, create or update
	// the operators themselves.
	var errorStrings []string
	for i, app := range apps {
		if err := p.ensureOperator(app, operatorConfig[i]); err != nil {
			errorStrings = append(errorStrings, err.Error())
			continue
		}
	}
	if errorStrings != nil {
		err := errors.New(strings.Join(errorStrings, "\n"))
		return errors.Annotate(err, "failed to provision all operators")
	}
	return nil
}

func (p *provisioner) ensureOperator(app string, config *caas.OperatorConfig) error {
	if err := p.operatorManager.EnsureOperator(app, p.agentConfig.DataDir(), config); err != nil {
		return errors.Annotatef(err, "failed to start operator for %q", app)
	}
	p.logger.Infof("started operator for application %q", app)
	return nil
}

func (p *provisioner) updateOperatorConfig(appName, password string, prevCfg caas.OperatorConfig) (*caas.OperatorConfig, error) {
	info, err := p.provisionerFacade.OperatorProvisioningInfo(appName)
	if err != nil {
		return nil, errors.Annotatef(err, "fetching operator provisioning info")
	}
	// Operators may have storage configured because charms
	// have persistent state which must be preserved between any
	// operator restarts. Newer charms though store state in the controller.
	if info.CharmStorage != nil && info.CharmStorage.Provider != k8sconstants.StorageProviderType {
		if spType := info.CharmStorage.Provider; spType == "" {
			return nil, errors.NotValidf("missing operator storage provider")
		} else {
			return nil, errors.NotSupportedf("operator storage provider %q", spType)
		}
	}
	p.logger.Tracef("using operator info %+v", info)

	cfg := &caas.OperatorConfig{
		ImageDetails:        info.ImageDetails,
		BaseImageDetails:    info.BaseImageDetails,
		Version:             info.Version,
		ResourceTags:        info.Tags,
		CharmStorage:        charmStorageParams(info.CharmStorage),
		ConfigMapGeneration: prevCfg.ConfigMapGeneration,
	}

	cfg.AgentConf, err = p.updateAgentConf(appName, password, info, prevCfg.AgentConf)
	if err != nil {
		return nil, errors.Annotatef(err, "updating agent config")
	}

	cfg.OperatorInfo, err = p.updateOperatorInfo(appName, prevCfg.OperatorInfo)
	if err != nil {
		return nil, errors.Annotatef(err, "updating operator info")
	}

	return cfg, nil
}

func (p *provisioner) updateAgentConf(appName, password string,
	info apicaasprovisioner.OperatorProvisioningInfo,
	prevAgentConfData []byte) ([]byte, error) {
	if len(prevAgentConfData) == 0 && password == "" {
		return nil, errors.NewNotValid(nil, fmt.Sprintf("no existing agent conf found and no new password generated for %q operator", appName))
	}
	if password == "" {
		// Read password from previous agent config for the existing operator.
		prevAgentConf, err := agent.ParseConfigData(prevAgentConfData)
		if err != nil {
			return nil, errors.Trace(err)
		}
		password = prevAgentConf.OldPassword()
	}

	appTag := names.NewApplicationTag(appName)
	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths: agent.Paths{
				DataDir: p.agentConfig.DataDir(),
				LogDir:  p.agentConfig.LogDir(),
			},
			Tag:          appTag,
			Controller:   p.agentConfig.Controller(),
			Model:        p.modelTag,
			APIAddresses: info.APIAddresses,
			CACert:       p.agentConfig.CACert(),
			Password:     password,

			// UpgradedToVersion is mandatory but not used by
			// caas operator agents as they are not upgraded insitu.
			UpgradedToVersion: info.Version,
		},
	)
	if err != nil {
		return nil, errors.Annotatef(err, "creating new agent config")
	}
	newAgentConfData, err := conf.Render()
	if err != nil {
		return nil, errors.Annotatef(err, "rendering new agent config")
	}
	p.logger.Debugf("agentConfData for %q changed %v", appName, !bytes.Equal(prevAgentConfData, newAgentConfData))
	return newAgentConfData, nil
}

func (p *provisioner) updateOperatorInfo(appName string, prevOperatorInfoData []byte) ([]byte, error) {
	var operatorInfo caas.OperatorInfo
	if prevOperatorInfoData != nil {
		prevOperatorInfo, err := caas.UnmarshalOperatorInfo(prevOperatorInfoData)
		if err != nil {
			return nil, errors.Annotatef(err, "unmarshalling operator info")
		}
		operatorInfo = *prevOperatorInfo
	}

	if operatorInfo.Cert == "" ||
		operatorInfo.PrivateKey == "" ||
		operatorInfo.CACert == "" {
		cert, err := p.provisionerFacade.IssueOperatorCertificate(appName)
		if err != nil {
			return nil, errors.Annotatef(err, "issuing certificate")
		}
		operatorInfo.Cert = cert.Cert
		operatorInfo.PrivateKey = cert.PrivateKey
		operatorInfo.CACert = cert.CACert
	}

	return operatorInfo.Marshal()
}

func charmStorageParams(in *storage.KubernetesFilesystemParams) *caas.CharmStorageParams {
	if in == nil {
		return nil
	}
	return &caas.CharmStorageParams{
		Provider:     in.Provider,
		Size:         in.Size,
		Attributes:   in.Attributes,
		ResourceTags: in.ResourceTags,
	}
}
