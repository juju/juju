// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/kr/pretty"
	core "k8s.io/api/core/v1"

	"github.com/juju/juju/caas/specs"
)

var boolValues = set.NewStrings(
	strings.Split("y|Y|yes|Yes|YES|n|N|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF", "|")...)

var specialValues = ":{}[],&*#?|-<>=!%@`"

func quoteStrings(config map[string]interface{}) {
	// Any string config values that could be interpreted as bools
	// or which contain special YAML chars need to be quoted.
	for k, v := range config {
		strValue, ok := v.(string)
		if !ok {
			continue
		}
		config[k] = quoteString(strValue)
	}
}

func quoteString(i string) string {
	if boolValues.Contains(i) || strings.IndexAny(i, specialValues) >= 0 {
		i = strings.Replace(i, "'", "''", -1)
		i = fmt.Sprintf("'%s'", i)
	}
	return i
}

type fieldRef struct {
	FieldRef *core.ObjectFieldSelector `json:"fieldRef" yaml:"fieldRef"`
}

func (fr fieldRef) to(name string) (out []core.EnvVar) {
	if fr.FieldRef != nil {
		out = append(out, core.EnvVar{Name: name, ValueFrom: &core.EnvVarSource{FieldRef: fr.FieldRef}})
	}
	return out
}

type resourceRef struct {
	ResourceRef *core.ResourceFieldSelector `json:"resourceRef" yaml:"resourceRef"`
}

func (rr resourceRef) to(name string) (out []core.EnvVar) {
	if rr.ResourceRef != nil {
		out = append(out, core.EnvVar{Name: name, ValueFrom: &core.EnvVarSource{ResourceFieldRef: rr.ResourceRef}})
	}
	return out
}

const (
	secretRefName    = "secretRef"
	configMapRefName = "configMapRef"
)

type secretRefValue []core.SecretEnvSource
type configMapRefValue []core.ConfigMapEnvSource

func (sr secretRefValue) to() (out []core.EnvFromSource) {
	for _, v := range sr {
		sRef := v
		out = append(out, core.EnvFromSource{SecretRef: &sRef})
	}
	return out
}

func (cmr configMapRefValue) to() (out []core.EnvFromSource) {
	for _, v := range cmr {
		cRef := v
		out = append(out, core.EnvFromSource{ConfigMapRef: &cRef})
	}
	return out
}

type secretKeyRef struct {
	SecretKeyRef *core.SecretKeySelector `json:"secretKeyRef" yaml:"secretKeyRef"`
}

func (skr secretKeyRef) to(name string) (out []core.EnvVar) {
	if skr.SecretKeyRef != nil {
		out = append(out, core.EnvVar{Name: name, ValueFrom: &core.EnvVarSource{SecretKeyRef: skr.SecretKeyRef}})
	}
	return out
}

type configMapKeyRef struct {
	ConfigMapKeyRef *core.ConfigMapKeySelector `json:"configMapKeyRef" yaml:"configMapKeyRef"`
}

func (cmkr configMapKeyRef) to(name string) (out []core.EnvVar) {
	if cmkr.ConfigMapKeyRef != nil {
		out = append(out, core.EnvVar{Name: name, ValueFrom: &core.EnvVarSource{ConfigMapKeyRef: cmkr.ConfigMapKeyRef}})
	}
	return out
}

type configValue struct {
	fieldRef    `json:",inline" yaml:",inline"`
	resourceRef `json:",inline" yaml:",inline"`

	secretKeyRef    `json:",inline" yaml:",inline"`
	configMapKeyRef `json:",inline" yaml:",inline"`
}

func (cv configValue) to(name string) (out []core.EnvVar, err error) {
	out = append(out, cv.fieldRef.to(name)...)
	out = append(out, cv.resourceRef.to(name)...)
	out = append(out, cv.secretKeyRef.to(name)...)
	out = append(out, cv.configMapKeyRef.to(name)...)

	if len(out) == 0 {
		return nil, errors.NotSupportedf("config format of %q", name)
	}
	if len(out) > 1 {
		return nil, errors.NotValidf("duplicated values found for config %q", name)
	}
	return out, nil
}

// JSON Unmarshal types - https://golang.org/pkg/encoding/json/#Unmarshal
func stringify(i interface{}) (string, error) {
	switch v := i.(type) {
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case float64:
		// All the numbers are float64 - https://golang.org/pkg/encoding/json/#Number
		return fmt.Sprintf("%g", v), nil
	default:
		return "", errors.NotSupportedf("%v with type %T", i, i)
	}
}

func processMapInterfaceValue(k string, v interface{}) (envVars []core.EnvVar, envFromSources []core.EnvFromSource, err error) {
	logger.Tracef("processing container config key: %q, value(%T): %+v", k, v, v)

	parse := func(into interface{}) error {
		jsonbody, err := json.Marshal(v)
		if err != nil {
			return errors.Trace(err)
		}
		decoder := newStrictYAMLOrJSONDecoder(bytes.NewReader(jsonbody), len(jsonbody))
		return decoder.Decode(&into)
	}
	switch k {
	case secretRefName:
		var val secretRefValue
		defer func() {
			envFromSources = append(envFromSources, val.to()...)
		}()
		return envVars, envFromSources, errors.Trace(parse(&val))
	case configMapRefName:
		var val configMapRefValue
		defer func() {
			envFromSources = append(envFromSources, val.to()...)
		}()
		return envVars, envFromSources, errors.Trace(parse(&val))
	default:
		var val configValue
		if err := parse(&val); err != nil {
			return nil, nil, errors.Trace(err)
		}
		items, err := val.to(k)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		envVars = append(envVars, items...)
		return envVars, envFromSources, nil
	}
}

// ContainerConfigToK8sEnvConfig converts ContainerConfig to k8s format for container value mount.
func ContainerConfigToK8sEnvConfig(cc specs.ContainerConfig) (envVars []core.EnvVar, envFromSources []core.EnvFromSource, err error) {
	valNames := set.NewStrings()
	for k, v := range cc {
		if valNames.Contains(k) {
			return nil, nil, errors.NotValidf("duplicated config %q", k)
		}
		valNames.Add(k)

		if k != secretRefName && k != configMapRefName {
			// Normal key value pairs.
			if strV, err := stringify(v); err == nil {
				envVars = append(envVars, core.EnvVar{Name: k, Value: strV})
				continue
			} else {
				logger.Criticalf("stringify %q:%#v, err -> %+v", k, v, err)
			}
		}

		switch envVal := v.(type) {
		case map[string]interface{}, []interface{}:

			vars, envFroms, err := processMapInterfaceValue(k, envVal)
			if err != nil {
				logger.Criticalf("processMapInterfaceValue err -> %v", errors.ErrorStack(err))
				return nil, nil, errors.Trace(err)
			}
			envVars = append(envVars, vars...)
			envFromSources = append(envFromSources, envFroms...)
		default:
			return nil, nil, errors.NotSupportedf("config %q with type %T", k, v)
		}
	}

	// Sort for tests.
	sort.SliceStable(envVars, func(i, j int) bool {
		return envVars[i].Name < envVars[j].Name
	})
	sort.SliceStable(envFromSources, func(i, j int) bool {
		return getEnvFromSourceName(envFromSources[i]) < getEnvFromSourceName(envFromSources[j])
	})

	logger.Criticalf("envVars -> %s", pretty.Sprint(envVars))
	logger.Criticalf("envFromSources -> %s", pretty.Sprint(envFromSources))
	return envVars, envFromSources, nil
}

func getEnvFromSourceName(in core.EnvFromSource) (out string) {
	if in.ConfigMapRef != nil {
		return in.ConfigMapRef.Name
	}
	if in.SecretRef != nil {
		return in.SecretRef.Name
	}
	return out
}
