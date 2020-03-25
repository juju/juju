// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package cloud provides functionality to parse information
// describing clouds, including regions, supported auth types etc.

package cloud

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gojsonschema"
	"gopkg.in/yaml.v2"
)

//ValidationWarning are JSON schema validation errors used to warn users about
//potential schema violations
type ValidationWarning struct {
	Messages []string
}

func (e *ValidationWarning) Error() string {
	str := ""
	for _, msg := range e.Messages {
		str = fmt.Sprintf("%s\n%s", str, msg)
	}

	return str
}

var cloudSetSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"clouds": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": cloudSchema,
		},
	},
	"additionalProperties": false,
}

var cloudSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"name":        map[string]interface{}{"type": "string"},
		"type":        map[string]interface{}{"type": "string"},
		"description": map[string]interface{}{"type": "string"},
		"auth-types": map[string]interface{}{
			"type":  "array",
			"items": map[string]interface{}{"type": "string"},
		},
		"host-cloud-region": map[string]interface{}{"type": "string"},
		"endpoint":          map[string]interface{}{"type": "string"},
		"identity-endpoint": map[string]interface{}{"type": "string"},
		"storage-endpoint":  map[string]interface{}{"type": "string"},
		"config":            map[string]interface{}{"type": "object"},
		"regions":           regionsSchema,
		"region-config":     map[string]interface{}{"type": "object"},
		"ca-certificates": map[string]interface{}{
			"type":  "array",
			"items": map[string]interface{}{"type": "string"},
		},
	},
	"additionalProperties": false,
}

var regionsSchema = map[string]interface{}{
	"type": "object",
	"additionalProperties": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"endpoint":          map[string]interface{}{"type": "string"},
			"identity-endpoint": map[string]interface{}{"type": "string"},
			"storage-endpoint":  map[string]interface{}{"type": "string"},
		},
		"additionalProperties": false,
	},
}

// ValidateCloudSet reports any erroneous properties found in cloud metadata
// YAML. If there are no erroneous properties, then ValidateCloudSet returns nil
// otherwise it return an error listing all erroneous properties and possible
// suggestion.
func ValidateCloudSet(data []byte) error {
	return validateCloud(data, &cloudSetSchema)
}

// ValidateOneCloud is like ValidateCloudSet but validates the metadata for only
// one cloud and not multiple.
func ValidateOneCloud(data []byte) error {
	return validateCloud(data, &cloudSchema)
}

func validateCloud(data []byte, jsonSchema *map[string]interface{}) error {
	var body interface{}
	if err := yaml.Unmarshal(data, &body); err != nil {
		return errors.Annotate(err, "cannot unmarshal yaml cloud metadata")
	}

	jsonBody := yamlToJSON(body)
	invalidKeys, err := validateCloudMetaData(jsonBody, jsonSchema)
	if err != nil {
		return errors.Annotate(err, "cannot validate yaml cloud metadata")
	}

	formatKeyError := func(invalidKey, similarValidKey string) string {
		str := fmt.Sprintf("property %s is invalid.", invalidKey)
		if similarValidKey != "" {
			str = fmt.Sprintf("%s Perhaps you mean %q.", str, similarValidKey)
		}
		return str
	}

	cloudValidationError := ValidationWarning{}
	for k, v := range invalidKeys {
		cloudValidationError.Messages = append(cloudValidationError.Messages, formatKeyError(k, v))
	}

	if len(cloudValidationError.Messages) != 0 {
		return &cloudValidationError
	}

	return nil
}

func cloudTags() []string {
	keys := make(map[string]struct{})
	collectTags(reflect.TypeOf((*cloud)(nil)), "yaml", []string{"map[string]*cloud.region", "yaml.MapSlice"}, &keys)
	keyList := make([]string, 0, len(keys))
	for k := range keys {
		keyList = append(keyList, k)
	}

	return keyList
}

// collectTags returns a set of keys for a specified struct tag. If no tag is
// specified for a particular field of the argument struct type, then the
// all-lowercase field name is used as per Go tag conventions. If the tag
// specified is not the name a conventionally formatted go struct tag, then the
// results of this function are invalid. Values of invalid kinds result in no
// processing.
func collectTags(t reflect.Type, tag string, ignoreTypes []string, keys *map[string]struct{}) {
	switch t.Kind() {

	case reflect.Array, reflect.Slice, reflect.Map, reflect.Ptr:
		collectTags(t.Elem(), tag, ignoreTypes, keys)

	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)

			fieldTag := field.Tag.Get(tag)
			var fieldTagKey string

			ignoredType := false
			for _, it := range ignoreTypes {
				if field.Type.String() == it {
					ignoredType = true
					break
				}
			}

			if fieldTag == "-" || ignoredType {
				continue
			}

			if len(fieldTag) > 0 {
				fieldTagKey = strings.Split(fieldTag, ",")[0]
			} else {
				fieldTagKey = strings.ToLower(field.Name)
			}

			(*keys)[fieldTagKey] = struct{}{}
			collectTags(field.Type, tag, ignoreTypes, keys)
		}
	}
}

func validateCloudMetaData(body interface{}, jsonSchema *map[string]interface{}) (map[string]string, error) {
	documentLoader := gojsonschema.NewGoLoader(body)
	schemaLoader := gojsonschema.NewGoLoader(jsonSchema)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return nil, err
	}

	minEditingDistance := 5

	validCloudProperties := cloudTags()
	suggestionMap := map[string]string{}
	for _, rsltErr := range result.Errors() {
		invalidProperty := strings.Split(rsltErr.Description, " ")[2]
		suggestionMap[invalidProperty] = ""
		editingDistance := minEditingDistance

		for _, validProperty := range validCloudProperties {

			dist := distance(invalidProperty, validProperty)
			if dist < editingDistance && dist < minEditingDistance {
				editingDistance = dist
				suggestionMap[invalidProperty] = validProperty
			}

		}
	}

	return suggestionMap, nil
}

func yamlToJSON(i interface{}) interface{} {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			m2[k.(string)] = yamlToJSON(v)
		}
		return m2
	case []interface{}:
		for i, v := range x {
			x[i] = yamlToJSON(v)
		}
	}
	return i
}

// The following "editing distance" comparator was lifted from
// https://github.com/arbovm/levenshtein/blob/master/levenshtein.go which has a
// compatible BSD license. We use it to calculate the distance between a
// discovered invalid yaml property and known good properties to identify
// suggestions.
func distance(str1, str2 string) int {
	var cost, lastdiag, olddiag int
	s1 := []rune(str1)
	s2 := []rune(str2)

	lenS1 := len(s1)
	lenS2 := len(s2)

	column := make([]int, lenS1+1)

	for y := 1; y <= lenS1; y++ {
		column[y] = y
	}

	for x := 1; x <= lenS2; x++ {
		column[0] = x
		lastdiag = x - 1
		for y := 1; y <= lenS1; y++ {
			olddiag = column[y]
			cost = 0
			if s1[y-1] != s2[x-1] {
				cost = 1
			}
			column[y] = min(
				column[y]+1,
				column[y-1]+1,
				lastdiag+cost)
			lastdiag = olddiag
		}
	}
	return column[lenS1]
}

func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
	} else {
		if b < c {
			return b
		}
	}
	return c
}
