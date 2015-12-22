// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package cloud provides functionality to parse information
// describing clouds, including regions, supported auth types etc.

package cloud

import (
	"io/ioutil"
	"os"
	"reflect"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/juju/osenv"
)

//go:generate go run ../generate/filetoconst.go fallbackPublicCloudInfo fallback-public-cloud.yaml fallback_public_cloud.go 2015

// AuthType is the type of authentication used by the cloud.
type AuthType string

const (
	// AccessKeyAuthType is an authentication type using a key and secret.
	AccessKeyAuthType = AuthType("access-key")

	// UserPassAuthType is an authentication type using a username and password.
	UserPassAuthType = AuthType("userpass")

	// OAuthAuthType is an authentication type using oauth2.
	OAuthAuthType = AuthType("oauth2")
)

// Clouds is a struct containing cloud definitions.
type Clouds struct {
	// Clouds is a map of cloud definitions, keyed on cloud name.
	Clouds map[string]Cloud `yaml:"clouds"`
}

// Cloud is a cloud definition.
type Cloud struct {
	// Type is the type of cloud, eg aws, openstack etc.
	Type string `yaml:"type"`

	// AuthTypes are the authentication modes supported by the cloud.
	AuthTypes []AuthType `yaml:"auth-types,omitempty,flow"`

	// Endpoint is the default endpoint for the cloud regions, may be
	// overridden by a region.
	Endpoint string `yaml:"endpoint,omitempty"`

	// Regions are the regions available in the cloud.
	Regions map[string]Region `yaml:"regions,omitempty"`
}

// Region is a cloud region.
type Region struct {
	// Endpoint is the region's endpoint URL.
	Endpoint string `yaml:"endpoint,omitempty"`
}

// JujuPublicClouds is the location where public cloud information is
// expected to be found. Requires JUJU_HOME to be set.
func JujuPublicClouds() string {
	return osenv.JujuHomePath("public-clouds.yaml")
}

// PublicCloudMetadata looks in searchPath for cloud metadata files and if none
// are found, returns the fallback public cloud metadata.
func PublicCloudMetadata(searchPath ...string) (clouds *Clouds, fallbackUsed bool, err error) {
	for _, file := range searchPath {
		data, err := ioutil.ReadFile(file)
		if err != nil && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, false, errors.Trace(err)
		}
		clouds, err = ParseCloudMetadata(data)
		if err != nil {
			return nil, false, errors.Trace(err)
		}
		return clouds, false, err
	}
	clouds, err = ParseCloudMetadata([]byte(fallbackPublicCloudInfo))
	return clouds, true, err
}

// ParseCloudMetadata parses the given yaml bytes into Clouds metadata.
func ParseCloudMetadata(data []byte) (*Clouds, error) {
	var metadata Clouds
	err := yaml.Unmarshal(data, &metadata)
	if err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal yaml cloud metadata")
	}
	metadata.denormaliseMetadata()
	return &metadata, nil
}

// To keep the metadata concise, attributes on the metadata struct which have the same value for each
// item may be moved up to a higher level in the tree. denormaliseMetadata descends the tree
// and fills in any missing attributes with values from a higher level.
func (metadata *Clouds) denormaliseMetadata() {
	for _, cloud := range metadata.Clouds {
		for name, region := range cloud.Regions {
			r := region
			inherit(&r, &cloud)
			cloud.Regions[name] = r
		}
	}
}

type structTags map[reflect.Type]map[string]int

var tagsForType structTags = make(structTags)

// RegisterStructTags ensures the yaml tags for the given structs are able to be used
// when parsing cloud metadata.
func RegisterStructTags(vals ...interface{}) {
	tags := mkTags(vals...)
	for k, v := range tags {
		tagsForType[k] = v
	}
}

func init() {
	RegisterStructTags(Clouds{}, Cloud{}, Region{})
}

func mkTags(vals ...interface{}) map[reflect.Type]map[string]int {
	typeMap := make(map[reflect.Type]map[string]int)
	for _, v := range vals {
		t := reflect.TypeOf(v)
		typeMap[t] = yamlTags(t)
	}
	return typeMap
}

// yamlTags returns a map from yaml tag to the field index for the string fields in the given type.
func yamlTags(t reflect.Type) map[string]int {
	if t.Kind() != reflect.Struct {
		panic(errors.Errorf("cannot get yaml tags on type %s", t))
	}
	tags := make(map[string]int)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Type != reflect.TypeOf("") {
			continue
		}
		if tag := f.Tag.Get("yaml"); tag != "" {
			if i := strings.Index(tag, ","); i >= 0 {
				tag = tag[0:i]
			}
			if tag == "-" {
				continue
			}
			if tag != "" {
				f.Name = tag
			}
		}
		tags[f.Name] = i
	}
	return tags
}

// inherit sets any blank fields in dst to their equivalent values in fields in src that have matching json tags.
// The dst parameter must be a pointer to a struct.
func inherit(dst, src interface{}) {
	for tag := range tags(dst) {
		setFieldByTag(dst, tag, fieldByTag(src, tag), false)
	}
}

// tags returns the field offsets for the JSON tags defined by the given value, which must be
// a struct or a pointer to a struct.
func tags(x interface{}) map[string]int {
	t := reflect.TypeOf(x)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		panic(errors.Errorf("expected struct, not %s", t))
	}

	if tagm := tagsForType[t]; tagm != nil {
		return tagm
	}
	panic(errors.Errorf("%s not found in type table", t))
}

// fieldByTag returns the value for the field in x with the given JSON tag, or "" if there is no such field.
func fieldByTag(x interface{}, tag string) string {
	tagm := tags(x)
	v := reflect.ValueOf(x)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if i, ok := tagm[tag]; ok {
		return v.Field(i).Interface().(string)
	}
	return ""
}

// setFieldByTag sets the value for the field in x with the given JSON tag to val.
// The override parameter specifies whether the value will be set even if the original value is non-empty.
func setFieldByTag(x interface{}, tag, val string, override bool) {
	i, ok := tags(x)[tag]
	if !ok {
		return
	}
	v := reflect.ValueOf(x).Elem()
	f := v.Field(i)
	if override || f.Interface().(string) == "" {
		f.Set(reflect.ValueOf(val))
	}
}
