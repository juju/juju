// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

import coresecrets "github.com/juju/juju/core/secrets"

// Role represents the role of a secret access grant
// as recorded in the secret_role lookup table.
type Role int

const (
	RoleNone Role = iota
	RoleView
	RoleManage
)

// MarshallRole converts a secret role to a db role id.
func MarshallRole(role coresecrets.SecretRole) Role {
	switch role {
	case coresecrets.RoleView:
		return RoleView
	case coresecrets.RoleManage:
		return RoleManage
	}
	return RoleNone
}

// GrantScopeType represents the type of a subject
// granted access to a secret as recorded in the
// secret_grant_subject_type lookup table.
type GrantScopeType int

const (
	ScopeUnit GrantScopeType = iota
	ScopeApplication
	ScopeModel
	ScopeRelation
)

// String implements fmt.Stringer.
func (s GrantScopeType) String() string {
	switch s {
	case ScopeUnit:
		return "unit"
	case ScopeApplication:
		return "application"
	case ScopeModel:
		return "model"
	case ScopeRelation:
		return "relation"
	}
	return ""
}

// GrantSubjectType represents the type of the
// scope of a secret access grant as recorded
// in the secret_grant_scope_type lookup table.
type GrantSubjectType int

const (
	SubjectUnit GrantSubjectType = iota
	SubjectApplication
	SubjectModel
	SubjectRemoteApplication
)

// String implements fmt.Stringer.
func (s GrantSubjectType) String() string {
	switch s {
	case SubjectUnit:
		return "unit"
	case SubjectApplication:
		return "application"
	case SubjectModel:
		return "model"
	case SubjectRemoteApplication:
		return "remote application"
	}
	return ""
}
