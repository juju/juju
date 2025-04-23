// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state provides the state methods used by the relation service
// for its domain.
//
// Interactions with the relations database schema:
//
// When an application is deployed, rows are created in the `application_endpoint`
// table to link an application, the charm's relation and a space together.
//
// During deployment of an application with a peer relation, a row is added to
// the `relation` and `relation_endpoint` tables. A peer relation is between
// the units of a single application and not added by a user.
//
// With any new row in the `relation` table, a sequential ID is assigned using
// the `sequence` table. The sequence is unique to a model. No relation
// ID is reused in the lifetime of a model. This ID is used by the units to
// identify the relation.
//
// When a user adds a relation, a row is added to the `relation` table.
// The relation and application endpoints are linked via new rows in the
// `relation_endpoint` table. A row in the `relation_status` table is
// created. Provided settings are added to the `application_setting` table.
//
// New relations must fit the following criteria:
//   - both applications must be alive
//   - both endpoints must satisfy `CanRelateTo()`
//   - if one endpoint is ContainerScoped, so must the other be
//   - if the endpoints are ContainerScoped, one application must be subordinate
//   - enforce the MaxRelationLimit for both applications
//
// A `relation_unit` row is created when a unit indicates it has joined the
// relation by entering scope. Entering and leaving scope must be idempotent.
// If the scope has not changed, the settings may be replaced.
//
// If a unit enters scope where the relation has container scope, a
// subordinate unit will also be created.
//
// A relation unit stores and retrieves its own settings, a key value pair,
// in the `relation_unit_settings` table.
//
// The application leader stores and retrieves settings for the application in
// `relation_application_settings`.
//
// A relation has a status kept in the relation_status table. Status is set
// by the application leader. Status types are defined in the
// `relation_status_type` table.
//
// Each relation has a life status: alive, dying and dead. A relation cannot
// be alive if both of its applications are also not alive. When set to dying
// <insert>. When set to dead <insert>. A relation cannot be dead until all
// relation units have left scope.
//
// The relation sequence is migrated in the sequence domain.
//
// Relation statuses are migrated in the status domain.
// TODO:
// * leave scope details
// * more on Life
package state
