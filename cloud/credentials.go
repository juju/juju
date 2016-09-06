// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/utils"
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

// Credential instances represent cloud credentials.
type Credential struct {
	authType   AuthType
	attributes map[string]string

	// Revoked is true if the credential has been revoked.
	Revoked bool

	// Label is optionally set to describe the credentials to a user.
	Label string
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
	AuthType   AuthType          `yaml:"auth-type"`
	Attributes map[string]string `yaml:",omitempty,inline"`
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

// NewCredential returns a new, immutable, Credential with the supplied
// auth-type and attributes.
func NewCredential(authType AuthType, attributes map[string]string) Credential {
	return Credential{authType: authType, attributes: copyStringMap(attributes)}
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
// If there is no schema with the matching auth-type, and error satisfying
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

// ValidateFileAttrValue returns the normalised file path, so
// long as the specified path is valid and not a directory.
func ValidateFileAttrValue(path string) (string, error) {
	if !filepath.IsAbs(path) && !strings.HasPrefix(path, "~") {
		return "", errors.Errorf("file path must be an absolute path: %s", path)
	}
	absPath, err := utils.NormalizePath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return "", errors.Errorf("invalid file path: %s", absPath)
	}
	if info.IsDir() {
		return "", errors.Errorf("file path must be a file: %s", absPath)
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

	// FilePath is true is the value of this attribute is a file path. If
	// this is true, then the attribute value will be set to the contents
	// of the file when the credential is "finalized".
	FilePath bool

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
	var credentialsYAML struct {
		Credentials map[string]interface{} `yaml:"credentials"`
	}
	err := yaml.Unmarshal(data, &credentialsYAML)
	if err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal yaml credentials")
	}
	credentials := make(map[string]CloudCredential)
	for cloud, v := range credentialsYAML.Credentials {
		v, err := cloudCredentialChecker{}.Coerce(
			v, []string{"credentials." + cloud},
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
		credentials[cloud] = v.(CloudCredential)
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
