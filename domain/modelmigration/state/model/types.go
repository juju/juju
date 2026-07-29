// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

// modelInfo represents the model's read only information from the model table
// in the model database.
type modelInfo struct {
	// ControllerUUID is the controllers unique id.
	ControllerUUID string `db:"controller_uuid"`
}

// modelType represents the model's deployment type.
type modelType struct {
	Type string `db:"type"`
}

// agentName represents an agent-bearing entity name.
type agentName struct {
	Name string `db:"name"`
}

type entityUUID struct {
	UUID string `db:"uuid"`
}

// machineInstanceID pairs a machine name with its provider cloud instance ID.
type machineInstanceID struct {
	MachineName string `db:"name"`
	InstanceID  string `db:"instance_id"`
}

// instanceID is a machine's provider cloud instance ID on its own.
type instanceID struct {
	InstanceID string `db:"instance_id"`
}

// secretBackendUUID represents a secret backend UUID as referenced by a
// model-database secret value ref.
type secretBackendUUID struct {
	BackendUUID string `db:"backend_uuid"`
}

// revisionBackend pairs a secret revision UUID with the backend UUID its
// external value ref points at.
type revisionBackend struct {
	RevisionUUID string `db:"revision_uuid"`
	BackendUUID  string `db:"backend_uuid"`
}

// architectureName represents an architecture's name.
type architectureName struct {
	Name string `db:"name"`
}

// versionArg carries an agent binary version for use as a query argument.
type versionArg struct {
	Version string `db:"version"`
}

// agentVersionTarget represents the target agent version column from the
// agent_version table.
type agentVersionTarget struct {
	TargetVersion string `db:"target_version"`
}

// relationValidationRow is a relation's identity used to validate imported
// relation-unit consistency.
type relationValidationRow struct {
	UUID string `db:"uuid"`
	ID   int    `db:"relation_id"`
}

// modelCloudInfo is the model's identity and cloud placement, used to build the
// cloud spec for credential validation.
type modelCloudInfo struct {
	ControllerUUID string `db:"controller_uuid"`
	Type           string `db:"type"`
	CloudName      string `db:"cloud"`
	CloudType      string `db:"cloud_type"`
	Region         string `db:"cloud_region"`
}

// configKeyValue is one model configuration entry.
type configKeyValue struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

// machineUUIDArg carries a machine UUID for use as a query argument.
type machineUUIDArg struct {
	UUID string `db:"uuid"`
}

// applicationUnitRow pairs a unit's name with the application it belongs to.
type applicationUnitRow struct {
	ApplicationName string `db:"application_name"`
	UnitName        string `db:"unit_name"`
}

// relationEndpointRow is one endpoint of a relation: the application it belongs
// to, the charm endpoint name, the endpoint's scope and whether the
// application's charm is a subordinate.
type relationEndpointRow struct {
	RelationUUID    string `db:"relation_uuid"`
	ApplicationName string `db:"application_name"`
	EndpointName    string `db:"endpoint_name"`
	Scope           string `db:"scope"`
	Subordinate     bool   `db:"subordinate"`
}

// subordinateUnitPrincipalRow pairs a subordinate unit's name with the name of
// the application its principal unit belongs to.
type subordinateUnitPrincipalRow struct {
	UnitName        string `db:"unit_name"`
	ApplicationName string `db:"application_name"`
}

// relationUnitScopeRow identifies the application for one unit in scope of a
// relation, along with the relation it belongs to.
type relationUnitScopeRow struct {
	RelationUUID    string `db:"relation_uuid"`
	UnitName        string `db:"unit_name"`
	ApplicationName string `db:"application_name"`
}

// offererModel represents a distinct (offerer controller, offerer model) pair
// referenced by the model's remote applications.
type offererModel struct {
	ControllerUUID string `db:"offerer_controller_uuid"`
	ModelUUID      string `db:"offerer_model_uuid"`
}

// setAgentVersionTarget represents the set of update values required for
// setting the model's target agent version.
type setAgentVersionTarget struct {
	TargetVersion   string `db:"target_version"`
	PreviousVersion string `db:"previous_version"`
}
