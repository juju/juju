// Copyright 2012-2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package checkers

import (
	"encoding/json"
	"fmt"

	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v2"
)

type codecEqualChecker struct {
	name      string
	marshal   func(interface{}) ([]byte, error)
	unmarshal func([]byte, interface{}) error
}

// BSONEquals defines a checker that checks whether a byte slice, when
// unmarshaled as BSON, is equal to the given value. Rather than
// unmarshaling into something of the expected body type, we reform
// the expected body in BSON and back to interface{} so we can check
// the whole content. Otherwise we lose information when unmarshaling.
var BSONEquals = &codecEqualChecker{
	name:      "BSONEquals",
	marshal:   bson.Marshal,
	unmarshal: bson.Unmarshal,
}

// JSONEquals defines a checker that checks whether a byte slice, when
// unmarshaled as JSON, is equal to the given value.
// Rather than unmarshaling into something of the expected
// body type, we reform the expected body in JSON and
// back to interface{}, so we can check the whole content.
// Otherwise we lose information when unmarshaling.
var JSONEquals = &codecEqualChecker{
	name:      "JSONEquals",
	marshal:   json.Marshal,
	unmarshal: json.Unmarshal,
}

// YAMLEquals defines a checker that checks whether a byte slice, when
// unmarshaled as YAML, is equal to the given value.
// Rather than unmarshaling into something of the expected
// body type, we reform the expected body in YAML and
// back to interface{}, so we can check the whole content.
// Otherwise we lose information when unmarshaling.
var YAMLEquals = &codecEqualChecker{
	name:      "YAMLEquals",
	marshal:   yaml.Marshal,
	unmarshal: yaml.Unmarshal,
}

func (checker *codecEqualChecker) Info() *gc.CheckerInfo {
	return &gc.CheckerInfo{
		Name:   checker.name,
		Params: []string{"obtained", "expected"},
	}
}

func (checker *codecEqualChecker) Check(params []interface{}, names []string) (result bool, error string) {
	gotContent, ok := params[0].(string)
	if !ok {
		return false, fmt.Sprintf("expected string, got %T", params[0])
	}
	expectContent := params[1]
	expectContentBytes, err := checker.marshal(expectContent)
	if err != nil {
		return false, fmt.Sprintf("cannot marshal expected contents: %v", err)
	}
	var expectContentVal interface{}
	if err := checker.unmarshal(expectContentBytes, &expectContentVal); err != nil {
		return false, fmt.Sprintf("cannot unmarshal expected contents: %v", err)
	}

	var gotContentVal interface{}
	if err := checker.unmarshal([]byte(gotContent), &gotContentVal); err != nil {
		return false, fmt.Sprintf("cannot unmarshal obtained contents: %v; %q", err, gotContent)
	}

	if ok, err := DeepEqual(gotContentVal, expectContentVal); !ok {
		return false, err.Error()
	}
	return true, ""
}
