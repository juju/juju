// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/names.v2"
)

type units struct {
	Version int     `yaml:"version"`
	Units_  []*unit `yaml:"units"`
}

type unit struct {
	Name_ string `yaml:"name"`

	Machine_ string `yaml:"machine"`

	AgentStatus_        *status        `yaml:"agent-status"`
	AgentStatusHistory_ StatusHistory_ `yaml:"agent-status-history"`

	WorkloadStatus_        *status        `yaml:"workload-status"`
	WorkloadStatusHistory_ StatusHistory_ `yaml:"workload-status-history"`

	WorkloadVersion_        string         `yaml:"workload-version,omitempty"`
	WorkloadVersionHistory_ StatusHistory_ `yaml:"workload-version-history"`

	Principal_    string   `yaml:"principal,omitempty"`
	Subordinates_ []string `yaml:"subordinates,omitempty"`

	PasswordHash_ string      `yaml:"password-hash"`
	Tools_        *agentTools `yaml:"tools"`

	MeterStatusCode_ string `yaml:"meter-status-code,omitempty"`
	MeterStatusInfo_ string `yaml:"meter-status-info,omitempty"`

	Annotations_ `yaml:"annotations,omitempty"`

	Constraints_ *constraints `yaml:"constraints,omitempty"`

	Payloads_ payloads `yaml:"payloads"`
}

// UnitArgs is an argument struct used to add a Unit to a Application in the Model.
type UnitArgs struct {
	Tag          names.UnitTag
	Machine      names.MachineTag
	PasswordHash string
	Principal    names.UnitTag
	Subordinates []names.UnitTag

	WorkloadVersion string
	MeterStatusCode string
	MeterStatusInfo string

	// TODO: storage attachment count
}

func newUnit(args UnitArgs) *unit {
	var subordinates []string
	for _, s := range args.Subordinates {
		subordinates = append(subordinates, s.Id())
	}
	u := &unit{
		Name_:                   args.Tag.Id(),
		Machine_:                args.Machine.Id(),
		PasswordHash_:           args.PasswordHash,
		Principal_:              args.Principal.Id(),
		Subordinates_:           subordinates,
		WorkloadVersion_:        args.WorkloadVersion,
		MeterStatusCode_:        args.MeterStatusCode,
		MeterStatusInfo_:        args.MeterStatusInfo,
		WorkloadStatusHistory_:  newStatusHistory(),
		WorkloadVersionHistory_: newStatusHistory(),
		AgentStatusHistory_:     newStatusHistory(),
	}
	u.setPayloads(nil)
	return u
}

// Tag implements Unit.
func (u *unit) Tag() names.UnitTag {
	return names.NewUnitTag(u.Name_)
}

// Name implements Unit.
func (u *unit) Name() string {
	return u.Name_
}

// Machine implements Unit.
func (u *unit) Machine() names.MachineTag {
	return names.NewMachineTag(u.Machine_)
}

// PasswordHash implements Unit.
func (u *unit) PasswordHash() string {
	return u.PasswordHash_
}

// Principal implements Unit.
func (u *unit) Principal() names.UnitTag {
	if u.Principal_ == "" {
		return names.UnitTag{}
	}
	return names.NewUnitTag(u.Principal_)
}

// Subordinates implements Unit.
func (u *unit) Subordinates() []names.UnitTag {
	var subordinates []names.UnitTag
	for _, s := range u.Subordinates_ {
		subordinates = append(subordinates, names.NewUnitTag(s))
	}
	return subordinates
}

// MeterStatusCode implements Unit.
func (u *unit) MeterStatusCode() string {
	return u.MeterStatusCode_
}

// MeterStatusInfo implements Unit.
func (u *unit) MeterStatusInfo() string {
	return u.MeterStatusInfo_
}

// Tools implements Unit.
func (u *unit) Tools() AgentTools {
	// To avoid a typed nil, check before returning.
	if u.Tools_ == nil {
		return nil
	}
	return u.Tools_
}

// SetTools implements Unit.
func (u *unit) SetTools(args AgentToolsArgs) {
	u.Tools_ = newAgentTools(args)
}

// WorkloadVersion implements Unit.
func (u *unit) WorkloadVersion() string {
	return u.WorkloadVersion_
}

// WorkloadStatus implements Unit.
func (u *unit) WorkloadStatus() Status {
	// To avoid typed nils check nil here.
	if u.WorkloadStatus_ == nil {
		return nil
	}
	return u.WorkloadStatus_
}

// SetWorkloadStatus implements Unit.
func (u *unit) SetWorkloadStatus(args StatusArgs) {
	u.WorkloadStatus_ = newStatus(args)
}

// WorkloadStatusHistory implements Unit.
func (u *unit) WorkloadStatusHistory() []Status {
	return u.WorkloadStatusHistory_.StatusHistory()
}

// SetWorkloadStatusHistory implements Unit.
func (u *unit) SetWorkloadStatusHistory(args []StatusArgs) {
	u.WorkloadStatusHistory_.SetStatusHistory(args)
}

// WorkloadVersionHistory implements Unit.
func (u *unit) WorkloadVersionHistory() []Status {
	return u.WorkloadVersionHistory_.StatusHistory()
}

// SetWorkloadVersionHistory implements Unit.
func (u *unit) SetWorkloadVersionHistory(args []StatusArgs) {
	u.WorkloadVersionHistory_.SetStatusHistory(args)
}

// AgentStatus implements Unit.
func (u *unit) AgentStatus() Status {
	// To avoid typed nils check nil here.
	if u.AgentStatus_ == nil {
		return nil
	}
	return u.AgentStatus_
}

// SetAgentStatus implements Unit.
func (u *unit) SetAgentStatus(args StatusArgs) {
	u.AgentStatus_ = newStatus(args)
}

// AgentStatusHistory implements Unit.
func (u *unit) AgentStatusHistory() []Status {
	return u.AgentStatusHistory_.StatusHistory()
}

// SetAgentStatusHistory implements Unit.
func (u *unit) SetAgentStatusHistory(args []StatusArgs) {
	u.AgentStatusHistory_.SetStatusHistory(args)
}

// Constraints implements HasConstraints.
func (u *unit) Constraints() Constraints {
	if u.Constraints_ == nil {
		return nil
	}
	return u.Constraints_
}

// SetConstraints implements HasConstraints.
func (u *unit) SetConstraints(args ConstraintsArgs) {
	u.Constraints_ = newConstraints(args)
}

// AddPayload implements Unit.
func (u *unit) AddPayload(args PayloadArgs) Payload {
	payload := newPayload(args)
	u.Payloads_.Payloads_ = append(u.Payloads_.Payloads_, payload)
	return payload
}

// Payloads implements Unit.
func (u *unit) Payloads() []Payload {
	var result []Payload
	for _, payload := range u.Payloads_.Payloads_ {
		result = append(result, payload)
	}
	return result
}

func (u *unit) setPayloads(payloadList []*payload) {
	u.Payloads_ = payloads{
		Version:   1,
		Payloads_: payloadList,
	}
}

// Validate impelements Unit.
func (u *unit) Validate() error {
	if u.Name_ == "" {
		return errors.NotValidf("missing name")
	}
	if u.AgentStatus_ == nil {
		return errors.NotValidf("unit %q missing agent status", u.Name_)
	}
	if u.WorkloadStatus_ == nil {
		return errors.NotValidf("unit %q missing workload status", u.Name_)
	}
	if u.Tools_ == nil {
		return errors.NotValidf("unit %q missing tools", u.Name_)
	}
	return nil
}

func importUnits(source map[string]interface{}) ([]*unit, error) {
	checker := versionedChecker("units")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "units version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := unitDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["units"].([]interface{})
	return importUnitList(sourceList, importFunc)
}

func importUnitList(sourceList []interface{}, importFunc unitDeserializationFunc) ([]*unit, error) {
	result := make([]*unit, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for unit %d, %T", i, value)
		}
		unit, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "unit %d", i)
		}
		result = append(result, unit)
	}
	return result, nil
}

type unitDeserializationFunc func(map[string]interface{}) (*unit, error)

var unitDeserializationFuncs = map[int]unitDeserializationFunc{
	1: importUnitV1,
}

func importUnitV1(source map[string]interface{}) (*unit, error) {
	fields := schema.Fields{
		"name":    schema.String(),
		"machine": schema.String(),

		"agent-status":             schema.StringMap(schema.Any()),
		"agent-status-history":     schema.StringMap(schema.Any()),
		"workload-status":          schema.StringMap(schema.Any()),
		"workload-status-history":  schema.StringMap(schema.Any()),
		"workload-version":         schema.String(),
		"workload-version-history": schema.StringMap(schema.Any()),

		"principal":    schema.String(),
		"subordinates": schema.List(schema.String()),

		"password-hash": schema.String(),
		"tools":         schema.StringMap(schema.Any()),

		"meter-status-code": schema.String(),
		"meter-status-info": schema.String(),

		"payloads": schema.StringMap(schema.Any()),
	}
	defaults := schema.Defaults{
		"principal":         "",
		"subordinates":      schema.Omit,
		"workload-version":  "",
		"meter-status-code": "",
		"meter-status-info": "",
	}
	addAnnotationSchema(fields, defaults)
	addConstraintsSchema(fields, defaults)
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "unit v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	result := &unit{
		Name_:                   valid["name"].(string),
		Machine_:                valid["machine"].(string),
		Principal_:              valid["principal"].(string),
		PasswordHash_:           valid["password-hash"].(string),
		WorkloadVersion_:        valid["workload-version"].(string),
		MeterStatusCode_:        valid["meter-status-code"].(string),
		MeterStatusInfo_:        valid["meter-status-info"].(string),
		WorkloadStatusHistory_:  newStatusHistory(),
		WorkloadVersionHistory_: newStatusHistory(),
		AgentStatusHistory_:     newStatusHistory(),
	}
	result.importAnnotations(valid)

	workloadStatusHistory := valid["workload-status-history"].(map[string]interface{})
	if err := importStatusHistory(&result.WorkloadStatusHistory_, workloadStatusHistory); err != nil {
		return nil, errors.Trace(err)
	}
	workloadVersionHistory := valid["workload-version-history"].(map[string]interface{})
	if err := importStatusHistory(&result.WorkloadVersionHistory_, workloadVersionHistory); err != nil {
		return nil, errors.Trace(err)
	}
	agentHistory := valid["agent-status-history"].(map[string]interface{})
	if err := importStatusHistory(&result.AgentStatusHistory_, agentHistory); err != nil {
		return nil, errors.Trace(err)
	}

	if constraintsMap, ok := valid["constraints"]; ok {
		constraints, err := importConstraints(constraintsMap.(map[string]interface{}))
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.Constraints_ = constraints
	}

	result.Subordinates_ = convertToStringSlice(valid["subordinates"])

	// Tools and status are required, so we expect them to be there.
	tools, err := importAgentTools(valid["tools"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.Tools_ = tools

	agentStatus, err := importStatus(valid["agent-status"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.AgentStatus_ = agentStatus

	workloadStatus, err := importStatus(valid["workload-status"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.WorkloadStatus_ = workloadStatus

	payloadMap := valid["payloads"].(map[string]interface{})
	payloads, err := importPayloads(payloadMap)
	if err != nil {
		return nil, errors.Annotate(err, "payloads")
	}
	result.setPayloads(payloads)

	return result, nil
}
