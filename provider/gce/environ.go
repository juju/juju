// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"code.google.com/p/goauth2/oauth"
	"code.google.com/p/goauth2/oauth/jwt"
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
)

// This file contains the core of the gce Environ implementation. You will
// probably not need to change this file very much to begin with; and if you
// never need to add any more fields, you may never need to touch it.
//
// The rest of the implementation is split into environ_instance.go (which
// must be implemented ) and environ_firewall.go (which can be safely
// ignored until you've got an environment bootstrapping successfully).

const (
	driverScopes = "https://www.googleapis.com/auth/compute " +
		"https://www.googleapis.com/auth/devstorage.full_control"

	tokenURL = "https://accounts.google.com/o/oauth2/token"

	authURL = "https://accounts.google.com/o/oauth2/auth"

	storageScratch    = "SCRATCH"
	storagePersistent = "PERSISTENT"

	statusUp = "UP"
)

var (
	globalOperationTimeout = 60 * time.Second
	globalOperationDelay   = 10 * time.Second

	signedImageDataOnly = false
)

// TODO(ericsnow) This should move out to providers/common.
func (env *environ) machineFullName(machineId string) string {
	return fmt.Sprintf("juju-%s-%s", env.Config().Name(), names.NewMachineTag(machineId))
}

type environ struct {
	common.SupportsUnitPlacementPolicy

	name string

	lock    sync.Mutex
	ecfg    *environConfig
	storage storage.Storage

	gce       *compute.Service
	region    string
	projectID string
}

//TODO (wwitzel3): Investigate simplestreams.HasRegion for this provider
var _ environs.Environ = (*environ)(nil)

func (env *environ) Name() string {
	return env.name
}

func (*environ) Provider() environs.EnvironProvider {
	return providerInstance
}

func (env *environ) getRegionURL() string {
	// TODO(ericsnow) finish this!
	return ""
}

func (env *environ) waitOperation(operation *compute.Operation) error {
	env = env.getSnapshot()

	attempts := utils.AttemptStrategy{
		Total: globalOperationTimeout,
		Delay: globalOperationDelay,
	}
	for a := attempts.Start(); a.Next(); {
		var err error
		if operation.Status == "DONE" {
			return nil
		}
		// TODO(ericsnow) should projectID come from inst?
		call := env.gce.GlobalOperations.Get(env.projectID, operation.ClientOperationId)
		operation, err = call.Do()
		if err != nil {
			return errors.Annotate(err, "while waiting for operation to complete")
		}
	}
	return errors.Errorf("timed out after %d seconds waiting for GCE operation to finish", globalOperationTimeout)
}

var newToken = func(ecfg *environConfig, scopes string) (*oauth.Token, error) {
	jtok := jwt.NewToken(ecfg.clientEmail(), scopes, []byte(ecfg.privateKey()))
	jtok.ClaimSet.Aud = tokenURL

	token, err := jtok.Assert(&http.Client{})
	return token, errors.Trace(err)
}

var newService = func(transport *oauth.Transport) (*compute.Service, error) {
	return compute.New(transport.Client())
}

func (env *environ) SetConfig(cfg *config.Config) error {
	env.lock.Lock()
	defer env.lock.Unlock()
	var oldCfg *config.Config
	if env.ecfg != nil {
		oldCfg = env.ecfg.Config
	}
	ecfg, err := validateConfig(cfg, oldCfg)
	if err != nil {
		return err
	}
	storage, err := newStorage(ecfg)
	if err != nil {
		return err
	}
	env.ecfg = ecfg
	env.storage = storage

	token, err := newToken(ecfg, driverScopes)
	if err != nil {
		return errors.Annotate(err, "can't retrieve auth token")
	}

	transport := &oauth.Transport{
		Config: &oauth.Config{
			ClientId: ecfg.clientID(),
			Scope:    driverScopes,
			TokenURL: tokenURL,
			AuthURL:  authURL,
		},
		Token: token,
	}

	service, err := compute.New(transport.Client())
	if err != nil {
		return err
	}

	env.gce = service
	return nil
}

func (env *environ) getSnapshot() *environ {
	env.lock.Lock()
	clone := *env
	env.lock.Unlock()
	clone.lock = sync.Mutex{}
	return &clone
}

func (env *environ) Config() *config.Config {
	return env.getSnapshot().ecfg.Config
}

func (env *environ) Storage() storage.Storage {
	return env.getSnapshot().storage
}

func (env *environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (arch, series string, _ environs.BootstrapFinalizer, _ error) {
	// You can probably ignore this method; the common implementation should work.
	return common.Bootstrap(ctx, env, params)
}

func (env *environ) Destroy() error {
	// You can probably ignore this method; the common implementation should work.
	return common.Destroy(env)
}

func (env *environ) ConstraintsValidator() (constraints.Validator, error) {
	return nil, errNotImplemented
}

func (env *environ) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	return errNotImplemented
}

// firewall stuff

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) OpenPorts(ports []network.PortRange) error {
	return errNotImplemented
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) ClosePorts(ports []network.PortRange) error {
	return errNotImplemented
}

// Ports returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) Ports() ([]network.PortRange, error) {
	return nil, errNotImplemented
}

// instance stuff

func (env *environ) getRawInstance(zone string, id string) (*compute.Instance, error) {
	call := env.gce.Instances.Get(env.projectID, zone, id)
	gInst, err := call.Do()
	return gInst, errors.Trace(err)
}

func (env *environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	// Please note that this must *not* return instances that have not been
	// allocated as part of this environment -- if it does, juju will see they
	// are not tracked in state, assume they're stale/rogue, and shut them down.
	// This advice applies even if an instance id passed in corresponds to a
	// real instance that's not part of the environment -- the Environ should
	// treat that no differently to a request for one that does not exist.
	_ = env.getSnapshot()
	return nil, errNotImplemented
}

func (env *environ) StateServerInstances() ([]instance.Id, error) {
	return nil, errNotImplemented
}

func (env *environ) SupportedArchitectures() ([]string, error) {
	return arch.AllSupportedArches, nil
}

// Networks

// SupportAddressAllocation takes a network.Id and returns a bool
// and an error. The bool indicates whether that network supports
// static ip address allocation.
func (env *environ) SupportAddressAllocation(netId network.Id) (bool, error) {
	return false, nil
}

// AllocateAddress requests a specific address to be allocated for the
// given instance on the given network.
func (env *environ) AllocateAddress(instId instance.Id, netId network.Id, addr network.Address) error {
	return errNotImplemented
}

func (env *environ) ReleaseAddress(instId instance.Id, netId network.Id, addr network.Address) error {
	return errNotImplemented
}

func (env *environ) Subnets(inst instance.Id) ([]network.BasicInfo, error) {
	return nil, errNotImplemented
}

func (env *environ) ListNetworks(inst instance.Id) ([]network.BasicInfo, error) {
	return nil, errNotImplemented
}

// SupportNetworks returns whether the environment has support to
// specify networks for services and machines.
func (env *environ) SupportNetworks() bool {
	return false
}

// SupportsUnitAssignment returns an error which, if non-nil, indicates
// that the environment does not support unit placement. If the environment
// does not support unit placement, then machines may not be created
// without units, and units cannot be placed explcitly.
func (env *environ) SupportsUnitPlacement() error {
	return errNotImplemented
}
