// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import "fmt"

// TokenType represents a way to identify an individual token.
type TokenType int

const (
	UNKNOWN TokenType = (iota - 1)
	EOF

	IDENT
	INT   //int literal
	FLOAT //float literal
	STRING

	EQ     // ==
	NEQ    // !=
	ASSIGN // =
	BANG   // !

	LT // <
	LE // <=
	GT // >
	GE // >=

	COMMA     // ,
	SEMICOLON // ;

	LPAREN   // (
	RPAREN   // )
	LBRACKET // [
	RBRACKET // ]

	BITAND  // &
	BITOR   // |
	CONDAND // &&
	CONDOR  // ||
	TRUE    // TRUE
	FALSE   // FALSE

	LAMBDA     // =>
	UNDERSCORE // _
	PERIOD     // .
)

func (t TokenType) String() string {
	switch t {
	case EOF:
		return "EOF"
	case IDENT:
		return "IDENT"
	case INT:
		return "INT"
	case FLOAT:
		return "FLOAT"
	case ASSIGN:
		return "="
	case BANG:
		return "!"
	case EQ:
		return "=="
	case NEQ:
		return "!="
	case LT:
		return "<"
	case LE:
		return "<="
	case GT:
		return ">"
	case GE:
		return ">="
	case LAMBDA:
		return "=>"
	case UNDERSCORE:
		return "_"
	case PERIOD:
		return "."
	case COMMA:
		return ","
	case SEMICOLON:
		return ";"
	case LPAREN:
		return "("
	case RPAREN:
		return ")"
	case LBRACKET:
		return "["
	case RBRACKET:
		return "]"
	case BITAND:
		return "&"
	case BITOR:
		return "|"
	case CONDAND:
		return "&&"
	case CONDOR:
		return "||"
	case STRING:
		return `""`
	case TRUE:
		return "true"
	case FALSE:
		return "false"
	default:
		return "<UNKNOWN>"
	}
}

// Position holds the location of the token within the query.
type Position struct {
	Offset int
	Line   int
	Column int
}

func (p Position) String() string {
	return fmt.Sprintf("<:%d:%d>", p.Line, p.Column)
}

// Token defines a token found with in a query, along with the position and what
// type it is.
type Token struct {
	Pos     Position
	Type    TokenType
	Literal string
}

// MakeToken creates a new token value.
func MakeToken(tokenType TokenType, char rune) Token {
	return Token{
		Type:    tokenType,
		Literal: string(char),
	}
}

var (
	// UnknownToken can be used as a sentinel token for a unknown state.
	UnknownToken = Token{
		Type: UNKNOWN,
	}
)

var tokenMap = map[rune]TokenType{
	'=': ASSIGN,
	';': SEMICOLON,
	'(': LPAREN,
	')': RPAREN,
	'[': LBRACKET,
	']': RBRACKET,
	',': COMMA,
	'!': BANG,
	'&': BITAND,
	'|': BITOR,
	'<': LT,
	'>': GT,
	'_': UNDERSCORE,
	'.': PERIOD,
}
