// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package cloud provides functionality to parse information
// describing clouds, including regions, supported auth types etc.

package cloud

import (
	"fmt"
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
	AccessKeyAuthType AuthType = "access-key"

	// UserPassAuthType is an authentication type using a username and password.
	UserPassAuthType AuthType = "userpass"

	// OAuth1AuthType is an authentication type using oauth1.
	OAuth1AuthType AuthType = "oauth1"

	// OAuth2AuthType is an authentication type using oauth2.
	OAuth2AuthType AuthType = "oauth2"

	// JSONFileAuthType is an authentication type that takes a path to
	// a JSON file.
	JSONFileAuthType AuthType = "jsonfile"

	// EmptyAuthType is the authentication type used for providers
	// that require no credentials, e.g. "lxd", and "manual".
	EmptyAuthType AuthType = "empty"
)

// Cloud is a cloud definition.
type Cloud struct {
	// Type is the type of cloud, eg aws, openstack etc.
	Type string

	// AuthTypes are the authentication modes supported by the cloud.
	AuthTypes []AuthType

	// Endpoint is the default endpoint for the cloud regions, may be
	// overridden by a region.
	Endpoint string

	// StorageEndpoint is the default storage endpoint for the cloud
	// regions, may be overridden by a region.
	StorageEndpoint string

	// Regions are the regions available in the cloud.
	//
	// Regions is a slice, and not a map, because order is important.
	// The first region in the slice is the default region for the
	// cloud.
	Regions []Region
}

// Region is a cloud region.
type Region struct {
	// Name is the name of the region.
	Name string

	// Endpoint is the region's primary endpoint URL.
	Endpoint string

	// StorageEndpoint is the region's storage endpoint URL.
	// If the cloud/region does not have a storage-specific
	// endpoint URL, this will be empty.
	StorageEndpoint string
}

// cloudSet contains cloud definitions, used for marshalling and
// unmarshalling.
type cloudSet struct {
	// Clouds is a map of cloud definitions, keyed on cloud name.
	Clouds map[string]*cloud `yaml:"clouds"`
}

// cloud is equivalent to Cloud, for marshalling and unmarshalling.
type cloud struct {
	Type            string     `yaml:"type"`
	AuthTypes       []AuthType `yaml:"auth-types,omitempty,flow"`
	Endpoint        string     `yaml:"endpoint,omitempty"`
	StorageEndpoint string     `yaml:"storage-endpoint,omitempty"`
	Regions         regions    `yaml:"regions,omitempty"`
}

// regions is a collection of regions, either as a map and/or
// as a yaml.MapSlice.
//
// When marshalling, we populate the Slice field only. This is
// necessary for us to control the order of map items.
//
// When unmarshalling, we populate both Map and Slice. Map is
// populated to simplify conversion to Region objects. Slice
// is populated so we can identify the first map item, which
// becomes the default region for the cloud.
type regions struct {
	Map   map[string]*region
	Slice yaml.MapSlice
}

// region is equivalent to Region, for marshalling and unmarshalling.
type region struct {
	Endpoint        string `yaml:"endpoint,omitempty"`
	StorageEndpoint string `yaml:"storage-endpoint,omitempty"`
}

// BuiltInProviderNames work out of the box.
var BuiltInProviderNames = []string{"lxd", "manual"}

// CloudByName returns the cloud with the specified name.
// If there exists no cloud with the specified name, an
// error satisfying errors.IsNotFound will be returned.
//
// TODO(axw) write unit tests for this.
func CloudByName(name string) (*Cloud, error) {
	// Personal clouds take precedence.
	personalClouds, err := PersonalCloudMetadata()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if cloud, ok := personalClouds[name]; ok {
		return &cloud, nil
	}
	clouds, _, err := PublicCloudMetadata(JujuPublicCloudsPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if cloud, ok := clouds[name]; ok {
		return &cloud, nil
	}
	return nil, errors.NotFoundf("cloud %s", name)
}

// JujuPublicCloudsPath is the location where public cloud information is
// expected to be found. Requires JUJU_HOME to be set.
func JujuPublicCloudsPath() string {
	return osenv.JujuXDGDataHomePath("public-clouds.yaml")
}

// PublicCloudMetadata looks in searchPath for cloud metadata files and if none
// are found, returns the fallback public cloud metadata.
func PublicCloudMetadata(searchPath ...string) (result map[string]Cloud, fallbackUsed bool, err error) {
	for _, file := range searchPath {
		data, err := ioutil.ReadFile(file)
		if err != nil && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, false, errors.Trace(err)
		}
		clouds, err := ParseCloudMetadata(data)
		if err != nil {
			return nil, false, errors.Trace(err)
		}
		return clouds, false, err
	}
	clouds, err := ParseCloudMetadata([]byte(fallbackPublicCloudInfo))
	return clouds, true, err
}

// ParseCloudMetadata parses the given yaml bytes into Clouds metadata.
func ParseCloudMetadata(data []byte) (map[string]Cloud, error) {
	var metadata cloudSet
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal yaml cloud metadata")
	}

	// Translate to the exported type. For each cloud, we store
	// the first region for the cloud as its default region.
	clouds := make(map[string]Cloud)
	for name, cloud := range metadata.Clouds {
		var regions []Region
		if len(cloud.Regions.Map) > 0 {
			for _, item := range cloud.Regions.Slice {
				name := fmt.Sprint(item.Key)
				r := cloud.Regions.Map[name]
				if r == nil {
					// r will be nil if none of the fields in
					// the YAML are set.
					regions = append(regions, Region{Name: name})
				} else {
					regions = append(regions, Region{
						name, r.Endpoint, r.StorageEndpoint,
					})
				}
			}
		}
		meta := Cloud{
			Type:            cloud.Type,
			AuthTypes:       cloud.AuthTypes,
			Endpoint:        cloud.Endpoint,
			StorageEndpoint: cloud.StorageEndpoint,
			Regions:         regions,
		}
		meta.denormaliseMetadata()
		clouds[name] = meta
	}
	return clouds, nil
}

// marshalCloudMetadata marshals the given clouds to YAML.
func marshalCloudMetadata(cloudsMap map[string]Cloud) ([]byte, error) {
	clouds := cloudSet{make(map[string]*cloud)}
	for name, metadata := range cloudsMap {
		var regions regions
		for _, r := range metadata.Regions {
			regions.Slice = append(regions.Slice, yaml.MapItem{
				r.Name, region{r.Endpoint, r.StorageEndpoint},
			})
		}
		clouds.Clouds[name] = &cloud{
			Type:            metadata.Type,
			AuthTypes:       metadata.AuthTypes,
			Endpoint:        metadata.Endpoint,
			StorageEndpoint: metadata.StorageEndpoint,
			Regions:         regions,
		}
	}
	data, err := yaml.Marshal(clouds)
	if err != nil {
		return nil, errors.Annotate(err, "cannot marshal cloud metadata")
	}
	return data, nil
}

// MarshalYAML implements the yaml.Marshaler interface.
func (r regions) MarshalYAML() (interface{}, error) {
	return r.Slice, nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (r *regions) UnmarshalYAML(f func(interface{}) error) error {
	if err := f(&r.Map); err != nil {
		return err
	}
	return f(&r.Slice)
}

// To keep the metadata concise, attributes on the metadata struct which
// have the same value for each item may be moved up to a higher level in
// the tree. denormaliseMetadata descends the tree and fills in any missing
// attributes with values from a higher level.
func (cloud Cloud) denormaliseMetadata() {
	for name, region := range cloud.Regions {
		r := region
		inherit(&r, &cloud)
		cloud.Regions[name] = r
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
	RegisterStructTags(Cloud{}, Region{})
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
