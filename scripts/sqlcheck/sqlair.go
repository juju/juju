// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// replacement is a span of SQL to substitute with a dummy token.
type replacement struct {
	start int
	end   int
	token string
}

// replaceSQLair rewrites sqlair input/output expressions into SQLite-safe
// dummy tokens while padding to the original width.
//
// Input expressions become "1" rather than "?" because pinned sqlfluff's
// sqlite dialect cannot parse "?" placeholders.
func replaceSQLair(ev *evaluated) *evaluated {
	if ev == nil || ev.Content == "" {
		return ev
	}
	reps := findSQLair(ev.Content)
	if len(reps) == 0 {
		return ev
	}
	return applyReplacements(ev, reps)
}

func applyReplacements(ev *evaluated, reps []replacement) *evaluated {
	if len(reps) == 0 {
		return ev
	}
	var b strings.Builder
	var m []origByte
	b.Grow(len(ev.Content))
	pos := 0
	for _, r := range reps {
		if r.start < pos || r.end > len(ev.Content) || r.start >= r.end {
			continue
		}
		if r.start > pos {
			b.WriteString(ev.Content[pos:r.start])
			m = append(m, ev.Map[pos:r.start]...)
		}
		orig := origByte{}
		if r.start < len(ev.Map) {
			orig = ev.Map[r.start]
		}
		// Replacements are not padded back to the original expression
		// width: padding would violate LT01 spacing rules wherever a
		// sqlair expression is followed by more SQL on the same line.
		// Column mapping does not depend on padding because every
		// segment carries its own original Go position.
		for i := 0; i < len(r.token); i++ {
			b.WriteByte(r.token[i])
			ob := orig
			ob.Kind = KindReplacement
			m = append(m, ob)
		}
		pos = r.end
	}
	if pos < len(ev.Content) {
		b.WriteString(ev.Content[pos:])
		m = append(m, ev.Map[pos:]...)
	}
	return &evaluated{Content: b.String(), Map: m}
}

func findSQLair(sql string) []replacement {
	p := &scn{s: sql}
	var reps []replacement
	for {
		if !p.advanceToNext() {
			break
		}
		start := p.pos
		if out, ok := p.parseOutput(); ok {
			reps = append(reps, out...)
			continue
		}
		p.pos = start
		if in, ok := p.parseInput(); ok {
			reps = append(reps, in...)
			continue
		}
		p.pos = start
		p.advanceByte()
	}
	return mergeReplacements(reps)
}

func replaceSQLPlaceholders(ev *evaluated) *evaluated {
	if ev == nil || ev.Content == "" {
		return ev
	}
	p := &scn{s: ev.Content}
	var reps []replacement
	for !p.atEnd() {
		if p.skipQuoted() {
			continue
		}
		if p.skipComment() {
			continue
		}
		if p.peek() == '?' {
			start := p.pos
			p.pos++
			for !p.atEnd() && p.peek() >= '0' && p.peek() <= '9' {
				p.pos++
			}
			reps = append(reps, replacement{start: start, end: p.pos, token: "1"})
			continue
		}
		if p.peek() == '$' && p.pos+1 < len(p.s) && p.s[p.pos+1] >= '0' && p.s[p.pos+1] <= '9' {
			start := p.pos
			p.pos++
			for !p.atEnd() && p.peek() >= '0' && p.peek() <= '9' {
				p.pos++
			}
			reps = append(reps, replacement{start: start, end: p.pos, token: "1"})
			continue
		}
		p.advanceByte()
	}
	if len(reps) == 0 {
		return ev
	}
	return applyReplacements(ev, reps)
}

func toSQLite(ev *evaluated) *evaluated {
	return replaceSQLPlaceholders(replaceSQLair(ev))
}

func mergeReplacements(reps []replacement) []replacement {
	if len(reps) == 0 {
		return nil
	}
	out := make([]replacement, 0, len(reps))
	for _, r := range reps {
		if r.start >= r.end {
			continue
		}
		if n := len(out); n > 0 && r.start < out[n-1].end {
			continue
		}
		out = append(out, r)
	}
	return out
}

type scn struct {
	s   string
	pos int
}

func (p *scn) atEnd() bool {
	return p.pos >= len(p.s)
}

func (p *scn) peek() byte {
	if p.atEnd() {
		return 0
	}
	return p.s[p.pos]
}

func (p *scn) advanceByte() {
	if p.atEnd() {
		return
	}
	_, size := utf8.DecodeRuneInString(p.s[p.pos:])
	if size < 1 {
		size = 1
	}
	p.pos += size
}

func (p *scn) skipByte(c byte) bool {
	if p.peek() == c {
		p.pos++
		return true
	}
	return false
}

func (p *scn) skipBlanks() bool {
	mark := p.pos
	for !p.atEnd() {
		if p.skipComment() {
			continue
		}
		switch p.peek() {
		case ' ', '\t', '\r', '\n':
			p.pos++
		default:
			return p.pos != mark
		}
	}
	return p.pos != mark
}

func (p *scn) skipStringCI(s string) bool {
	if p.pos+len(s) > len(p.s) {
		return false
	}
	if !strings.EqualFold(p.s[p.pos:p.pos+len(s)], s) {
		return false
	}
	end := p.pos + len(s)
	if end < len(p.s) {
		r := rune(p.s[end])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			return false
		}
	}
	p.pos = end
	return true
}

func (p *scn) skipQuoted() bool {
	c := p.peek()
	if c != '\'' && c != '"' {
		return false
	}
	p.pos++
	for !p.atEnd() {
		if p.peek() == c {
			p.pos++
			if p.peek() == c {
				p.pos++
				continue
			}
			return true
		}
		p.advanceByte()
	}
	return true
}

func (p *scn) skipComment() bool {
	if p.pos+1 >= len(p.s) {
		return false
	}
	if p.s[p.pos] == '-' && p.s[p.pos+1] == '-' {
		p.pos += 2
		for !p.atEnd() && p.peek() != '\n' {
			p.pos++
		}
		return true
	}
	if p.s[p.pos] == '/' && p.s[p.pos+1] == '*' {
		p.pos += 2
		for p.pos+1 < len(p.s) {
			if p.s[p.pos] == '*' && p.s[p.pos+1] == '/' {
				p.pos += 2
				return true
			}
			p.advanceByte()
		}
		p.pos = len(p.s)
		return true
	}
	return false
}

func (p *scn) advanceToNext() bool {
	for !p.atEnd() {
		if p.skipQuoted() {
			continue
		}
		if p.skipComment() {
			continue
		}
		switch p.peek() {
		case '(', '*', '$', '&':
			return true
		case ' ', '\t', '\n', '\r', '=', ',', '[', '>', '<', '+', '-', '/', '|', '%':
			p.advanceByte()
			if p.atEnd() {
				return false
			}
			if isNameStart(p.peek()) {
				return true
			}
			continue
		}
		p.advanceByte()
	}
	return false
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isNameChar(c byte) bool {
	return isNameStart(c) || (c >= '0' && c <= '9')
}

func (p *scn) skipName() bool {
	if p.atEnd() || !isNameStart(p.peek()) {
		return false
	}
	p.pos++
	for !p.atEnd() && isNameChar(p.peek()) {
		p.pos++
	}
	return true
}

func (p *scn) skipIdent() bool {
	if p.skipQuoted() {
		return true
	}
	mark := p.pos
	for !p.atEnd() && isNameChar(p.peek()) {
		p.pos++
	}
	return p.pos > mark
}

func (p *scn) skipIdentOrStar() bool {
	if p.skipByte('*') {
		return true
	}
	return p.skipIdent()
}

func (p *scn) skipParens() bool {
	if !p.skipByte('(') {
		return false
	}
	depth := 1
	for depth > 0 && !p.atEnd() {
		if p.skipQuoted() {
			continue
		}
		if p.skipComment() {
			continue
		}
		switch p.peek() {
		case '(':
			depth++
			p.pos++
		case ')':
			depth--
			p.pos++
		default:
			p.advanceByte()
		}
	}
	return depth == 0
}

func (p *scn) parseTypeMember(sigil byte) bool {
	if !p.skipByte(sigil) {
		return false
	}
	if !p.skipName() {
		p.pos--
		return false
	}
	if p.skipByte('[') {
		p.skipBlanks()
		if !p.skipByte(':') {
			return false
		}
		p.skipBlanks()
		return p.skipByte(']')
	}
	if !p.skipByte('.') {
		return false
	}
	return p.skipIdentOrStar()
}

func (p *scn) parseAmpType() bool {
	return p.parseTypeMember('&')
}

func (p *scn) parseDollarType() bool {
	return p.parseTypeMember('$')
}

func (p *scn) parseColumn() (star bool, ok bool) {
	mark := p.pos
	if p.skipByte('*') {
		return true, true
	}
	if !p.skipIdent() {
		p.pos = mark
		return false, false
	}
	if p.skipByte('.') {
		star = p.peek() == '*'
		if !p.skipIdentOrStar() {
			p.pos = mark
			return false, false
		}
		return star, true
	}
	if p.peek() == '(' {
		if !p.skipParens() {
			p.pos = mark
			return false, false
		}
	}
	return false, true
}

func (p *scn) parseColumnList() (star bool, paren bool, ok bool) {
	mark := p.pos
	if st, one := p.parseColumn(); one {
		return st, false, true
	}
	p.pos = mark
	if !p.skipByte('(') {
		return false, false, false
	}
	paren = true
	p.skipBlanks()
	for {
		st, colOK := p.parseColumn()
		if !colOK {
			p.pos = mark
			return false, false, false
		}
		if st {
			star = true
		}
		p.skipBlanks()
		if p.skipByte(')') {
			return star, paren, true
		}
		if !p.skipByte(',') {
			p.pos = mark
			return false, false, false
		}
		p.skipBlanks()
	}
}

func (p *scn) parseTargetTypes() bool {
	mark := p.pos
	if p.parseAmpType() {
		return true
	}
	p.pos = mark
	if !p.skipByte('(') {
		return false
	}
	p.skipBlanks()
	if !p.parseAmpType() {
		p.pos = mark
		return false
	}
	for {
		p.skipBlanks()
		if p.skipByte(')') {
			return true
		}
		if !p.skipByte(',') {
			p.pos = mark
			return false
		}
		p.skipBlanks()
		if !p.parseAmpType() {
			p.pos = mark
			return false
		}
	}
}

func (p *scn) parseOutput() ([]replacement, bool) {
	start := p.pos
	if p.parseAmpType() {
		return []replacement{{start: start, end: p.pos, token: "c"}}, true
	}
	p.pos = start
	colStart := p.pos
	star, paren, ok := p.parseColumnList()
	if !ok {
		p.pos = start
		return nil, false
	}
	p.skipBlanks()
	if !p.skipStringCI("AS") {
		p.pos = start
		return nil, false
	}
	p.skipBlanks()
	typeStart := p.pos
	if !p.parseTargetTypes() {
		p.pos = start
		return nil, false
	}
	_ = colStart
	// Parenthesized column lists such as `(a, b) AS &M.*` are not
	// valid SQLite once rewritten as `(a, b) AS c`. Replace the
	// whole output expression with a dummy identifier.
	if star || paren {
		return []replacement{{start: start, end: p.pos, token: "c"}}, true
	}
	return []replacement{{start: typeStart, end: p.pos, token: "c"}}, true
}

func (p *scn) parseInput() ([]replacement, bool) {
	start := p.pos
	if p.parseDollarType() {
		return []replacement{{start: start, end: p.pos, token: "1"}}, true
	}
	p.pos = start
	return p.parseInsert()
}

func (p *scn) parseInsert() ([]replacement, bool) {
	start := p.pos
	star, paren, ok := p.parseColumnList()
	if !ok || !paren {
		p.pos = start
		return nil, false
	}
	starEnd := p.pos
	p.skipBlanks()
	if !p.skipStringCI("VALUES") {
		p.pos = start
		return nil, false
	}
	p.skipBlanks()
	if !p.skipByte('(') {
		p.pos = start
		return nil, false
	}
	var reps []replacement
	if star {
		reps = append(reps, replacement{start: start, end: starEnd, token: "(c)"})
	}
	p.skipBlanks()
	found := false
	for {
		item := p.pos
		if p.parseDollarType() {
			found = true
			reps = append(reps, replacement{start: item, end: p.pos, token: "1"})
		} else if p.skipLiteralInList() {
			// Literal SQL in the VALUES list is left untouched.
		} else {
			p.pos = start
			return nil, false
		}
		p.skipBlanks()
		if p.skipByte(')') {
			if !found {
				p.pos = start
				return nil, false
			}
			return reps, true
		}
		if !p.skipByte(',') {
			p.pos = start
			return nil, false
		}
		p.skipBlanks()
	}
}

func (p *scn) skipLiteralInList() bool {
	mark := p.pos
	depth := 0
	for !p.atEnd() {
		if p.skipQuoted() {
			continue
		}
		if p.skipComment() {
			continue
		}
		switch p.peek() {
		case '(':
			depth++
			p.pos++
		case ')':
			if depth == 0 {
				return p.pos > mark
			}
			depth--
			p.pos++
		case ',':
			if depth == 0 {
				return p.pos > mark
			}
			p.pos++
		default:
			p.advanceByte()
		}
	}
	p.pos = mark
	return false
}
