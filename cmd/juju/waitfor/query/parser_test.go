// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"bufio"
	"os"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type parserSuite struct{}

var _ = gc.Suite(&parserSuite{})

func (p *parserSuite) TestParserMultipleExpressions(c *gc.C) {
	query := `life; abc;`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exp, gc.DeepEquals, &QueryExpression{
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
			&ExpressionStatement{
				Expression: &Identifier{
					Token: Token{
						Pos:     Position{Line: 1, Column: 7, Offset: 6},
						Type:    IDENT,
						Literal: "abc",
					},
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 7, Offset: 6},
					Type:    IDENT,
					Literal: "abc",
				},
			},
		},
	})
}

func (p *parserSuite) TestParserIdent(c *gc.C) {
	query := `life`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exp, gc.DeepEquals, &QueryExpression{
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
	})
}

func (p *parserSuite) TestParserString(c *gc.C) {
	query := `"abc"`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exp, gc.DeepEquals, &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &String{
					Token: Token{
						Pos:        Position{Line: 1, Column: 1, Offset: 0},
						Type:       STRING,
						Literal:    "abc",
						Terminated: true,
					},
				},
				Token: Token{
					Pos:        Position{Line: 1, Column: 1, Offset: 0},
					Type:       STRING,
					Literal:    "abc",
					Terminated: true,
				},
			},
		},
	})
}

func (p *parserSuite) TestParserInteger(c *gc.C) {
	query := `1`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exp, gc.DeepEquals, &QueryExpression{
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
	})
}

func (p *parserSuite) TestParserFloat(c *gc.C) {
	query := `1.1`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exp, gc.DeepEquals, &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &Float{
					Token: Token{
						Pos:     Position{Line: 1, Column: 1, Offset: 0},
						Type:    FLOAT,
						Literal: "1.1",
					},
					Value: 1.1,
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    FLOAT,
					Literal: "1.1",
				},
			},
		},
	})
}

func (p *parserSuite) TestParserBool(c *gc.C) {
	query := `true false`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exp, gc.DeepEquals, &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &Bool{
					Token: Token{
						Pos:     Position{Line: 1, Column: 1, Offset: 0},
						Type:    BOOL,
						Literal: "true",
					},
					Value: true,
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    BOOL,
					Literal: "true",
				},
			},
			&ExpressionStatement{
				Expression: &Bool{
					Token: Token{
						Pos:     Position{Line: 1, Column: 6, Offset: 5},
						Type:    BOOL,
						Literal: "false",
					},
					Value: false,
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 6, Offset: 5},
					Type:    BOOL,
					Literal: "false",
				},
			},
		},
	})
}

func (p *parserSuite) TestParserGroup(c *gc.C) {
	query := `(abc)`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exp, gc.DeepEquals, &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &Identifier{
					Token: Token{
						Pos:     Position{Line: 1, Column: 2, Offset: 1},
						Type:    IDENT,
						Literal: "abc",
					},
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    LPAREN,
					Literal: "(",
				},
			},
		},
	})
}

func (p *parserSuite) TestParserInfixLogicalAND(c *gc.C) {
	query := `true && true`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exp, gc.DeepEquals, &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &InfixExpression{
					Left: &Bool{
						Token: Token{
							Pos:     Position{Line: 1, Column: 1, Offset: 0},
							Type:    BOOL,
							Literal: "true",
						},
						Value: true,
					},
					Operator: "&&",
					Right: &Bool{
						Token: Token{
							Pos:     Position{Line: 1, Column: 9, Offset: 8},
							Type:    BOOL,
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
					Type:    BOOL,
					Literal: "true",
				},
			},
		},
	})
}

func (p *parserSuite) TestParserInfixLogicalOR(c *gc.C) {
	query := `true || true`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exp, gc.DeepEquals, &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &InfixExpression{
					Left: &Bool{
						Token: Token{
							Pos:     Position{Line: 1, Column: 1, Offset: 0},
							Type:    BOOL,
							Literal: "true",
						},
						Value: true,
					},
					Operator: "||",
					Right: &Bool{
						Token: Token{
							Pos:     Position{Line: 1, Column: 9, Offset: 8},
							Type:    BOOL,
							Literal: "true",
						},
						Value: true,
					},
					Token: Token{
						Pos:     Position{Line: 1, Column: 6, Offset: 5},
						Type:    CONDOR,
						Literal: "||",
					},
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    BOOL,
					Literal: "true",
				},
			},
		},
	})
}

func (p *parserSuite) TestParserInfixLambda(c *gc.C) {
	query := `_ => _`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exp, gc.DeepEquals, &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &LambdaExpression{
					Argument: &Identifier{
						Token: Token{
							Pos:     Position{Line: 1, Column: 1, Offset: 0},
							Type:    UNDERSCORE,
							Literal: "_",
						},
					},
					Expressions: []Expression{
						&Identifier{
							Token: Token{
								Pos:     Position{Line: 1, Column: 6, Offset: 5},
								Type:    UNDERSCORE,
								Literal: "_",
							},
						},
					},
					Token: Token{
						Pos:     Position{Line: 1, Column: 3, Offset: 2},
						Type:    LAMBDA,
						Literal: "=>",
					},
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    UNDERSCORE,
					Literal: "_",
				},
			},
		},
	})
}

func FuzzLexerParser(f *testing.F) {
	readCorpus(f)

	f.Fuzz(func(t *testing.T, value string) {
		lex := NewLexer(value)
		parser := NewParser(lex)
		_, _ = parser.Run()
	})
}

func readCorpus(f *testing.F) {
	f.Helper()

	file, err := os.Open("./testfiles/success")
	if err != nil {
		f.Fatalf("unable to read file")
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		t := scanner.Text()
		if t == "" {
			continue
		}
		f.Add(t)
	}
	if err := scanner.Err(); err != nil {
		f.Fatal(err)
	}
}
