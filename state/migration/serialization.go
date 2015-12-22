// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

// importModel constructs a new Model from a map that in normal usage situations
// will be the result of interpreting a large YAML document.
//
// This method is a package internal serialisation method.
func importModel(source map[string]interface{}) (*model, error) {
	version, err := getVersion(source)
	if err != nil {
		return nil, errors.Trace(err)
	}

	importFunc, ok := modelDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}

	return importFunc(source)
}

type modelDeserializationFunc func(map[string]interface{}) (*model, error)

var modelDeserializationFuncs = map[int]modelDeserializationFunc{
	1: importModelV1,
}

func importModelV1(source map[string]interface{}) (*model, error) {
	result := &model{Version: 1}

	fields := schema.Fields{
		"owner":    schema.String(),
		"config":   schema.StringMap(schema.Any()),
		"machines": schema.StringMap(schema.Any()),
	}
	checker := schema.FieldMap(fields, nil) // no defaults

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "model v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	result.Owner_ = valid["owner"].(string)
	result.Config_ = valid["config"].(map[string]interface{})

	machineMap := valid["machines"].(map[string]interface{})
	machines, err := importMachines(machineMap)
	if err != nil {
		return nil, errors.Annotatef(err, "machines")
	}
	result.setMachines(machines)

	return result, nil
}

func importMachines(source map[string]interface{}) ([]*machine, error) {
	checker := versionedChecker("machines")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "machines version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := machineDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["machines"].([]interface{})
	return importMachineList(sourceList, importFunc)
}

func importMachineList(sourceList []interface{}, importFunc machineDeserializationFunc) ([]*machine, error) {
	result := make([]*machine, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for machine %d, %T", i, value)
		}
		machine, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "machine %d", i)
		}
		result = append(result, machine)
	}
	return result, nil
}

type machineDeserializationFunc func(map[string]interface{}) (*machine, error)

var machineDeserializationFuncs = map[int]machineDeserializationFunc{
	1: importMachineV1,
}

func importMachineV1(source map[string]interface{}) (*machine, error) {
	result := &machine{}

	fields := schema.Fields{
		"id":         schema.String(),
		"containers": schema.List(schema.StringMap(schema.Any())),
	}
	checker := schema.FieldMap(fields, nil) // no defaults

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "machine v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	result.Id_ = valid["id"].(string)
	machineList := valid["containers"].([]interface{})
	machines, err := importMachineList(machineList, importMachineV1)
	if err != nil {
		return nil, errors.Annotatef(err, "containers")
	}
	result.Containers_ = machines

	return result, nil

}

func versionedChecker(name string) schema.Checker {
	fields := schema.Fields{
		"version": schema.Int(),
	}
	if name != "" {
		fields[name] = schema.List(schema.StringMap(schema.Any()))
	}
	return schema.FieldMap(fields, nil) // no defaults
}

func getVersion(source map[string]interface{}) (int, error) {
	checker := versionedChecker("")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return 0, errors.Trace(err)
	}
	valid := coerced.(map[string]interface{})
	return int(valid["version"].(int64)), nil
}
