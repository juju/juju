package oracle

import (
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// oracleEnviron implements the environs.Environ interface
// and has behaviour specific to the
// description that the interface provides.
type oracleEnviron struct {
	p *environProvider
}

func newOracleEnviron() *oracleEnviron {
	return &oracleEnviron{}
}

func (o oracleEnviron) Validate(cfg, old *config.Config) (valid *config.Config, _ error) {
	return nil, nil
}

func (o oracleEnviron) StorageProviderTypes() ([]environs.ProviderType, error) {

}

func (o oracleEnviron) StorageProvider(t environs.ProviderType) (Provider, error) {

}

func (o oracleEnviron) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	return nil, nil
}

func (o oracleEnviron) StopInstances(...instance.Id) error {
	return nil
}

func (o oracleEnviron) AllInstances() ([]instance.Instance, error) {
	return nil
}

func (o oracleEnviron) MaintainInstance(args StartInstanceParams) error {
	return nil
}

func (o oracleEnviron) Config() *config.Config {
	return nil
}

func (o oracleEnviron) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) string {
	return ""
}

func (o oracleEnviron) BootstrapMessage() string {
	return ""
}

func (o oracleEnviron) Create(params environs.CreateParams) error {
	return nil
}

func (o oracleEnviron) ConstraintsValidator() (constraints.Validator, error) {
	return nil, nil
}

func (o oracleEnviron) SetConfig(cfg *config.Config) error {
	return nil
}

func (o oracleEnviron) Instances(ids []instance.Id) ([]instance.Instance, error) {
	return nil
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

func (o oracleEnviron) Provider() EnvironProvider {
	return o.p
}

func (o oracleEnviron) PrecheckInstance(series string, cons constrants.Value, placement string) error {
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
	return nil, nil
}
