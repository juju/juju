// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package assumes

import chassumes "github.com/juju/juju/domain/deployment/charm/assumes"

// HasFeature reports whether an assumes expression tree contains the named
// feature.
func HasFeature(tree *chassumes.ExpressionTree, featureName string) bool {
	if tree == nil {
		return false
	}
	return expressionHasFeature(tree.Expression, featureName)
}

func expressionHasFeature(expr chassumes.Expression, featureName string) bool {
	switch expr := expr.(type) {
	case chassumes.FeatureExpression:
		return expr.Name == featureName
	case *chassumes.FeatureExpression:
		return expr.Name == featureName
	case chassumes.CompositeExpression:
		for _, subExpr := range expr.SubExpressions {
			if expressionHasFeature(subExpr, featureName) {
				return true
			}
		}
	case *chassumes.CompositeExpression:
		for _, subExpr := range expr.SubExpressions {
			if expressionHasFeature(subExpr, featureName) {
				return true
			}
		}
	}
	return false
}
