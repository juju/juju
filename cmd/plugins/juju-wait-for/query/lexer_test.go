// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	gc "gopkg.in/check.v1"
)

type lexerSuite struct{}

var _ = gc.Suite(&lexerSuite{})

func (p *lexerSuite) TestReadNext(c *gc.C) {
	expected := []Token{
		{
			Type: -1,
		},
		{
			Pos:     Position{Offset: 0, Line: 1, Column: 1},
			Type:    IDENT,
			Literal: "a",
		},
		{
			Pos:     Position{Offset: 2, Line: 1, Column: 3},
			Type:    IDENT,
			Literal: "b",
		},
		{
			Pos:     Position{Offset: 4, Line: 1, Column: 5},
			Type:    INT,
			Literal: "1",
		},
		{
			Pos:     Position{Offset: 6, Line: 1, Column: 7},
			Type:    ASSIGN,
			Literal: "=",
		},
		{
			Pos:     Position{Offset: 8, Line: 1, Column: 9},
			Type:    FLOAT,
			Literal: "2.1",
		},
		{
			Pos:     Position{Offset: 12, Line: 1, Column: 13},
			Type:    BANG,
			Literal: "!",
		},
		{
			Pos:     Position{Offset: 14, Line: 1, Column: 15},
			Type:    COMMA,
			Literal: ",",
		},
		{
			Pos:     Position{Offset: 16, Line: 1, Column: 17},
			Type:    SEMICOLON,
			Literal: ";",
		},
		{
			Pos:     Position{Offset: 18, Line: 1, Column: 19},
			Type:    LPAREN,
			Literal: "(",
		},
		{
			Pos:     Position{Offset: 20, Line: 1, Column: 21},
			Type:    RPAREN,
			Literal: ")",
		},
		{
			Pos:     Position{Offset: 22, Line: 1, Column: 23},
			Type:    BITAND,
			Literal: "&",
		},
		{
			Pos:     Position{Offset: 24, Line: 1, Column: 25},
			Type:    BITOR,
			Literal: "|",
		},
		{
			Pos:     Position{Offset: 26, Line: 1, Column: 27},
			Type:    17,
			Literal: "[",
		},
		{
			Pos:     Position{Offset: 28, Line: 1, Column: 29},
			Type:    18,
			Literal: "]",
		},
	}

	lex := NewLexer(`a b 1 = 2.1 ! , ; ( ) & | [ ]`)

	tok := UnknownToken
	var got []Token
	for ; tok.Type != EOF; tok = lex.NextToken() {
		got = append(got, tok)
	}

	c.Assert(got, gc.DeepEquals, expected)
}

func (p *lexerSuite) TestReadNextComplexTypes(c *gc.C) {
	tests := []struct {
		Input    string
		Expected []Token
	}{{
		Input: "==",
		Expected: []Token{{
			Type: -1,
		}, {
			Pos:     Position{Offset: 0, Line: 1, Column: 1},
			Type:    EQ,
			Literal: "==",
		}},
	}, {
		Input: "!=",
		Expected: []Token{{
			Type: -1,
		}, {
			Pos:     Position{Offset: 0, Line: 1, Column: 1},
			Type:    NEQ,
			Literal: "!=",
		}},
	}, {
		Input: "&&",
		Expected: []Token{{
			Type: -1,
		}, {
			Pos:     Position{Offset: 0, Line: 1, Column: 1},
			Type:    CONDAND,
			Literal: "&&",
		}},
	}, {
		Input: "||",
		Expected: []Token{{
			Type: -1,
		}, {
			Pos:     Position{Offset: 0, Line: 1, Column: 1},
			Type:    CONDOR,
			Literal: "||",
		}},
	}}

	for _, test := range tests {
		lex := NewLexer(test.Input)

		tok := UnknownToken
		var got []Token
		for ; tok.Type != EOF; tok = lex.NextToken() {
			got = append(got, tok)
		}

		c.Assert(got, gc.DeepEquals, test.Expected)
	}
}

func (p *lexerSuite) TestReadNextBool(c *gc.C) {
	tests := []struct {
		Input    string
		Expected []Token
	}{{
		Input: "true",
		Expected: []Token{{
			Type: -1,
		}, {
			Pos:     Position{Offset: 0, Line: 1, Column: 1},
			Type:    TRUE,
			Literal: "true",
		}},
	}, {
		Input: "false",
		Expected: []Token{{
			Type: -1,
		}, {
			Pos:     Position{Offset: 0, Line: 1, Column: 1},
			Type:    FALSE,
			Literal: "false",
		}},
	}}

	for _, test := range tests {
		lex := NewLexer(test.Input)

		tok := UnknownToken
		var got []Token
		for ; tok.Type != EOF; tok = lex.NextToken() {
			got = append(got, tok)
		}

		c.Assert(got, gc.DeepEquals, test.Expected)
	}
}

func (p *lexerSuite) TestReadNextIdent(c *gc.C) {
	tests := []struct {
		Input    string
		Expected []Token
	}{{
		Input: `a`,
		Expected: []Token{{
			Type: -1,
		}, {
			Pos:     Position{Offset: 0, Line: 1, Column: 1},
			Type:    IDENT,
			Literal: `a`,
		}},
	}, {
		Input: `abc`,
		Expected: []Token{{
			Type: -1,
		}, {
			Pos:     Position{Offset: 0, Line: 1, Column: 1},
			Type:    IDENT,
			Literal: "abc",
		}},
	}, {
		Input: `abc with a space`,
		Expected: []Token{{
			Type: -1,
		}, {
			Pos:     Position{Offset: 0, Line: 1, Column: 1},
			Type:    IDENT,
			Literal: "abc",
		}, {
			Pos:     Position{Offset: 4, Line: 1, Column: 5},
			Type:    IDENT,
			Literal: "with",
		}, {
			Pos:     Position{Offset: 9, Line: 1, Column: 10},
			Type:    IDENT,
			Literal: "a",
		}, {
			Pos:     Position{Offset: 11, Line: 1, Column: 12},
			Type:    IDENT,
			Literal: "space",
		}},
	}}

	for _, test := range tests {
		lex := NewLexer(test.Input)

		tok := UnknownToken
		var got []Token
		for ; tok.Type != EOF; tok = lex.NextToken() {
			got = append(got, tok)
		}

		c.Assert(got, gc.DeepEquals, test.Expected)
	}
}

func (p *lexerSuite) TestReadNextString(c *gc.C) {
	tests := []struct {
		Input    string
		Expected []Token
	}{{
		Input: `""`,
		Expected: []Token{{
			Type: -1,
		}, {
			Pos:     Position{Offset: 0, Line: 1, Column: 1},
			Type:    STRING,
			Literal: ``,
		}},
	}, {
		Input: `"abc"`,
		Expected: []Token{{
			Type: -1,
		}, {
			Pos:     Position{Offset: 0, Line: 1, Column: 1},
			Type:    STRING,
			Literal: "abc",
		}},
	}, {
		Input: `"abc with a space"`,
		Expected: []Token{{
			Type: -1,
		}, {
			Pos:     Position{Offset: 0, Line: 1, Column: 1},
			Type:    STRING,
			Literal: "abc with a space",
		}},
	}}

	for _, test := range tests {
		lex := NewLexer(test.Input)

		tok := UnknownToken
		var got []Token
		for ; tok.Type != EOF; tok = lex.NextToken() {
			got = append(got, tok)
		}

		c.Assert(got, gc.DeepEquals, test.Expected)
	}
}

func (p *lexerSuite) TestReadNextNumber(c *gc.C) {
	tests := []struct {
		Input    string
		Expected []Token
	}{{
		Input: `1234567890`,
		Expected: []Token{{
			Type: -1,
		}, {
			Pos:     Position{Offset: 0, Line: 1, Column: 1},
			Type:    INT,
			Literal: `1234567890`,
		}},
	}, {
		Input: `1.000002`,
		Expected: []Token{{
			Type: -1,
		}, {
			Pos:     Position{Offset: 0, Line: 1, Column: 1},
			Type:    FLOAT,
			Literal: "1.000002",
		}},
	}, {
		Input: `20.000002`,
		Expected: []Token{{
			Type: -1,
		}, {
			Pos:     Position{Offset: 0, Line: 1, Column: 1},
			Type:    FLOAT,
			Literal: "20.000002",
		}},
	}, {
		Input: `0.000002`,
		Expected: []Token{{
			Type: -1,
		}, {
			Pos:     Position{Offset: 0, Line: 1, Column: 1},
			Type:    FLOAT,
			Literal: "0.000002",
		}},
	}}

	for _, test := range tests {
		lex := NewLexer(test.Input)

		tok := UnknownToken
		var got []Token
		for ; tok.Type != EOF; tok = lex.NextToken() {
			got = append(got, tok)
		}

		c.Assert(got, gc.DeepEquals, test.Expected)
	}
}
