package oracle

import (
	"github.com/juju/jsonschema"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

const (
	providerType = "oracle"
)

// provide friendly aliases for the register provider function
var providerAliases = []string{"ocl", "orcl", "oci"}

// environProvider type implements environs.EnvironProvider interface
// this will represent a computing and storage provider of the orcale cloud
// alongiside environs.EnvironProvider this implements config.Validator interface and
// environs.ProviderCredentials also.
type environProvider struct {
	//TODO
	// cli *oracleAPI // oracle client api to the REST API.
}

func (e environProvider) CloudSchema() *jsonschema.Schema {
	return nil
}

// Ping tests the connection to the oracle cloud to verify the endoint is valid.
func (e environProvider) Ping(endpoint string) error {
	return nil
}

// PrepareConfig prepares the configuration for the new model, based on
// the provided arguments. PrepareConfig is expected to produce a
// deterministic output
func (e environProvider) PrepareConfig(config environs.PrepareConfigParams) (*config.Config, error) {
	return nil, nil
}

// Open opens the oracle environment complaint with Juju and returns it. The configuration must have
// passed through PrepareConfig at some point in its lifecycle.
//
// This operation is not performing any expensive operation.
func (e environProvider) Open(params environs.OpenParams) (environs.Environ, error) {

	return nil, nil
}

// Validate method will validate model configuration
// This will ensure that the config passed is a valid configuration for the oracle cloud.
// If old is not nil, Validate should use it to determine whether a configuration change is valid.
func (e environProvider) Validate(cfg, old *config.Config) (valid *config.Config, _ error) {
	return nil, nil
}

// CredentialSchemas returns credential schemas, keyed on authentication type. This is used to validate existing oracle credentials, or to generate new ones.
func (e environProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return nil
}

// DetectCredentials automatically detects one or more oracle credentials from the environmnet. This may involve, for example inspecting environmnet variables, or reading configuration files in well-defined locations.
// If no credentials can be detected, the func will return an error satisfying errors.IsNotFound
func (e environProvider) DetectCredentials() (*cloud.CloudCredential, error) {
	return nil, nil
}

// FinalizeCredential finalizes a oracle credential, updating any attributes as
// necessarry. This is done clinet-side, when adding the credential to credentials.yaml
// and before uploading crdentials to the controller.
// The provider may completely alter a credential, even goiing as far as changing the auth-type, but the output must be a fully formed credential that is orcale complaint.
func (e environProvider) FinalizeCredential(
	cfx environs.FinalizeCredentialContext,
	params environs.FinalizeCredentialParams,
) (*cloud.Credential, error) {
	return nil, nil
}
