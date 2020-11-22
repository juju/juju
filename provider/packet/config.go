package packet

import (
	"github.com/juju/juju/environs/config"
)

// func (c *Config) Apply(attrs map[string]interface{}) (*Config, error) {
// 	defined := c.AllAttrs()
// 	for k, v := range attrs {
// 		defined[k] = v
// 	}
// 	return New(NoDefaults, defined)
// }

func validateConfig(cfg, old *config.Config) (*environConfig, error) {
	// Check for valid changes for the base config values.
	// if err := config.Validate(cfg, old); err != nil {
	// 	return nil, err
	// }
	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}
	ecfg := &environConfig{cfg, validated}

	// if vpcID := ecfg.vpcID(); isVPCIDSetButInvalid(vpcID) {
	// 	return nil, fmt.Errorf("vpc-id: %q is not a valid AWS VPC ID", vpcID)
	// } else if !isVPCIDSet(vpcID) && ecfg.forceVPCID() {
	// 	return nil, fmt.Errorf("cannot use vpc-id-force without specifying vpc-id as well")
	// }

	// if old != nil {
	// 	attrs := old.UnknownAttrs()

	// 	if vpcID, _ := attrs["vpc-id"].(string); vpcID != ecfg.vpcID() {
	// 		return nil, fmt.Errorf("cannot change vpc-id from %q to %q", vpcID, ecfg.vpcID())
	// 	}

	// 	if forceVPCID, _ := attrs["vpc-id-force"].(bool); forceVPCID != ecfg.forceVPCID() {
	// 		return nil, fmt.Errorf("cannot change vpc-id-force from %v to %v", forceVPCID, ecfg.forceVPCID())
	// 	}
	// }

	// // ssl-hostname-verification cannot be disabled
	// if !ecfg.SSLHostnameVerification() {
	// 	return nil, fmt.Errorf("disabling ssh-hostname-verification is not supported")
	// }
	return ecfg, nil
}
