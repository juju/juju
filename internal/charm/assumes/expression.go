// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package assumes

import "github.com/juju/juju/core/semversion"

var (
	_ Expression = (*FeatureExpression)(nil)
	_ Expression = (*CompositeExpression)(nil)
)

// ExpressionType represents the type of an assumes expression.
type ExpressionType string

const (
	AnyOfExpression ExpressionType = "any-of"
	AllOfExpression ExpressionType = "all-of"
)

// Expression is an interface implemented by all expression types in this package.
type Expression interface {
	Type() ExpressionType
}

// VersionConstraint describes a constraint for required feature versions.
type VersionConstraint string

const (
	VersionGTE VersionConstraint = ">="
	VersionLT  VersionConstraint = "<"
)

// FeatureExpression describes a feature that is required by the charm in order
// to be successfully deployed. Feature expressions may additionally specify a
// version constraint.
type FeatureExpression struct {
	// The name of the featureflag.
	Name string

	// A feature within an assumes block may optionally specify a version
	// constraint.
	Constraint VersionConstraint
	Version    *semversion.Number

	// The raw, unprocessed version string for serialization purposes.
	rawVersion string
}

// Type implements Expression.
func (FeatureExpression) Type() ExpressionType { return ExpressionType("feature") }

// CompositeExpression describes a composite expression that applies some
// operator to a sub-expression list.
type CompositeExpression struct {
	ExprType       ExpressionType
	SubExpressions []Expression
}

// Type implements Expression.
func (expr CompositeExpression) Type() ExpressionType { return expr.ExprType }
