package oracle

import (
	"fmt"
	"os"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/storage"
)

// oracleEnviron implements the environs.Environ interface
// and has behaviour specific that the interface provides.
type oracleEnviron struct {
	mu *sync.Mutex

	p    *environProvider
	spec environs.CloudSpec
	cfg  *config.Config
}

func newOracleEnviron(p *environProvider, args environs.OpenParams) *oracleEnviron {
	env := &oracleEnviron{
		p:    p,
		mu:   &sync.Mutex{},
		spec: args.Cloud,
		cfg:  args.Config,
	}

	return env
}

func (o oracleEnviron) PrepareForBootstrap(ctx environs.BootstrapContext) error {
	return nil
}

func (o oracleEnviron) Validate(cfg, old *config.Config) (valid *config.Config, _ error) {
	return nil, nil
}

func (o oracleEnviron) StorageProviderTypes() ([]storage.ProviderType, error) {
	return nil, nil
}

func (o oracleEnviron) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	return nil, nil
}

func (o oracleEnviron) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	return nil, nil
}

func (o oracleEnviron) StopInstances(...instance.Id) error {
	return nil
}

func (o oracleEnviron) AllInstances() ([]instance.Instance, error) {
	return nil, nil
}

func (o oracleEnviron) MaintainInstance(args environs.StartInstanceParams) error {
	return nil
}

func (o oracleEnviron) Config() *config.Config {
	return o.cfg
}

func (o oracleEnviron) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	logger.Infof("Loging into the oracle cloud infrastructure")
	if err := o.p.client.Authenticate(); err != nil {
		return nil, errors.Trace(err)
	}

	shapes, err := o.p.client.AllShapeDetails()
	if err != nil {
		return nil, errors.Trace(err)
	}

	shape, err := findShape(shapes.Result, params.BootstrapConstraints)
	if err != nil {
		return nil, errors.Trace(err)
	}

	logger.Infof(
		"Choosing the %s with %d cores and %d MB ram",
		shape.name, shape.cpus, shape.ram,
	)

	os.Exit(1)

	fmt.Println("=============================")
	fmt.Printf("%+v\n", params.BootstrapConstraints)
	fmt.Println("=============================")
	return nil, nil
}

func (o oracleEnviron) BootstrapMessage() string {
	return ""
}

func (o oracleEnviron) Create(params environs.CreateParams) error {
	return nil
}

// ConstraintsValidator returns a constraints.Validator instance which
// is used to validate and merge constraints.
//
// Validator defines operations on constraints attributes which are
// used to ensure a constraints value is valid, as well as being able
// to handle overridden attributes.
//
// This will use the default validator implementation from the constraints package.
func (o oracleEnviron) ConstraintsValidator() (constraints.Validator, error) {
	// list of unsupported oracle provider constraints
	unsupportedConstraints := []string{
		constraints.Container,
		constraints.CpuPower,
		constraints.RootDisk,
		constraints.Arch,
		constraints.InstanceType,
		constraints.VirtType,
		constraints.Spaces,
	}

	// we choose to use the default validator implementation
	validator := constraints.NewValidator()

	// we must feed the validator that the oracle cloud
	// provider does not support these constraints
	validator.RegisterUnsupported(unsupportedConstraints)

	return newConstraintsAdaptor(validator), nil
}

func (o oracleEnviron) SetConfig(cfg *config.Config) error {
	return nil
}

func (o oracleEnviron) Instances(ids []instance.Id) ([]instance.Instance, error) {
	return nil, nil
}

func (o oracleEnviron) ControllerInstances(controllerUUID string) ([]instance.Id, error) {
	return nil, nil
}

func (o oracleEnviron) Destroy() error {
	return nil
}

func (o oracleEnviron) DestroyController(controllerUUID string) error {
	return nil
}

func (o oracleEnviron) Provider() environs.EnvironProvider {
	return o.p
}

func (e oracleEnviron) AdoptResources(controllerUUID string, fromVersion version.Number) error {

	return nil
}

func (o oracleEnviron) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	return nil
}

func (o oracleEnviron) OpenPorts(rules []network.IngressRule) error {
	return nil
}

func (o oracleEnviron) ClosePorts(rules []network.IngressRule) error {
	return nil
}

func (o oracleEnviron) IngressRules() ([]network.IngressRule, error) {
	return nil, nil
}

func (o oracleEnviron) InstanceTypes(constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	var i instances.InstanceTypesWithCostMetadata
	return i, nil
}

// Providing this methods oracleEnviron implements also the simplestreams.HasRegion
// interface
//
// Region returns the necessary attributes to uniquely identify this cloud instance.
func (o oracleEnviron) Region() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   o.spec.Region,
		Endpoint: o.spec.Endpoint,
	}, nil
}
