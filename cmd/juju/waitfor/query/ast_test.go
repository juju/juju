// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	gc "gopkg.in/check.v1"
)

type astSuite struct{}

var _ = gc.Suite(&astSuite{})

func (p *astSuite) TestQueryExpressionString(c *gc.C) {
	exp := &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &Identifier{
					Token: Token{
						Literal: "abc",
					},
				},
			},
			&ExpressionStatement{
				Expression: &Identifier{
					Token: Token{
						Literal: "efg",
					},
				},
			},
		},
	}
	c.Assert(exp.String(), gc.DeepEquals, "abc;efg;")
}

func (p *astSuite) TestExpressionStatementEmptyString(c *gc.C) {
	exp := &ExpressionStatement{
		Expression: &Identifier{
			Token: Token{
				Literal: "",
			},
		},
	}
	c.Assert(exp.String(), gc.DeepEquals, ";")
}

func (p *astSuite) TestExpressionStatementString(c *gc.C) {
	exp := &ExpressionStatement{
		Expression: &Identifier{
			Token: Token{
				Literal: "abc",
			},
		},
	}
	c.Assert(exp.String(), gc.DeepEquals, "abc;")
}

func (p *astSuite) TestInfixExpressionString(c *gc.C) {
	exp := &InfixExpression{
		Left: &Identifier{
			Token: Token{
				Literal: "abc",
			},
		},
		Operator: "&&",
		Right: &Identifier{
			Token: Token{
				Literal: "efg",
			},
		},
	}
	c.Assert(exp.String(), gc.DeepEquals, "(abc && efg)")
}

func (p *astSuite) TestIdentifierString(c *gc.C) {
	exp := &Identifier{
		Token: Token{
			Literal: "abc",
		},
	}
	c.Assert(exp.String(), gc.DeepEquals, "abc")
}

func (p *astSuite) TestEmptyString(c *gc.C) {
	exp := &Empty{
		Token: Token{
			Literal: "abc",
		},
	}
	c.Assert(exp.String(), gc.DeepEquals, "()")
}

func (p *astSuite) TestIntegerString(c *gc.C) {
	exp := &Integer{
		Token: Token{
			Literal: "1",
		},
	}
	c.Assert(exp.String(), gc.DeepEquals, "1")
}

func (p *astSuite) TestFloatString(c *gc.C) {
	exp := &Float{
		Token: Token{
			Literal: "1.123",
		},
	}
	c.Assert(exp.String(), gc.DeepEquals, "1.123")
}

func (p *astSuite) TestBoolString(c *gc.C) {
	exp := &Bool{
		Token: Token{
			Literal: "true",
		},
	}
	c.Assert(exp.String(), gc.DeepEquals, "true")
}
