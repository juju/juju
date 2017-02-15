package oracle

import (
	"fmt"
	"os"

	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	envinstance "github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/storage"

	oci "github.com/hoenirvili/go-oracle-cloud/api"
)

// oracleEnviron implements the environs.Environ interface
// and has behaviour specific that the interface provides.
type oracleEnviron struct {
	p    *environProvider
	spec environs.CloudSpec
	cfg  *config.Config
}

func newOracleEnviron(p *environProvider, args environs.OpenParams) *oracleEnviron {
	env := &oracleEnviron{
		p:    p,
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

// Bootstrap creates a new environment, and an instance inside the
// oracle cloud infrastracture to host the controller for that
// environment. The instnace will have have the series and architecture
// of the Environ's choice, constrained to those of the available tools.
// Bootstrap will return the instance's
// architecture, series, and a function that must be called to finalize
// the bootstrap process by transferring the tools and installing the
// initial Juju controller.
//
// Bootstrap will use just one specific architecture because the oracle
// cloud only supports amd64.
func (o oracleEnviron) Bootstrap(
	ctx environs.BootstrapContext,
	params environs.BootstrapParams,
) (*environs.BootstrapResult, error) {
	// in order too make the entire bootstrap process prossible
	// we must take into accounting some things:
	// validate if we have a shape based on the bootstrap constraints
	// and pick the right one
	// validate if we have in the imagelist a image correspoding to the
	// image tools specified.

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

	imagelist, err := checkImageList(o.p.client, params.ImageMetadata)
	if err != nil {
		return nil, errors.Trace(err)
	}

	os.Exit(1)

	//TODO
	instance, err := launchBootstrapConstroller(o.p.client, []oci.InstanceParams{
		{
			Shape:     shape.name,
			Imagelist: imagelist,
			Label:     "",
			SSHKeys:   nil,
			Name:      "Bootstrap Juju Controller",
		},
	})
	if err != nil {
		return nil, err
	}

	_ = instance
	return nil, nil
}

// here we should check with the client if the we have already a ubuntu/centos/what ever image is specified int he tools metadata
// if there already exists then we should return it
func checkImageList(c *oci.Client, tools []*imagemetadata.ImageMetadata) (string, error) {
	var imageVersion string

	if c == nil {
		return "", errors.NotFoundf("Cannot use nil client")
	}

	if tools == nil {
		return "", errors.NotFoundf("No tools imagemedatada provided")
	}

	for _, val := range tools {
		if len(val.Version) > 0 {
			imageVersion = val.Version
			logger.Infof("Found tools %s and searching tghourgh the oracle imagelist", val)
			break
		}
	}

	if imageVersion == "" {
		return "", errors.NotFoundf("No version found in the tools")
	}

	resp, err := c.AllImageList()
	if err != nil {
		return "", errors.Trace(err)
	}

	for _, val := range resp.Result {
		//TODO
	}
	fmt.Printf("%+v", resp)
	return imageVersion, nil
}

// launchBootstrapController creates a new vm inside
// the oracle infrastracuture and parses the response into a instance.Instance
func launchBootstrapConstroller(c *oci.Client, params []oci.InstanceParams) (instance.Instance, error) {
	if c == nil {
		return nil, errors.NotFoundf("Cannot use nil client")
	}

	resp, err := c.CreateInstance(params)
	if err != nil {
		return nil, errors.Trace(err)
	}

	instance, err := newInstance(resp)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return instance, nil
}

func (o oracleEnviron) BootstrapMessage() string {
	return "SomeBootstrapMessage"
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

func (o oracleEnviron) InstanceTypes(constraints.Value) (envinstance.InstanceTypesWithCostMetadata, error) {
	var i envinstance.InstanceTypesWithCostMetadata
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
