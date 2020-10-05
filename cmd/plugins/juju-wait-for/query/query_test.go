// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type querySuite struct{}

var _ = gc.Suite(&querySuite{})

func (s *querySuite) TestQueryScope(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	scope := NewMockScope(ctrl)
	scope.EXPECT().GetIdentValue("life").Return("alive", nil).Times(2)

	src := `life == "death" || life == "alive"`

	query, err := Parse(src)
	c.Assert(err, jc.ErrorIsNil)

	done, err := query.Run(scope)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(done, jc.IsTrue)
}

func (s *querySuite) TestRunIdent(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	scope := NewMockScope(ctrl)
	scope.EXPECT().GetIdentValue("life").Return("alive", nil)

	exp := &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &Identifier{
					Token: Token{
						Pos:     Position{Line: 1, Column: 1, Offset: 0},
						Type:    IDENT,
						Literal: "life",
					},
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    IDENT,
					Literal: "life",
				},
			},
		},
	}

	var query Query
	result, err := query.run(exp, scope)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, "alive")
}

func (s *querySuite) TestRunString(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	scope := NewMockScope(ctrl)

	exp := &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &String{
					Token: Token{
						Pos:     Position{Line: 1, Column: 1, Offset: 0},
						Type:    STRING,
						Literal: "abc",
					},
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    STRING,
					Literal: "abc",
				},
			},
		},
	}

	var query Query
	result, err := query.run(exp, scope)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, "abc")
}

func (s *querySuite) TestRunInteger(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	scope := NewMockScope(ctrl)

	exp := &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &Integer{
					Token: Token{
						Pos:     Position{Line: 1, Column: 1, Offset: 0},
						Type:    INT,
						Literal: "1",
					},
					Value: 1,
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    INT,
					Literal: "1",
				},
			},
		},
	}

	var query Query
	result, err := query.run(exp, scope)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, int64(1))
}

func (s *querySuite) TestRunFloat(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	scope := NewMockScope(ctrl)

	exp := &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &Float{
					Token: Token{
						Pos:     Position{Line: 1, Column: 1, Offset: 0},
						Type:    FLOAT,
						Literal: "1.12",
					},
					Value: 1.12,
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    FLOAT,
					Literal: "1.12",
				},
			},
		},
	}

	var query Query
	result, err := query.run(exp, scope)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, float64(1.12))
}

func (s *querySuite) TestRunBool(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	scope := NewMockScope(ctrl)

	exp := &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &Bool{
					Token: Token{
						Pos:     Position{Line: 1, Column: 1, Offset: 0},
						Type:    TRUE,
						Literal: "true",
					},
					Value: true,
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    TRUE,
					Literal: "true",
				},
			},
		},
	}

	var query Query
	result, err := query.run(exp, scope)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, true)
}

func (s *querySuite) TestRunInfixLogicalAND(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	scope := NewMockScope(ctrl)

	exp := &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &InfixExpression{
					Left: &Bool{
						Token: Token{
							Pos:     Position{Line: 1, Column: 1, Offset: 0},
							Type:    TRUE,
							Literal: "true",
						},
						Value: true,
					},
					Operator: "&&",
					Right: &Bool{
						Token: Token{
							Pos:     Position{Line: 1, Column: 9, Offset: 8},
							Type:    TRUE,
							Literal: "true",
						},
						Value: true,
					},
					Token: Token{
						Pos:     Position{Line: 1, Column: 6, Offset: 5},
						Type:    CONDAND,
						Literal: "&&",
					},
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    TRUE,
					Literal: "true",
				},
			},
		},
	}

	var query Query
	result, err := query.run(exp, scope)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, true)
}

func (s *querySuite) TestRunInfixLogicalOR(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	scope := NewMockScope(ctrl)

	exp := &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &InfixExpression{
					Left: &Bool{
						Token: Token{
							Pos:     Position{Line: 1, Column: 1, Offset: 0},
							Type:    TRUE,
							Literal: "true",
						},
						Value: true,
					},
					Operator: "||",
					Right: &Bool{
						Token: Token{
							Pos:     Position{Line: 1, Column: 9, Offset: 8},
							Type:    TRUE,
							Literal: "false",
						},
						Value: false,
					},
					Token: Token{
						Pos:     Position{Line: 1, Column: 6, Offset: 5},
						Type:    CONDOR,
						Literal: "||",
					},
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    TRUE,
					Literal: "true",
				},
			},
		},
	}

	var query Query
	result, err := query.run(exp, scope)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, true)
}
