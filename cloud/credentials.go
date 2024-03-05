// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/utils/v4"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/yaml.v2"
)

// CloudCredential contains attributes used to define credentials for a cloud.
type CloudCredential struct {
	// DefaultCredential is the named credential to use by default.
	DefaultCredential string `yaml:"default-credential,omitempty"`

	// DefaultRegion is the cloud region to use by default.
	DefaultRegion string `yaml:"default-region,omitempty"`

	// AuthCredentials is the credentials for a cloud, keyed on name.
	AuthCredentials map[string]Credential `yaml:",omitempty,inline"`
}

func (c *CloudCredential) validateDefaultCredential() {
	if c.DefaultCredential != "" {
		stillHaveDefault := false
		for name := range c.AuthCredentials {
			if name == c.DefaultCredential {
				stillHaveDefault = true
				break
			}
		}
		if !stillHaveDefault {
			c.DefaultCredential = ""
		}
	}
}

// Credential instances represent cloud credentials.
type Credential struct {
	authType   AuthType
	attributes map[string]string

	// Revoked is true if the credential has been revoked.
	Revoked bool

	// Label is optionally set to describe the credentials to a user.
	Label string

	// Invalid is true if the credential is invalid.
	Invalid bool

	// InvalidReason contains the reason why a credential was flagged as invalid.
	// It is expected that this string will be empty when a credential is valid.
	InvalidReason string
}

// AuthType returns the authentication type.
func (c Credential) AuthType() AuthType {
	return c.authType
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string)
	for k, v := range in {
		out[k] = v
	}
	return out
}

// Attributes returns the credential attributes.
func (c Credential) Attributes() map[string]string {
	return copyStringMap(c.attributes)
}

type credentialInternal struct {
	AuthType   AuthType          `yaml:"auth-type" json:"auth-type"`
	Attributes map[string]string `yaml:",omitempty,inline" json:",omitempty,inline"`
}

// MarshalYAML implements the yaml.Marshaler interface.
func (c Credential) MarshalYAML() (interface{}, error) {
	return credentialInternal{c.authType, c.attributes}, nil
}

// UnmarshalYAML implements the yaml.Marshaler interface.
func (c *Credential) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var internal credentialInternal
	if err := unmarshal(&internal); err != nil {
		return err
	}
	*c = Credential{authType: internal.AuthType, attributes: internal.Attributes}
	return nil
}

// MarshalJSON implements the json.Marshaler interface.
func (c Credential) MarshalJSON() ([]byte, error) {
	return json.Marshal(credentialInternal{c.authType, c.attributes})
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (c *Credential) UnmarshalJSON(b []byte) error {
	var internal credentialInternal
	if err := json.Unmarshal(b, &internal); err != nil {
		return err
	}
	*c = Credential{authType: internal.AuthType, attributes: internal.Attributes}
	return nil
}

// NewCredential returns a new, immutable, Credential with the supplied
// auth-type and attributes.
func NewCredential(authType AuthType, attributes map[string]string) Credential {
	return Credential{authType: authType, attributes: copyStringMap(attributes)}
}

// NewNamedCredential returns an immutable Credential with the supplied properties.
func NewNamedCredential(name string, authType AuthType, attributes map[string]string, revoked bool) Credential {
	return Credential{
		Label:      name,
		authType:   authType,
		attributes: copyStringMap(attributes),
		Revoked:    revoked,
	}
}

// NewEmptyCredential returns a new Credential with the EmptyAuthType
// auth-type.
func NewEmptyCredential() Credential {
	return Credential{authType: EmptyAuthType, attributes: nil}
}

// NewEmptyCloudCredential returns a new CloudCredential with an empty
// default credential.
func NewEmptyCloudCredential() *CloudCredential {
	return &CloudCredential{AuthCredentials: map[string]Credential{"default": NewEmptyCredential()}}
}

// NamedCredentialAttr describes the properties of a named credential attribute.
type NamedCredentialAttr struct {
	// Name is the name of the credential value.
	Name string

	// CredentialAttr holds the properties of the credential value.
	CredentialAttr
}

// CredentialSchema describes the schema of a credential. Credential schemas
// are specific to cloud providers.
type CredentialSchema []NamedCredentialAttr

// Attribute returns the named CredentialAttr value.
func (s CredentialSchema) Attribute(name string) (*CredentialAttr, bool) {
	for _, value := range s {
		if value.Name == name {
			result := value.CredentialAttr
			return &result, true
		}
	}
	return nil, false
}

// FinalizeCredential finalizes a credential by matching it with one of the
// provided credential schemas, and reading any file attributes into their
// corresponding non-file attributes. This will also validate the credential.
//
// If there is no schema with the matching auth-type, an error satisfying
// errors.IsNotSupported will be returned.
func FinalizeCredential(
	credential Credential,
	schemas map[AuthType]CredentialSchema,
	readFile func(string) ([]byte, error),
) (*Credential, error) {
	schema, ok := schemas[credential.authType]
	if !ok {
		return nil, errors.NotSupportedf("auth-type %q", credential.authType)
	}
	attrs, err := schema.Finalize(credential.attributes, readFile)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Credential{authType: credential.authType, attributes: attrs}, nil
}

// Finalize finalizes the given credential attributes against the credential
// schema. If the attributes are invalid, Finalize will return an error.
//
// An updated attribute map will be returned, having any file attributes
// deleted, and replaced by their non-file counterparts with the values set
// to the contents of the files.
func (s CredentialSchema) Finalize(
	attrs map[string]string,
	readFile func(string) ([]byte, error),
) (map[string]string, error) {
	checker, err := s.schemaChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}
	m := make(map[string]interface{})
	for k, v := range attrs {
		m[k] = v
	}
	result, err := checker.Coerce(m, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	resultMap := result.(map[string]interface{})
	newAttrs := make(map[string]string)

	// Construct the final credential attributes map, reading values from files as necessary.
	for _, field := range s {
		if field.FileAttr != "" {
			if err := s.processFileAttrValue(field, resultMap, newAttrs, readFile); err != nil {
				return nil, errors.Trace(err)
			}
			continue
		}
		name := field.Name
		if field.FilePath {
			pathValue, ok := resultMap[name]
			if ok && pathValue != "" {
				absPath, err := ValidateFileAttrValue(pathValue.(string))
				if err != nil {
					return nil, errors.Trace(err)
				}
				data, err := readFile(absPath)
				if err != nil {
					return nil, errors.Annotatef(err, "reading file for %q", name)
				}
				if len(data) == 0 {
					return nil, errors.NotValidf("empty file for %q", name)
				}
				newAttrs[name] = string(data)
				continue
			}
		}
		if val, ok := resultMap[name]; ok {
			newAttrs[name] = val.(string)
		}
	}
	return newAttrs, nil
}

// ExpandFilePathsOfCredential iterates over the credential schema attributes
// and checks if the credential attribute has the ExpandFilePath flag set. If so
// the value of the credential attribute will be interrupted as a file with it's
// contents replaced with that of the file.
func ExpandFilePathsOfCredential(
	cred Credential,
	schemas map[AuthType]CredentialSchema,
) (Credential, error) {
	schema, exists := schemas[cred.AuthType()]
	if !exists {
		return cred, nil
	}

	attributes := cred.Attributes()
	for _, credAttr := range schema {
		if !credAttr.CredentialAttr.ExpandFilePath {
			continue
		}

		val, exists := attributes[credAttr.Name]
		if !exists || val == "" {
			continue
		}

		// NOTE: tlm dirty fix for lp1976620. This will be removed in Juju 3.0
		// when we stop overloading the keys in cloud credentials with different
		// values.
		if block, _ := pem.Decode([]byte(val)); block != nil {
			continue
		}

		abs, err := ValidateFileAttrValue(val)
		if err != nil {
			return cred, fmt.Errorf("determining file path value for credential attribute: %w", err)
		}

		contents, err := os.ReadFile(abs)
		if err != nil {
			return cred, fmt.Errorf("reading file %q contents for credential attribute %q: %w", abs, credAttr.Name, err)
		}

		attributes[credAttr.Name] = string(contents)
	}

	return NewNamedCredential(cred.Label, cred.AuthType(), attributes, cred.Revoked), nil
}

// ValidateFileAttrValue returns the normalised file path, so
// long as the specified path is valid and not a directory.
func ValidateFileAttrValue(path string) (string, error) {
	absPath, err := utils.ExpandPath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("invalid file path: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("file path %q must be a file", absPath)
	}
	return absPath, nil
}

func (s CredentialSchema) processFileAttrValue(
	field NamedCredentialAttr, resultMap map[string]interface{}, newAttrs map[string]string,
	readFile func(string) ([]byte, error),
) error {
	name := field.Name
	if fieldVal, ok := resultMap[name]; ok {
		if _, ok := resultMap[field.FileAttr]; ok {
			return errors.NotValidf(
				"specifying both %q and %q",
				name, field.FileAttr,
			)
		}
		newAttrs[name] = fieldVal.(string)
		return nil
	}
	fieldVal, ok := resultMap[field.FileAttr]
	if !ok {
		return errors.NewNotValid(nil, fmt.Sprintf(
			"either %q or %q must be specified",
			name, field.FileAttr,
		))
	}
	data, err := readFile(fieldVal.(string))
	if err != nil {
		return errors.Annotatef(err, "reading file for %q", name)
	}
	if len(data) == 0 {
		return errors.NotValidf("empty file for %q", name)
	}
	newAttrs[name] = string(data)
	return nil
}

func (s CredentialSchema) schemaChecker() (schema.Checker, error) {
	fields := make(environschema.Fields)
	for _, field := range s {
		fields[field.Name] = environschema.Attr{
			Description: field.Description,
			Type:        environschema.Tstring,
			Group:       environschema.AccountGroup,
			Mandatory:   field.FileAttr == "" && !field.Optional,
			Secret:      field.Hidden,
			Values:      field.Options,
		}
	}
	// TODO(axw) add support to environschema for attributes whose values
	// can be read in from a file.
	for _, field := range s {
		if field.FileAttr == "" {
			continue
		}
		if _, ok := fields[field.FileAttr]; ok {
			return nil, errors.Errorf("duplicate field %q", field.FileAttr)
		}
		fields[field.FileAttr] = environschema.Attr{
			Description: field.Description + " (file)",
			Type:        environschema.Tstring,
			Group:       environschema.AccountGroup,
			Mandatory:   false,
			Secret:      false,
		}
	}

	schemaFields, schemaDefaults, err := fields.ValidationSchema()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return schema.StrictFieldMap(schemaFields, schemaDefaults), nil
}

// CredentialAttr describes the properties of a credential attribute.
type CredentialAttr struct {
	// Description is a human-readable description of the credential
	// attribute.
	Description string

	// Hidden controls whether or not the attribute value will be hidden
	// when being entered interactively. Regardless of this, all credential
	// attributes are provided only to the Juju controllers.
	Hidden bool

	// FileAttr is the name of an attribute that may be specified instead
	// of this one, which points to a file that will be read in and its
	// value used for this attribute.
	FileAttr string

	// FilePath is true if the value of this attribute is a file path. If
	// this is true, then the attribute value will be set to the contents
	// of the file when the credential is "finalized".
	FilePath bool

	// ExpandFilePath reads in the FilePath, validating the file path correctly.
	// If the file path is correct, it will then read and replace the path,
	// with the associated content. The contents of the file in "finalized" will
	// be the file contents, not the filepath.
	ExpandFilePath bool

	// Optional controls whether the attribute is required to have a non-empty
	// value or not. Attributes default to mandatory.
	Optional bool

	// Options, if set, define the allowed values for this field.
	Options []interface{}
}

type cloudCredentialChecker struct{}

func (c cloudCredentialChecker) Coerce(v interface{}, path []string) (interface{}, error) {
	out := CloudCredential{
		AuthCredentials: make(map[string]Credential),
	}
	v, err := schema.StringMap(cloudCredentialValueChecker{}).Coerce(v, path)
	if err != nil {
		return nil, err
	}
	mapv := v.(map[string]interface{})
	for k, v := range mapv {
		switch k {
		case "default-region":
			out.DefaultRegion = v.(string)
		case "default-credential":
			out.DefaultCredential = v.(string)
		default:
			out.AuthCredentials[k] = v.(Credential)
		}
	}
	return out, nil
}

type cloudCredentialValueChecker struct{}

func (c cloudCredentialValueChecker) Coerce(v interface{}, path []string) (interface{}, error) {
	field := path[len(path)-1]
	switch field {
	case "default-region", "default-credential":
		return schema.String().Coerce(v, path)
	}
	v, err := schema.StringMap(schema.String()).Coerce(v, path)
	if err != nil {
		return nil, err
	}
	mapv := v.(map[string]interface{})

	authType, _ := mapv["auth-type"].(string)
	if authType == "" {
		return nil, errors.Errorf("%v: missing auth-type", strings.Join(path, ""))
	}

	attrs := make(map[string]string)
	delete(mapv, "auth-type")
	for k, v := range mapv {
		attrs[k] = v.(string)
	}
	if len(attrs) == 0 {
		attrs = nil
	}
	return Credential{authType: AuthType(authType), attributes: attrs}, nil
}

// ParseCredentials parses the given yaml bytes into Credentials, but does
// not validate the credential attributes.
func ParseCredentials(data []byte) (map[string]CloudCredential, error) {
	credentialCollection, err := ParseCredentialCollection(data)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudNames := credentialCollection.CloudNames()
	credentials := make(map[string]CloudCredential)
	for _, cloud := range cloudNames {
		v, err := credentialCollection.CloudCredential(cloud)
		if err != nil {
			return nil, errors.Trace(err)
		}
		credentials[cloud] = *v
	}
	return credentials, nil
}

// RemoveSecrets returns a copy of the given credential with secret fields removed.
func RemoveSecrets(
	credential Credential,
	schemas map[AuthType]CredentialSchema,
) (*Credential, error) {
	schema, ok := schemas[credential.authType]
	if !ok {
		return nil, errors.NotSupportedf("auth-type %q", credential.authType)
	}
	redactedAttrs := credential.Attributes()
	for _, attr := range schema {
		if attr.Hidden {
			delete(redactedAttrs, attr.Name)
		}
	}
	return &Credential{authType: credential.authType, attributes: redactedAttrs}, nil
}

// CredentialCollection holds CloudCredential(s) that are lazily validated.
type CredentialCollection struct {
	Credentials map[string]interface{} `yaml:"credentials"`
}

// ParseCredentialCollection parses YAML bytes for the credential
func ParseCredentialCollection(data []byte) (*CredentialCollection, error) {
	collection := CredentialCollection{}
	err := yaml.Unmarshal(data, &collection)
	if err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal yaml credentials")
	}
	return &collection, nil
}

// CloudCredential returns a copy of the CloudCredential for the specified cloud or
// an error when the CloudCredential was not found or failed to pass validation.
func (c *CredentialCollection) CloudCredential(cloudName string) (*CloudCredential, error) {
	credentialValue, ok := c.Credentials[cloudName]
	if !ok {
		return nil, errors.NotFoundf("credentials for cloud %s", cloudName)
	}
	if credential, ok := credentialValue.(CloudCredential); ok {
		return &credential, nil
	}
	credentialValue, err := cloudCredentialChecker{}.Coerce(
		credentialValue, []string{"credentials." + cloudName},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	credential := credentialValue.(CloudCredential)
	credential.validateDefaultCredential()
	c.Credentials[cloudName] = credential
	return &credential, nil
}

// CloudNames returns the cloud names to which credentials inside the CredentialCollection belong.
func (c *CredentialCollection) CloudNames() []string {
	var cloudNames []string
	for k := range c.Credentials {
		cloudNames = append(cloudNames, k)
	}
	return cloudNames
}

// UpdateCloudCredential stores a CloudCredential for a specific cloud.
func (c *CredentialCollection) UpdateCloudCredential(cloudName string, details CloudCredential) {
	if len(details.AuthCredentials) == 0 {
		delete(c.Credentials, cloudName)
		return
	}
	if c.Credentials == nil {
		c.Credentials = make(map[string]interface{})
	}
	details.validateDefaultCredential()
	c.Credentials[cloudName] = details
}
