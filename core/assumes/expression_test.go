// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package assumes

import (
	"testing"

	"github.com/juju/tc"

	chassumes "github.com/juju/juju/domain/deployment/charm/assumes"
)

type expressionSuite struct{}

func TestExpressionSuite(t *testing.T) {
	tc.Run(t, &expressionSuite{})
}

func (s *expressionSuite) TestHasFeature(c *tc.C) {
	tree := &chassumes.ExpressionTree{
		Expression: chassumes.CompositeExpression{
			ExprType: chassumes.AllOfExpression,
			SubExpressions: []chassumes.Expression{
				chassumes.FeatureExpression{Name: "juju"},
				chassumes.CompositeExpression{
					ExprType: chassumes.AnyOfExpression,
					SubExpressions: []chassumes.Expression{
						chassumes.FeatureExpression{Name: "k8s-api"},
						chassumes.FeatureExpression{Name: "unitless"},
					},
				},
			},
		},
	}

	c.Check(HasFeature(tree, "unitless"), tc.IsTrue)
	c.Check(HasFeature(tree, "storage"), tc.IsFalse)
	c.Check(HasFeature(nil, "unitless"), tc.IsFalse)
}
