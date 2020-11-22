package packet

// import (
// 	"github.com/juju/juju/cloudconfig/cloudinit"
// 	"github.com/juju/juju/environs"
// 	"github.com/juju/schema"
// 	"gopkg.in/goose.v2/nova"
// )

// type configurator struct {
// }

// func (c *configurator) GetCloudConfig(args environs.StartInstanceParams) (cloudinit.CloudConfig, error) {
// 	trace()
// 	return nil, nil
// }

// func (c *configurator) GetConfigDefaults() schema.Defaults {
// 	trace()
// 	return schema.Defaults{
// 		"use-floating-ip":      false,
// 		"use-default-secgroup": false,
// 		"network":              "",
// 		"external-network":     "",
// 		"use-openstack-gbp":    false,
// 		"policy-target-group":  "",
// 	}
// }

// func (c *configurator) ModifyRunServerOptions(_ *nova.RunServerOpts) {
// 	trace()
// }
