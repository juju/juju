// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/names.v2"
)

// CloudCredential represents the current cloud credential for the model.
type CloudCredential interface {
	Owner() string
	Cloud() string
	Name() string
	AuthType() string
	Attributes() map[string]string
}

// CloudCredentialArgs is an argument struct used to create a new internal
// cloudCredential type that supports the CloudCredential interface.
type CloudCredentialArgs struct {
	Owner      names.UserTag
	Cloud      names.CloudTag
	Name       string
	AuthType   string
	Attributes map[string]string
}

func newCloudCredential(args CloudCredentialArgs) *cloudCredential {
	return &cloudCredential{
		Version:     1,
		Owner_:      args.Owner.Id(),
		Cloud_:      args.Cloud.Id(),
		Name_:       args.Name,
		AuthType_:   args.AuthType,
		Attributes_: args.Attributes,
	}
}

// cloudCredential represents an IP CloudCredential of some form.
type cloudCredential struct {
	Version int `yaml:"version"`

	Owner_      string            `yaml:"owner"`
	Cloud_      string            `yaml:"cloud"`
	Name_       string            `yaml:"name"`
	AuthType_   string            `yaml:"auth-type"`
	Attributes_ map[string]string `yaml:"attributes,omitempty"`
}

// Owner implements CloudCredential.
func (c *cloudCredential) Owner() string {
	return c.Owner_
}

// Cloud implements CloudCredential.
func (c *cloudCredential) Cloud() string {
	return c.Cloud_
}

// Name implements CloudCredential.
func (c *cloudCredential) Name() string {
	return c.Name_
}

// AuthType implements CloudCredential.
func (c *cloudCredential) AuthType() string {
	return c.AuthType_
}

// Attributes implements CloudCredential.
func (c *cloudCredential) Attributes() map[string]string {
	return c.Attributes_
}

// importCloudCredential constructs a new CloudCredential from a map
// representing a serialised CloudCredential instance.
func importCloudCredential(source map[string]interface{}) (*cloudCredential, error) {
	version, err := getVersion(source)
	if err != nil {
		return nil, errors.Annotate(err, "cloudCredential version schema check failed")
	}

	importFunc, ok := cloudCredentialDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}

	return importFunc(source)
}

type cloudCredentialDeserializationFunc func(map[string]interface{}) (*cloudCredential, error)

var cloudCredentialDeserializationFuncs = map[int]cloudCredentialDeserializationFunc{
	1: importCloudCredentialV1,
}

func importCloudCredentialV1(source map[string]interface{}) (*cloudCredential, error) {
	fields := schema.Fields{
		"owner":      schema.String(),
		"cloud":      schema.String(),
		"name":       schema.String(),
		"auth-type":  schema.String(),
		"attributes": schema.StringMap(schema.String()),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"attributes": schema.Omit,
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "cloudCredential v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	creds := &cloudCredential{
		Version:   1,
		Owner_:    valid["owner"].(string),
		Cloud_:    valid["cloud"].(string),
		Name_:     valid["name"].(string),
		AuthType_: valid["auth-type"].(string),
	}
	if attributes, found := valid["attributes"]; found {
		creds.Attributes_ = convertToStringMap(attributes)
	}
	return creds, nil
}
