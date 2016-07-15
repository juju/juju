// JSON marshaling and unmarshaling support for Openstack compute data structures.
// This package deals with the difference between the API and on-the-wire data types.
// Differences include encoding entity IDs as string vs int, depending on the Openstack
// variant used.
//
// The marshaling support is included primarily for use by the test doubles. It needs to be
// included here since Go requires methods to implemented in the same package as their receiver.

package nova

import (
	"encoding/json"
	"fmt"
	"strconv"
)

const (
	idTag            = "id"
	instanceIdTag    = "instance_id"
	groupIdTag       = "group_id"
	parentGroupIdTag = "parent_group_id"
)

var useNumericIds bool = false

// convertId returns the id as either a string or an int depending on what
// implementation of Openstack we are emulating.
func convertId(id string) interface{} {
	if !useNumericIds {
		return id
	}
	result, err := strconv.Atoi(id)
	if err != nil {
		panic(err)
	}
	return result
}

// getIdAsStringPtr extracts the field with the specified tag from the json data
// and returns it converted to a string pointer.
func getIdAsStringPtr(b []byte, tag string) (*string, error) {
	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	val, ok := out[tag]
	if !ok || val == nil {
		return nil, nil
	}
	floatVal, ok := val.(float64)
	var strVal string
	if ok {
		strVal = fmt.Sprint(int(floatVal))
	} else {
		strVal = fmt.Sprint(val)
	}
	return &strVal, nil
}

// getIdAsString extracts the field with the specified tag from the json data
// and returns it converted to a string.
func getIdAsString(b []byte, tag string) (string, error) {
	strPtr, err := getIdAsStringPtr(b, tag)
	if err != nil {
		return "", err
	}
	if strPtr == nil {
		return "", nil
	}
	return *strPtr, nil
}

// appendJSON marshals the given attribute value and appends it as an encoded value to the given json data.
// The newly encode (attr, value) is inserted just before the closing "}" in the json data.
func appendJSON(data []byte, attr string, value interface{}) ([]byte, error) {
	newData, err := json.Marshal(&value)
	if err != nil {
		return nil, err
	}
	strData := string(data)
	result := fmt.Sprintf(`%s, "%s":%s}`, strData[:len(strData)-1], attr, string(newData))
	return []byte(result), nil
}

type jsonEntity Entity

func (entity *Entity) UnmarshalJSON(b []byte) error {
	var je jsonEntity = jsonEntity(*entity)
	var err error
	if err = json.Unmarshal(b, &je); err != nil {
		return err
	}
	if je.Id, err = getIdAsString(b, idTag); err != nil {
		return err
	}
	*entity = Entity(je)
	return nil
}

func (entity Entity) MarshalJSON() ([]byte, error) {
	var je jsonEntity = jsonEntity(entity)
	data, err := json.Marshal(&je)
	if err != nil {
		return nil, err
	}
	id := convertId(entity.Id)
	return appendJSON(data, idTag, id)
}

type jsonFlavorDetail FlavorDetail

func (flavorDetail *FlavorDetail) UnmarshalJSON(b []byte) error {
	var jfd jsonFlavorDetail = jsonFlavorDetail(*flavorDetail)
	var err error
	if err = json.Unmarshal(b, &jfd); err != nil {
		return err
	}
	if jfd.Id, err = getIdAsString(b, idTag); err != nil {
		return err
	}
	*flavorDetail = FlavorDetail(jfd)
	return nil
}

func (flavorDetail FlavorDetail) MarshalJSON() ([]byte, error) {
	var jfd jsonFlavorDetail = jsonFlavorDetail(flavorDetail)
	data, err := json.Marshal(&jfd)
	if err != nil {
		return nil, err
	}
	id := convertId(flavorDetail.Id)
	return appendJSON(data, idTag, id)
}

type jsonServerDetail ServerDetail

func (serverDetail *ServerDetail) UnmarshalJSON(b []byte) error {
	var jsd jsonServerDetail = jsonServerDetail(*serverDetail)
	var err error
	if err = json.Unmarshal(b, &jsd); err != nil {
		return err
	}
	if jsd.Id, err = getIdAsString(b, idTag); err != nil {
		return err
	}
	*serverDetail = ServerDetail(jsd)
	return nil
}

func (serverDetail ServerDetail) MarshalJSON() ([]byte, error) {
	var jsd jsonServerDetail = jsonServerDetail(serverDetail)
	data, err := json.Marshal(&jsd)
	if err != nil {
		return nil, err
	}
	id := convertId(serverDetail.Id)
	return appendJSON(data, idTag, id)
}

type jsonFloatingIP FloatingIP

func (floatingIP *FloatingIP) UnmarshalJSON(b []byte) error {
	var jfip jsonFloatingIP = jsonFloatingIP(*floatingIP)
	var err error
	if err = json.Unmarshal(b, &jfip); err != nil {
		return err
	}
	if instIdPtr, err := getIdAsStringPtr(b, instanceIdTag); err != nil {
		return err
	} else {
		jfip.InstanceId = instIdPtr
	}
	if jfip.Id, err = getIdAsString(b, idTag); err != nil {
		return err
	}
	*floatingIP = FloatingIP(jfip)
	return nil
}

func (floatingIP FloatingIP) MarshalJSON() ([]byte, error) {
	var jfip jsonFloatingIP = jsonFloatingIP(floatingIP)
	data, err := json.Marshal(&jfip)
	if err != nil {
		return nil, err
	}
	id := convertId(floatingIP.Id)
	data, err = appendJSON(data, idTag, id)
	if err != nil {
		return nil, err
	}
	if floatingIP.InstanceId == nil {
		return data, nil
	}
	instId := convertId(*floatingIP.InstanceId)
	return appendJSON(data, instanceIdTag, instId)
}

type jsonSecurityGroup SecurityGroup

func (securityGroup *SecurityGroup) UnmarshalJSON(b []byte) error {
	var jsg jsonSecurityGroup = jsonSecurityGroup(*securityGroup)
	var err error
	if err = json.Unmarshal(b, &jsg); err != nil {
		return err
	}
	if jsg.Id, err = getIdAsString(b, idTag); err != nil {
		return err
	}
	*securityGroup = SecurityGroup(jsg)
	return nil
}

func (securityGroup SecurityGroup) MarshalJSON() ([]byte, error) {
	var jsg jsonSecurityGroup = jsonSecurityGroup(securityGroup)
	data, err := json.Marshal(&jsg)
	if err != nil {
		return nil, err
	}
	id := convertId(securityGroup.Id)
	return appendJSON(data, idTag, id)
}

type jsonSecurityGroupRule SecurityGroupRule

func (securityGroupRule *SecurityGroupRule) UnmarshalJSON(b []byte) error {
	var jsgr jsonSecurityGroupRule = jsonSecurityGroupRule(*securityGroupRule)
	var err error
	if err = json.Unmarshal(b, &jsgr); err != nil {
		return err
	}
	if jsgr.Id, err = getIdAsString(b, idTag); err != nil {
		return err
	}
	if jsgr.ParentGroupId, err = getIdAsString(b, parentGroupIdTag); err != nil {
		return err
	}
	*securityGroupRule = SecurityGroupRule(jsgr)
	return nil
}

func (securityGroupRule SecurityGroupRule) MarshalJSON() ([]byte, error) {
	var jsgr jsonSecurityGroupRule = jsonSecurityGroupRule(securityGroupRule)
	data, err := json.Marshal(&jsgr)
	if err != nil {
		return nil, err
	}
	id := convertId(securityGroupRule.Id)
	data, err = appendJSON(data, idTag, id)
	if err != nil {
		return nil, err
	}
	if securityGroupRule.ParentGroupId == "" {
		return data, nil
	}
	id = convertId(securityGroupRule.ParentGroupId)
	return appendJSON(data, parentGroupIdTag, id)
}

type jsonRuleInfo RuleInfo

func (ruleInfo *RuleInfo) UnmarshalJSON(b []byte) error {
	var jri jsonRuleInfo = jsonRuleInfo(*ruleInfo)
	var err error
	if err = json.Unmarshal(b, &jri); err != nil {
		return err
	}
	if jri.ParentGroupId, err = getIdAsString(b, parentGroupIdTag); err != nil {
		return err
	}
	if groupIdPtr, err := getIdAsStringPtr(b, groupIdTag); err != nil {
		return err
	} else {
		jri.GroupId = groupIdPtr
	}
	*ruleInfo = RuleInfo(jri)
	return nil
}

func (ruleInfo RuleInfo) MarshalJSON() ([]byte, error) {
	var jri jsonRuleInfo = jsonRuleInfo(ruleInfo)
	data, err := json.Marshal(&jri)
	if err != nil {
		return nil, err
	}
	id := convertId(ruleInfo.ParentGroupId)
	data, err = appendJSON(data, parentGroupIdTag, id)
	if err != nil {
		return nil, err
	}
	if ruleInfo.GroupId == nil {
		return data, nil
	}
	id = convertId(*ruleInfo.GroupId)
	return appendJSON(data, groupIdTag, id)
}
