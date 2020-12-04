package packet

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	envinstance "github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/storage"
	"github.com/juju/schema"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	"github.com/packethost/packngo"
	"gopkg.in/juju/environschema.v1"
)

type environConfig struct {
	config *config.Config
	attrs  map[string]interface{}
}

type environ struct {
	ecfgMutex    sync.Mutex
	ecfg         *environConfig
	name         string
	cloud        environscloudspec.CloudSpec
	packetClient *packngo.Client
}

var providerInstance environProvider

func (e *environ) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	return nil
}

func (e *environ) Bootstrap(ctx environs.BootstrapContext, callCtx context.ProviderCallContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return common.Bootstrap(ctx, e, callCtx, args)
}

func (e *environ) AllInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return e.getPacketInstancesByTag("juju-instance", "")
}

// if values tag and state are left empty it will return all instances
func (e *environ) getPacketInstancesByTag(tag, state string) ([]instances.Instance, error) {
	toReturn := []instances.Instance{}
	opt := &packngo.ListOptions{Search: tag}

	devices, _, err := e.packetClient.Devices.List(e.cloud.Credential.Attributes()["project-id"], opt)
	if err != nil {
		return nil, err
	}

	for _, d := range devices {
		if state != "" && d.State == state {
			toReturn = append(toReturn, &packetDevice{e, &d})
		} else if state == "" {
			toReturn = append(toReturn, &packetDevice{e, &d})
		}
	}

	return toReturn, nil
}

func (e *environ) AllRunningInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return e.getPacketInstancesByTag("juju-instance", "active")
}

func (e *environ) Config() *config.Config {
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	return e.ecfg.config
}

func (e *environ) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported([]string{constraints.CpuPower, constraints.VirtType})
	validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
	validator.RegisterVocabulary(constraints.Arch, []string{arch.AMD64, arch.ARM64, arch.I386, arch.PPC64EL})
	return validator, nil
}

func (e *environ) ControllerInstances(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	return nil, nil
}

func (e *environ) Create(ctx context.ProviderCallContext, args environs.CreateParams) error {
	return nil
}

func (e *environ) Destroy(ctx context.ProviderCallContext) error {
	return common.Destroy(e, ctx)
}

func (e *environ) DestroyController(ctx context.ProviderCallContext, controllerUUID string) error {
	_, err := e.packetClient.Devices.Delete(controllerUUID, true)
	if err != nil {
		return err
	}

	return nil
}

func (e *environ) InstanceTypes(context.ProviderCallContext, constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	var i envinstance.InstanceTypesWithCostMetadata
	return i, nil
}

func (e *environ) Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error) {
	toReturn := []instances.Instance{}
	for _, id := range ids {
		//TODO handle case when some of the instanes are missing
		d, _, err := e.packetClient.Devices.Get(string(id), nil)
		if err != nil {
			return nil, err
		}
		toReturn = append(toReturn, &packetDevice{e, d})

	}
	if len(toReturn) == 0 {
		return nil, environs.ErrNoInstances
	}
	return toReturn, nil
}

func (e *environ) MaintainInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) error {
	return nil
}

func (e *environ) PrecheckInstance(ctx context.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	return nil
}

func (e *environ) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	e.name = controllerName
	return nil
}

func (*environ) Provider() environs.EnvironProvider {
	return &environProvider{}
}

func (e *environ) SetConfig(cfg *config.Config) error {
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return errors.Annotate(err, "invalid config change")
	}
	e.ecfg = ecfg
	return nil
}

var configImmutableFields = []string{}
var configFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()
var configSchema = environschema.Fields{}
var configDefaults = schema.Defaults{}

func newConfig(cfg, old *config.Config) (*environConfig, error) {
	// Ensure that the provided config is valid.
	if err := config.Validate(cfg, old); err != nil {
		return nil, errors.Trace(err)
	}
	attrs, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if old != nil {
		// There's an old configuration. Validate it so that any
		// default values are correctly coerced for when we check
		// the old values later.
		oldEcfg, err := newConfig(old, nil)
		if err != nil {
			return nil, errors.Annotatef(err, "invalid base config")
		}
		for _, attr := range configImmutableFields {
			oldv, newv := oldEcfg.attrs[attr], attrs[attr]
			if oldv != newv {
				return nil, errors.Errorf(
					"%s: cannot change from %v to %v",
					attr, oldv, newv,
				)
			}
		}
	}

	ecfg := &environConfig{
		config: cfg,
		attrs:  attrs,
	}
	return ecfg, nil
}

func (e *environ) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (result *environs.StartInstanceResult, resultErr error) {
	plan := "t1.small.x86"
	OS := "ubuntu_18_04"
	k, _, keyErr := e.packetClient.SSHKeys.Create(&packngo.SSHKeyCreateRequest{
		ProjectID: e.cloud.Credential.Attributes()["project-id"],
		Key:       e.ecfg.config.AuthorizedKeys(),
		Label:     "juju",
	})

	userdata := fmt.Sprintf("#!/bin/bash\nrm /etc/ssh/ssh_host_*dsa* \nrm /etc/ssh/ssh_host_ed*\nrm /sbin/initctl\nsudo apt update\nsudo apt install -y dmidecode\nset -e\n(grep ubuntu /etc/group) || groupadd ubuntu\n(id ubuntu &> /dev/null) || useradd -m ubuntu -s /bin/bash -g ubuntu\numask 0077\ntemp=$(mktemp)\necho 'ubuntu ALL=(ALL) NOPASSWD:ALL' > $temp\ninstall -m 0440 $temp /etc/sudoers.d/90-juju-ubuntu\nrm $temp\nsu ubuntu -c 'install -D -m 0600 /dev/null ~/.ssh/authorized_keys'\nexport authorized_keys=\"%s\"\nif [ ! -z \"$authorized_keys\" ]; then\nsu ubuntu -c 'printf \"%%s\n\" \"$authorized_keys\" >> ~/.ssh/authorized_keys'\nfi", e.ecfg.config.AuthorizedKeys())
	// logger.Errorf("*****environ %s", spew.Sdump(e))

	juserdata, err := providerinit.ComposeUserData(args.InstanceConfig, nil, PacketRenderer{})

	userdata = userdata + "\n" + strings.Replace(string(juserdata), "#!/bin/bash", "", 0)
	// fmt.Println("******", string(userdata))

	device := &packngo.DeviceCreateRequest{
		Hostname:     e.name,
		Facility:     []string{e.cloud.Region},
		Plan:         plan,
		OS:           OS,
		ProjectID:    e.cloud.Credential.Attributes()["project-id"],
		BillingCycle: "hourly",
		UserData:     userdata,
		Tags:         []string{fmt.Sprintf("juju-controller-%s", e.cloud.Name), "juju-instance"},
	}

	if keyErr == nil {
		device.ProjectSSHKeys = []string{k.ID}
	}
	// logger.Errorf("*****device %s", spew.Sdump(device))

	d, _, err := e.packetClient.Devices.Create(device)
	if err != nil {
		return nil, err
	}

	d, err = waitDeviceActive(e.packetClient, d.ID)

	inst := &packetDevice{e, d}
	amd64 := arch.AMD64
	mem, _ := strconv.ParseUint(inst.Plan.Specs.Memory.Total, 10, 64)
	cpus := uint64(inst.Plan.Specs.Cpus[0].Count)
	hc := &instance.HardwareCharacteristics{
		Arch: &amd64,
		Mem:  &mem,
		// RootDisk: &instanceSpec.InstanceType.RootDisk,
		CpuCores: &cpus,
	}

	return &environs.StartInstanceResult{
		Instance: inst,
		Hardware: hc,
	}, nil
}

func (e *environ) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
	return nil
}

func (e *environ) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	return nil, errors.NotFoundf("storage provider %q", t)
}

func (e *environ) StorageProviderTypes() ([]storage.ProviderType, error) {
	return nil, nil
}

func waitDeviceActive(c *packngo.Client, id string) (*packngo.Device, error) {
	// 15 minutes = 180 * 5sec-retry
	for i := 0; i < 180; i++ {
		<-time.After(5 * time.Second)
		d, _, err := c.Devices.Get(id, nil)
		if err != nil {
			return nil, err
		}
		if d.State == "active" {
			return d, nil
		}
		if d.State == "failed" {
			return nil, fmt.Errorf("device %s provisioning failed", id)
		}
	}

	return nil, fmt.Errorf("device %s is still not active after timeout", id)
}
