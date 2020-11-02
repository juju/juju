// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Lexer takes a query and breaks it down into tokens that can be consumed at
// at later date.
// The lexer in question is lazy and requires the calling of next to move it
// forward.
type Lexer struct {
	input []rune
	char  rune

	position     int
	readPosition int
	line         int
	column       int
}

// NewLexer creates a new Lexer from a given input.
func NewLexer(input string) *Lexer {
	lex := &Lexer{
		input:  []rune(input),
		char:   ' ',
		line:   1,
		column: 1,
	}

	return lex
}

// ReadNext will attempt to read the next character and correctly setup the
// positional values for the input.
func (l *Lexer) ReadNext() {
	if l.readPosition >= len(l.input) {
		l.char = 0
	} else {
		l.char = l.input[l.readPosition]
		if l.char == '\n' {
			l.column = 1
			l.line++
		} else {
			l.column++
		}
	}

	l.position = l.readPosition
	l.readPosition++
}

// Peek will attempt to read the next rune if it's available.
func (l *Lexer) Peek() rune {
	return l.PeekN(0)
}

// PeekN attempts to read the next rune by a given offset, it it's available.
func (l *Lexer) PeekN(n int) rune {
	if l.readPosition+n >= len(l.input) {
		return 0
	}
	return l.input[l.readPosition+n]
}

// NextToken attempts to grab the next token available.
func (l *Lexer) NextToken() Token {
	var tok Token
	l.skipWhitespace()

	pos := l.getPosition()
	pos.Column--

	if t, ok := tokenMap[l.char]; ok {
		switch t {
		case ASSIGN:
			if peek := l.Peek(); peek == '=' {
				tok = Token{
					Type:    EQ,
					Literal: string(l.char) + string(peek),
				}
				l.ReadNext()
			} else if peek == '>' {
				tok = Token{
					Type:    LAMBDA,
					Literal: string(l.char) + string(peek),
				}
				l.ReadNext()
			} else {
				tok = MakeToken(ASSIGN, l.char)
			}
		case BANG:
			if peek := l.Peek(); peek == '=' {
				tok = Token{
					Type:    NEQ,
					Literal: string(l.char) + string(peek),
				}
				l.ReadNext()
			} else {
				tok = MakeToken(BANG, l.char)
			}
		case BITAND:
			if peek := l.Peek(); peek == '&' {
				tok = Token{
					Type:    CONDAND,
					Literal: string(l.char) + string(peek),
				}
				l.ReadNext()
			} else {
				tok = MakeToken(BITAND, l.char)
			}
		case BITOR:
			if peek := l.Peek(); peek == '|' {
				tok = Token{
					Type:    CONDOR,
					Literal: string(l.char) + string(peek),
				}
				l.ReadNext()
			} else {
				tok = MakeToken(BITOR, l.char)
			}
		case LT:
			if peek := l.Peek(); peek == '=' {
				tok = Token{
					Type:    LE,
					Literal: string(l.char) + string(peek),
				}
				l.ReadNext()
			} else {
				tok = MakeToken(LT, l.char)
			}
		case GT:
			if peek := l.Peek(); peek == '=' {
				tok = Token{
					Type:    GE,
					Literal: string(l.char) + string(peek),
				}
				l.ReadNext()
			} else {
				tok = MakeToken(GT, l.char)
			}
		case UNDERSCORE:
			tok = MakeToken(UNDERSCORE, l.char)
		default:
			tok = MakeToken(t, l.char)
		}

		l.ReadNext()

		tok.Pos = pos
		return tok
	}

	newToken := l.readRunesToken()
	newToken.Pos = pos
	return newToken
}

func (l *Lexer) readRunesToken() Token {
	var tok Token
	switch {
	case l.char == 0:
		tok.Literal = ""
		tok.Type = EOF
		return tok
	case isDigit(l.char):
		literal := l.readNumber()
		if strings.Contains(literal, ".") {
			tok.Type = FLOAT
		} else {
			tok.Type = INT
		}
		tok.Literal = literal
		return tok
	case isLetter(l.char):
		tok.Literal = l.readIdentifier()
		switch strings.ToLower(tok.Literal) {
		case "true":
			tok.Type = TRUE
		case "false":
			tok.Type = FALSE
		case "_":
			tok.Type = UNDERSCORE
		default:
			tok.Type = IDENT
		}
		return tok
	case isQuote(l.char):
		if s, err := l.readString(l.char); err == nil {
			tok.Type = STRING
			tok.Literal = s
			return tok
		}
	}
	l.ReadNext()
	return MakeToken(UNKNOWN, l.char)
}

func (l *Lexer) skipWhitespace() {
	for unicode.IsSpace(l.char) {
		l.ReadNext()
	}
}

func (l *Lexer) readIdentifier() string {
	position := l.position
	for isLetter(l.char) || isDigit(l.char) || l.char == '-' {
		l.ReadNext()
	}
	return string(l.input[position:l.position])
}

func (l *Lexer) readString(r rune) (string, error) {
	var ret []rune

	for {
		l.ReadNext()
		switch l.char {
		case '\n':
			return "", errors.New("unexpected EOL")
		case 0:
			return "", errors.New("unexpected EOF")
		case r:
			l.ReadNext()
			return string(ret), nil
		default:
			ret = append(ret, l.char)
		}
	}
}

// scanNumber returns number beginning at current position.
func (l *Lexer) readNumber() string {
	var ret []rune

	ret = append(ret, l.char)
	l.ReadNext()

	for isDigit(l.char) || l.char == '.' {
		if l.char == '.' {
			if l.Peek() == '.' {
				return string(ret)
			}
		}

		ret = append(ret, l.char)
		l.ReadNext()
	}
	return string(ret)
}

func (l *Lexer) getPosition() Position {
	return Position{
		Offset: l.position,
		Line:   l.line,
		Column: l.column,
	}
}

func isLetter(char rune) bool {
	return 'a' <= char && char <= 'z' || 'A' <= char && char <= 'Z' || char == '_' || char >= utf8.RuneSelf && unicode.IsLetter(char)
}

func isDigit(char rune) bool {
	return '0' <= char && char <= '9' || char >= utf8.RuneSelf && unicode.IsDigit(char)
}

func isQuote(char rune) bool {
	return char == 34
}
