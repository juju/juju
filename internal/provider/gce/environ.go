// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"google.golang.org/api/compute/v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	jujuhttp "github.com/juju/juju/internal/http"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/provider/gce/google"
)

type gceConnection interface {
	VerifyCredentials() error

	// Instance gets the up-to-date info about the given instance
	// and returns it.
	Instance(id, zone string) (google.Instance, error)
	Instances(prefix string, statuses ...string) ([]google.Instance, error)
	AddInstance(spec google.InstanceSpec) (*google.Instance, error)
	RemoveInstances(prefix string, ids ...string) error
	UpdateMetadata(key, value string, ids ...string) error

	IngressRules(fwname string) (firewall.IngressRules, error)
	OpenPorts(fwname string, rules firewall.IngressRules) error
	ClosePorts(fwname string, rules firewall.IngressRules) error
	RemoveFirewall(fwname string) error

	AvailabilityZones(region string) ([]google.AvailabilityZone, error)
	// Subnetworks returns the subnetworks that machines can be
	// assigned to in the given region.
	Subnetworks(region string) ([]*compute.Subnetwork, error)
	// Networks returns the available networks that exist across
	// regions.
	Networks() ([]*compute.Network, error)

	// Storage related methods.

	// CreateDisks will attempt to create the disks described by <disks> spec and
	// return a slice of Disk representing the created disks or error if one of them failed.
	CreateDisks(zone string, disks []google.DiskSpec) ([]*google.Disk, error)
	// Disks will return a list of all Disks found in the project.
	Disks() ([]*google.Disk, error)
	// Disk will return a Disk representing the disk identified by the
	// passed <name> or error.
	Disk(zone, id string) (*google.Disk, error)
	// RemoveDisk will destroy the disk identified by <name> in <zone>.
	RemoveDisk(zone, id string) error
	// SetDiskLabels sets the labels on a disk, ensuring that the disk's
	// label fingerprint matches the one supplied.
	SetDiskLabels(zone, id, labelFingerprint string, labels map[string]string) error
	// AttachDisk will attach the volume identified by <volumeName> into the instance
	// <instanceId> and return an AttachedDisk representing it or error.
	AttachDisk(zone, volumeName, instanceId string, mode google.DiskMode) (*google.AttachedDisk, error)
	// DetachDisk will detach <volumeName> disk from <instanceId> if possible
	// and return error.
	DetachDisk(zone, instanceId, volumeName string) error
	// InstanceDisks returns a list of the disks attached to the passed instance.
	InstanceDisks(zone, instanceId string) ([]*google.AttachedDisk, error)
	// ListMachineTypes returns a list of machines available in the project and zone provided.
	ListMachineTypes(zone string) ([]google.MachineType, error)
}

type environ struct {
	common.CredentialInvalidator
	environs.NoSpaceDiscoveryEnviron
	environs.NoContainerAddressesEnviron
	environs.NoLXDProfiler

	name  string
	uuid  string
	cloud environscloudspec.CloudSpec
	gce   gceConnection

	lock sync.Mutex // lock protects access to ecfg
	ecfg *environConfig

	instTypeListLock    sync.Mutex
	instCacheExpireAt   time.Time
	cachedInstanceTypes []instances.InstanceType

	// namespace is used to create the machine and device hostnames.
	namespace instance.Namespace
}

var _ environs.Environ = (*environ)(nil)
var _ environs.NetworkingEnviron = (*environ)(nil)

// Function entry points defined as variables so they can be overridden
// for testing purposes.
var (
	newConnection = func(ctx context.Context, conn google.ConnectionConfig, creds *google.Credentials) (gceConnection, error) {
		return google.Connect(ctx, conn, creds)
	}
	destroyEnv = common.Destroy
	bootstrap  = common.Bootstrap
)

func newEnviron(ctx context.Context, cloud environscloudspec.CloudSpec, cfg *config.Config, invalidator environs.CredentialInvalidator) (*environ, error) {
	ecfg, err := newConfig(ctx, cfg, nil)
	if err != nil {
		return nil, errors.Annotate(err, "invalid config")
	}

	namespace, err := instance.NewNamespace(cfg.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	e := &environ{
		CredentialInvalidator: common.NewCredentialInvalidator(invalidator, google.IsAuthorisationFailure),
		name:                  ecfg.config.Name(),
		uuid:                  ecfg.config.UUID(),
		ecfg:                  ecfg,
		namespace:             namespace,
	}
	if err = e.SetCloudSpec(ctx, cloud); err != nil {
		return nil, err
	}
	return e, nil
}

// SetCloudSpec is specified in the environs.Environ interface.
func (e *environ) SetCloudSpec(_ context.Context, spec environscloudspec.CloudSpec) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	e.cloud = spec
	credAttrs := spec.Credential.Attributes()
	if spec.Credential.AuthType() == jujucloud.JSONFileAuthType {
		contents := credAttrs[credAttrFile]
		credential, err := parseJSONAuthFile(strings.NewReader(contents))
		if err != nil {
			return errors.Trace(err)
		}
		credAttrs = credential.Attributes()
	}

	credential := &google.Credentials{
		ClientID:    credAttrs[credAttrClientID],
		ProjectID:   credAttrs[credAttrProjectID],
		ClientEmail: credAttrs[credAttrClientEmail],
		PrivateKey:  []byte(credAttrs[credAttrPrivateKey]),
	}

	connectionConfig := google.ConnectionConfig{
		Region:    spec.Region,
		ProjectID: credential.ProjectID,

		// TODO (Stickupkid): Pass the http.Client through on the construction
		// of the environ.
		HTTPClient: jujuhttp.NewClient(
			jujuhttp.WithSkipHostnameVerification(spec.SkipTLSVerify),
			jujuhttp.WithLogger(logger.Child("http", corelogger.HTTP)),
		),
	}

	// TODO (stickupkid): Pass the context through the method call.
	ctx := context.Background()

	// Connect and authenticate.
	var err error
	e.gce, err = newConnection(ctx, connectionConfig, credential)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Name returns the name of the environment.
func (env *environ) Name() string {
	return env.name
}

// Provider returns the environment provider that created this env.
func (*environ) Provider() environs.EnvironProvider {
	return providerInstance
}

// Region returns the CloudSpec to use for the provider, as configured.
func (env *environ) Region() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   env.cloud.Region,
		Endpoint: env.cloud.Endpoint,
	}, nil
}

// SetConfig updates the env's configuration.
func (env *environ) SetConfig(ctx context.Context, cfg *config.Config) error {
	env.lock.Lock()
	defer env.lock.Unlock()

	ecfg, err := newConfig(ctx, cfg, env.ecfg.config)
	if err != nil {
		return errors.Annotate(err, "invalid config change")
	}
	env.ecfg = ecfg
	return nil
}

// Config returns the configuration data with which the env was created.
func (env *environ) Config() *config.Config {
	env.lock.Lock()
	defer env.lock.Unlock()
	return env.ecfg.config
}

// PrepareForBootstrap implements environs.Environ.
func (env *environ) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	if ctx.ShouldVerifyCredentials() {
		if err := env.gce.VerifyCredentials(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// Bootstrap creates a new instance, choosing the series and arch out of
// available tools. The series and arch are returned along with a func
// that must be called to finalize the bootstrap process by transferring
// the tools and installing the initial juju controller.
func (env *environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	// Ensure the API server port is open (globally for all instances
	// on the network, not just for the specific node of the state
	// server). See LP bug #1436191 for details.
	rules := firewall.IngressRules{
		firewall.NewIngressRule(
			network.PortRange{
				FromPort: params.ControllerConfig.APIPort(),
				ToPort:   params.ControllerConfig.APIPort(),
				Protocol: "tcp",
			},
		),
	}
	if params.ControllerConfig.AutocertDNSName() != "" {
		// Open port 80 as well as it handles Let's Encrypt HTTP challenge.
		rules = append(rules, firewall.NewIngressRule(network.MustParsePortRange("80/tcp")))
	}

	if err := env.gce.OpenPorts(env.globalFirewallName(), rules); err != nil {
		return nil, env.HandleCredentialError(ctx, err)
	}
	return bootstrap(ctx, env, params)
}

// Destroy shuts down all known machines and destroys the rest of the
// known environment.
func (env *environ) Destroy(ctx context.Context) error {
	err := destroyEnv(env, ctx)
	if err != nil {
		return errors.Trace(err)
	}
	err = env.cleanupFirewall(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// DestroyController implements the Environ interface.
func (env *environ) DestroyController(ctx context.Context, controllerUUID string) error {
	// TODO(wallyworld): destroy hosted model resources
	return env.Destroy(ctx)
}
