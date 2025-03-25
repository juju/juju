// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package assumes

import (
	coreerrors "github.com/juju/juju/core/errors"
	chassumes "github.com/juju/juju/internal/charm/assumes"
	"github.com/juju/juju/internal/errors"
)

// satisfyExpr checks whether the feature set contents satisfy the provided
// "assumes" expression. The function can process either feature or composite
// expressions.
func satisfyExpr(fs FeatureSet, expr chassumes.Expression, exprTreeDepth int) error {
	switch expr := expr.(type) {
	case chassumes.FeatureExpression:
		return satisfyFeatureExpr(fs, expr)
	case chassumes.CompositeExpression:
		return satisfyCompositeExpr(fs, expr, exprTreeDepth)
	default:
		return errors.Errorf("assumes expression type %q %w", expr.Type(), coreerrors.NotSupported)
	}
}

// satisfyExpr checks whether the feature set contents satisfy the provided
// "assumes" feature expression.
//
// The expression is matched if the feature set contains the required feature
// name and any of the following conditions is true:
// a) The feature set entry OR the assumes expression does not specify a version.
//  2. Both the feature set entry AND the assumes expression specify versions
//     AND the required version constraint (>= or <) is satisfied.
func satisfyFeatureExpr(fs FeatureSet, expr chassumes.FeatureExpression) error {
	supported, defined := fs.Get(expr.Name)
	if !defined {
		errStr := featureMissingErr(expr.Name)
		featDescr := UserFriendlyFeatureDescriptions[expr.Name]
		return featureError(expr.Name, featDescr, "%s", errStr)
	}

	// If the "assumes" feature expression does not specify a version or the
	// provided feature omits its version, then the expression is always
	// satisfied.
	if expr.Version == nil || supported.Version == nil {
		return nil
	}

	// Compare versions
	var satisfied bool
	switch expr.Constraint {
	case chassumes.VersionGTE:
		satisfied = supported.Version.Compare(*expr.Version) >= 0
	case chassumes.VersionLT:
		satisfied = supported.Version.Compare(*expr.Version) < 0
	}

	if satisfied {
		return nil
	}

	// Version mismatch. Get the nice error message.
	errStr := featureVersionMismatchErr(expr.Name, string(expr.Constraint), expr.Version.String(), supported.Version.String())

	var featDescr = supported.Description
	if featDescr == "" {
		// The feature set should always have a feature description.
		// Try the fallback descriptions if it is missing.
		featDescr = UserFriendlyFeatureDescriptions[featDescr]
	}
	return featureError(expr.Name, featDescr, "%s", errStr)
}

// satisfyCompositeExpr checks whether the feature set contents satisfy the
// provided "assumes" composite expression.
//
// For an any-of kind of expression, the sub-expression evaluation will be
// short-circuited when the first satisfied sub-expression is found. For all-of
// kind of expressions, all sub-expressions must be matched.
//
// If the expression cannot be satisfied, the function returns a multi-error
// value listing any detected conflicts.
func satisfyCompositeExpr(fs FeatureSet, expr chassumes.CompositeExpression, exprTreeDepth int) error {
	var errList = make([]error, 0, len(expr.SubExpressions))
	for _, subExpr := range expr.SubExpressions {
		err := satisfyExpr(fs, subExpr, exprTreeDepth+1)
		if err == nil && expr.ExprType == chassumes.AnyOfExpression {
			// We can short-circuit the check if this is an any-of
			// expression and we found a matching subexpression.
			return nil
		} else if err != nil {
			errList = append(errList, err)
		}
	}

	if len(errList) == 0 {
		return nil
	}

	// The root of the expression tree is always an implicit all-of
	// expression. To improve UX, we should avoid using the switch statement
	// below which introduces yet another indentation level and instead
	// emit a top-level descriptive message.
	if exprTreeDepth == 0 {
		return requirementsNotSatisfied("Charm cannot be deployed because:", errList)
	}

	switch expr.Type() {
	case chassumes.AllOfExpression:
		return notSatisfiedError("charm requires all of the following:", errList...)
	case chassumes.AnyOfExpression:
		return notSatisfiedError("charm requires at least one of the following:", errList...)
	default:
		return notSatisfiedError("charm requires "+string(expr.Type())+" of the following:", errList...)
	}
}
