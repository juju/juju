// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"strconv"
)

const (
	LOWEST = iota
	PCONDOR
	PCONDAND
	EQUALS
	LESSGREATER
	PPRODUCT
	CALL
	INDEX
)

var precedence = map[TokenType]int{
	CONDOR:   PCONDOR,
	CONDAND:  PCONDAND,
	EQ:       EQUALS,
	NEQ:      EQUALS,
	LPAREN:   CALL,
	LAMBDA:   CALL,
	LT:       LESSGREATER,
	LE:       LESSGREATER,
	GT:       LESSGREATER,
	GE:       LESSGREATER,
	LBRACKET: INDEX,
	PERIOD:   INDEX,
}

type Parser struct {
	lex *Lexer

	prevToken    Token
	currentToken Token
	peekToken    Token

	prefix map[TokenType]PrefixFunc
	infix  map[TokenType]InfixFunc
}

type PrefixFunc func() (Expression, error)
type InfixFunc func(Expression) (Expression, error)

// NewParser creates a parser for consuming a lexer tokens.
func NewParser(lex *Lexer) *Parser {
	p := &Parser{
		lex: lex,
	}
	p.prefix = map[TokenType]PrefixFunc{
		IDENT:      p.parseIdentifier,
		UNDERSCORE: p.parseIdentifier,
		INT:        p.parseInteger,
		FLOAT:      p.parseFloat,
		STRING:     p.parseString,
		LPAREN:     p.parseGroup,
		BOOL:       p.parseBool,
	}
	p.infix = map[TokenType]InfixFunc{
		EQ:       p.parseInfixExpression,
		NEQ:      p.parseInfixExpression,
		CONDAND:  p.parseInfixExpression,
		CONDOR:   p.parseInfixExpression,
		LT:       p.parseInfixExpression,
		LE:       p.parseInfixExpression,
		GT:       p.parseInfixExpression,
		GE:       p.parseInfixExpression,
		PERIOD:   p.parseAccessor,
		LBRACKET: p.parseIndex,
		LPAREN:   p.parseCall,
		LAMBDA:   p.parseLambda,
	}
	p.nextToken()
	p.nextToken()
	return p
}

// Run the parser to the end, which is either an EOF or an error.
func (p *Parser) Run() (*QueryExpression, error) {
	var exp QueryExpression
	for p.currentToken.Type != EOF {
		expr, err := p.parseExpressionStatement()
		if err != nil {
			return nil, err
		}
		exp.Expressions = append(exp.Expressions, expr)
		p.nextToken()
	}
	return &exp, nil
}

func (p *Parser) parseIdentifier() (Expression, error) {
	return &Identifier{
		Token: p.currentToken,
	}, nil
}

func (p *Parser) parseString() (Expression, error) {
	if !p.currentToken.Terminated {
		return nil, ErrSyntaxError(p.currentToken.Pos, p.currentToken.Type, STRING)
	}
	return &String{
		Token: p.currentToken,
	}, nil
}

func (p *Parser) parseBool() (Expression, error) {
	value, err := strconv.ParseBool(p.currentToken.Literal)
	if err != nil {
		return nil, ErrSyntaxError(p.currentToken.Pos, p.currentToken.Type, BOOL)
	}
	return &Bool{
		Token: p.currentToken,
		Value: value,
	}, nil
}

func (p *Parser) parseInteger() (Expression, error) {
	value, err := strconv.ParseInt(p.currentToken.Literal, 10, 64)
	if err != nil {
		return nil, ErrSyntaxError(p.currentToken.Pos, p.currentToken.Type, INT)
	}
	return &Integer{
		Token: p.currentToken,
		Value: value,
	}, nil
}

func (p *Parser) parseFloat() (Expression, error) {
	value, err := strconv.ParseFloat(p.currentToken.Literal, 64)
	if err != nil {
		return nil, ErrSyntaxError(p.currentToken.Pos, p.currentToken.Type, FLOAT)
	}
	return &Float{
		Token: p.currentToken,
		Value: value,
	}, nil
}

func (p *Parser) parseExpressionStatement() (Expression, error) {
	stmt := &ExpressionStatement{
		Token: p.currentToken,
	}
	expr, err := p.parseExpression(LOWEST)
	if err != nil {
		return nil, err
	}
	stmt.Expression = expr

	if p.isPeekToken(SEMICOLON) {
		p.nextToken()
	}
	return stmt, nil
}

func (p *Parser) parseExpression(precedence int) (Expression, error) {
	prefix := p.prefix[p.currentToken.Type]
	if prefix == nil {
		if p.currentToken.Type != EOF {
			return nil, ErrSyntaxError(p.currentToken.Pos, p.currentToken.Type)
		}
		return nil, nil
	}
	leftExp, err := prefix()
	if err != nil {
		return nil, err
	}
	// Run the infix function until the next token has
	// a higher precedence.
	for !p.isPeekToken(SEMICOLON) && precedence < p.peekPrecedence() {
		infix := p.infix[p.peekToken.Type]
		if infix == nil {
			return leftExp, nil
		}
		p.nextToken()
		if leftExp, err = infix(leftExp); err != nil {
			return nil, err
		}
	}

	return leftExp, nil
}

func (p *Parser) parseInfixExpression(left Expression) (Expression, error) {
	expr := &InfixExpression{
		Token:    p.currentToken,
		Operator: p.currentToken.Literal,
		Left:     left,
	}
	precedence := p.currentPrecedence()
	p.nextToken()
	right, err := p.parseExpression(precedence)
	if err != nil {
		return nil, err
	}

	expr.Right = right
	return expr, nil
}

func (p *Parser) parseGroup() (Expression, error) {
	p.nextToken()
	if p.currentToken.Type == LPAREN && p.isCurrentToken(RPAREN) {
		return &Empty{
			Token: p.currentToken,
		}, nil
	}

	exp, err := p.parseExpression(LOWEST)
	if err != nil {
		return nil, err
	}
	if err := p.expectPeek(RPAREN); err != nil {
		return nil, err
	}
	return exp, nil
}

func (p *Parser) parseIndex(left Expression) (Expression, error) {
	p.nextToken()

	index, err := p.parseExpression(LOWEST)
	if err != nil {
		return nil, err
	}
	expression := &IndexExpression{
		Token: p.currentToken,
		Left:  left,
		Index: index,
	}
	if err := p.expectPeek(RBRACKET); err != nil {
		return nil, err
	}
	return expression, nil
}

func (p *Parser) parseCall(left Expression) (Expression, error) {
	if p.isPeekToken(RPAREN) {
		currentToken := p.currentToken
		p.nextToken()
		return &CallExpression{
			Token: currentToken,
			Name:  left,
		}, nil
	}

	p.nextToken()

	first, err := p.parseExpression(LOWEST)
	if err != nil {
		return nil, err
	}
	arguments := []Expression{
		first,
	}
	for p.isPeekToken(COMMA) {
		p.nextToken()
		p.nextToken()

		next, err := p.parseExpression(LOWEST)
		if err != nil {
			return nil, err
		}
		arguments = append(arguments, next)
	}
	if err := p.expectPeek(RPAREN); err != nil {
		return nil, err
	}

	return &CallExpression{
		Token:     p.currentToken,
		Name:      left,
		Arguments: arguments,
	}, nil
}

func (p *Parser) parseLambda(left Expression) (Expression, error) {
	if p.isPeekToken(UNDERSCORE) {
		currentToken := p.currentToken
		p.nextToken()

		expr, err := p.parseExpression(LOWEST)
		if err != nil {
			return nil, err
		}

		return &LambdaExpression{
			Token:    currentToken,
			Argument: left,
			Expressions: []Expression{
				expr,
			},
		}, nil
	}

	p.nextToken()

	first, err := p.parseExpression(LOWEST)
	if err != nil {
		return nil, err
	}
	expressions := []Expression{
		first,
	}
	for p.isPeekToken(SEMICOLON) {
		p.nextToken()
		p.nextToken()
		next, err := p.parseExpression(LOWEST)
		if err != nil {
			return nil, err
		}
		expressions = append(expressions, next)
	}
	if !p.isPeekToken(EOF) && !p.isPeekToken(RPAREN) {
		return nil, p.expectPeek(RPAREN)
	}

	return &LambdaExpression{
		Token:       p.currentToken,
		Argument:    left,
		Expressions: expressions,
	}, nil
}

func (p *Parser) parseAccessor(left Expression) (Expression, error) {
	precedence := p.currentPrecedence()
	p.nextToken()
	right, err := p.parseExpression(precedence)
	if err != nil {
		return nil, err
	}

	return &AccessorExpression{
		Token: p.currentToken,
		Left:  left,
		Right: right,
	}, nil
}

func (p *Parser) currentPrecedence() int {
	if p, ok := precedence[p.currentToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) peekPrecedence() int {
	if p, ok := precedence[p.peekToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) isPeekToken(t TokenType) bool {
	return p.peekToken.Type == t
}

func (p *Parser) isCurrentToken(t TokenType) bool {
	return p.currentToken.Type == t
}

func (p *Parser) nextToken() {
	p.prevToken = p.currentToken
	p.currentToken = p.peekToken
	p.peekToken = p.lex.NextToken()
}

func (p *Parser) expectPeek(t TokenType) error {
	if p.isPeekToken(t) {
		p.nextToken()
		return nil
	}
	return ErrSyntaxError(p.currentToken.Pos, p.peekToken.Type, t)
}
