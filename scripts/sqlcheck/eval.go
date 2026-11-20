// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"unicode/utf8"
)

type evalOutcome struct {
	val     *evaluated
	dynamic bool
	reason  string
}

func staticVal(v *evaluated) evalOutcome {
	return evalOutcome{val: v}
}

func dynamicVal(reason string) evalOutcome {
	return evalOutcome{dynamic: true, reason: reason}
}

func noneVal() evalOutcome {
	return evalOutcome{}
}

type evalEnv struct {
	fset       *token.FileSet
	files      map[string]*parsedFile
	pkgStrings map[string]*evaluated
}

func (env *evalEnv) fileFor(pos token.Pos) *parsedFile {
	if !pos.IsValid() {
		return nil
	}
	tf := env.fset.File(pos)
	if tf == nil {
		return nil
	}
	return env.files[tf.Name()]
}

func (env *evalEnv) eval(expr ast.Expr) evalOutcome {
	if expr == nil {
		return noneVal()
	}
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return env.eval(e.X)
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return noneVal()
		}
		return env.evalBasicLit(e)
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return noneVal()
		}
		left := env.eval(e.X)
		if left.dynamic {
			return left
		}
		right := env.eval(e.Y)
		if right.dynamic {
			return right
		}
		if left.val == nil || right.val == nil {
			if left.val != nil || right.val != nil {
				return dynamicVal("could not reconstruct SQL: concatenation is not static")
			}
			return noneVal()
		}
		return staticVal(concatEvaluated(left.val, right.val))
	case *ast.Ident:
		return env.evalIdent(e)
	case *ast.SelectorExpr:
		return dynamicVal("could not reconstruct SQL: qualified identifier is not static")
	case *ast.SliceExpr:
		return env.evalSlice(e)
	case *ast.CallExpr:
		return env.evalCall(e)
	default:
		return noneVal()
	}
}

func (env *evalEnv) evalBasicLit(lit *ast.BasicLit) evalOutcome {
	pf := env.fileFor(lit.Pos())
	if pf == nil {
		return noneVal()
	}
	start := pf.tf.Offset(lit.Pos())
	end := pf.tf.Offset(lit.End())
	if start < 0 || end > len(pf.src) || start >= end {
		return noneVal()
	}
	raw := pf.src[start:end]
	v, err := unquoteLiteral(pf.path, start, raw)
	if err != nil {
		return noneVal()
	}
	return staticVal(v)
}

func unquoteLiteral(file string, start int, raw []byte) (*evaluated, error) {
	if len(raw) < 2 {
		return nil, strconv.ErrSyntax
	}
	switch raw[0] {
	case '`':
		content := raw[1 : len(raw)-1]
		m := make([]origByte, len(content))
		for i := range content {
			m[i] = origByte{
				File: file, Offset: start + 1 + i,
				Kind: KindLiteral, RawString: true,
			}
		}
		return &evaluated{Content: string(content), Map: m}, nil
	case '"', '\'':
		return unquoteInterpreted(file, start, raw)
	default:
		return nil, strconv.ErrSyntax
	}
}

func unquoteInterpreted(file string, start int, raw []byte) (*evaluated, error) {
	i := 1
	end := len(raw) - 1
	var b strings.Builder
	var m []origByte
	for i < end {
		off := start + i
		if raw[i] != '\\' {
			r, size := utf8.DecodeRune(raw[i:end])
			if r == utf8.RuneError && size == 1 {
				b.WriteByte(raw[i])
				m = append(m, origByte{File: file, Offset: off, Kind: KindLiteral})
				i++
				continue
			}
			b.Write(raw[i : i+size])
			for j := range size {
				m = append(m, origByte{File: file, Offset: off + j, Kind: KindLiteral})
			}
			i += size
			continue
		}
		escOff := off
		i++
		if i >= end {
			return nil, strconv.ErrSyntax
		}
		ch, size, err := decodeEscape(raw[i:end])
		if err != nil {
			return nil, err
		}
		i += size
		var enc [utf8.UTFMax]byte
		n := utf8.EncodeRune(enc[:], ch)
		b.Write(enc[:n])
		for range n {
			m = append(m, origByte{File: file, Offset: escOff, Kind: KindEscape})
		}
	}
	return &evaluated{Content: b.String(), Map: m}, nil
}

func decodeEscape(s []byte) (rune, int, error) {
	if len(s) == 0 {
		return 0, 0, strconv.ErrSyntax
	}
	switch s[0] {
	case 'a':
		return '\a', 1, nil
	case 'b':
		return '\b', 1, nil
	case 'f':
		return '\f', 1, nil
	case 'n':
		return '\n', 1, nil
	case 'r':
		return '\r', 1, nil
	case 't':
		return '\t', 1, nil
	case 'v':
		return '\v', 1, nil
	case '\\', '"', '\'':
		return rune(s[0]), 1, nil
	case 'x':
		return parseHexRune(s[1:], 2)
	case 'u':
		return parseHexRune(s[1:], 4)
	case 'U':
		return parseHexRune(s[1:], 8)
	case '0', '1', '2', '3', '4', '5', '6', '7':
		return parseOctalRune(s)
	default:
		return 0, 0, strconv.ErrSyntax
	}
}

// parseHexRune parses n hex digits from s and returns the rune they
// encode, along with the number of bytes consumed: the n digits plus
// the escape letter that the caller's switch already matched.
func parseHexRune(s []byte, n int) (rune, int, error) {
	if len(s) < n {
		return 0, 0, strconv.ErrSyntax
	}
	var v rune
	for i := range n {
		d, ok := hexVal(s[i])
		if !ok {
			return 0, 0, strconv.ErrSyntax
		}
		v = v<<4 | rune(d)
	}
	return v, n + 1, nil
}

func parseOctalRune(s []byte) (rune, int, error) {
	if len(s) < 3 {
		return 0, 0, strconv.ErrSyntax
	}
	var v rune
	for i := range 3 {
		c := s[i]
		if c < '0' || c > '7' {
			return 0, 0, strconv.ErrSyntax
		}
		v = v*8 + rune(c-'0')
	}
	return v, 3, nil
}

func hexVal(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	}
	return 0, false
}

func (env *evalEnv) evalIdent(id *ast.Ident) evalOutcome {
	if id == nil {
		return noneVal()
	}
	if v, ok := env.pkgStrings[id.Name]; ok && (id.Obj == nil || id.Obj.Kind == ast.Con) {
		return staticVal(v)
	}
	if id.Obj == nil {
		return noneVal()
	}
	if id.Obj.Kind == ast.Con {
		if spec, ok := id.Obj.Decl.(*ast.ValueSpec); ok {
			return env.evalValueSpecIdent(id, spec)
		}
		return noneVal()
	}
	site, ok := lastAssignBefore(id, env.fileFor(id.Pos()))
	if !ok {
		return noneVal()
	}
	if site.tok != token.ASSIGN && site.tok != token.DEFINE {
		return dynamicVal("could not reconstruct SQL: string is appended to at runtime")
	}
	if site.rhs == nil {
		if len(site.allRhs) == 1 {
			return dynamicVal("could not reconstruct SQL: multi-value assignment is not static")
		}
		return noneVal()
	}
	return env.eval(site.rhs)
}

func (env *evalEnv) evalValueSpecIdent(id *ast.Ident, spec *ast.ValueSpec) evalOutcome {
	idx := identIndex(spec.Names, id.Name)
	if idx < 0 {
		return noneVal()
	}
	if idx >= len(spec.Values) {
		if v, ok := env.pkgStrings[id.Name]; ok {
			return staticVal(v)
		}
		return noneVal()
	}
	return env.eval(spec.Values[idx])
}

func identIndex(names []*ast.Ident, name string) int {
	for i, n := range names {
		if n.Name == name {
			return i
		}
	}
	return -1
}

type assignSite struct {
	pos    token.Pos
	tok    token.Token
	rhs    ast.Expr
	allRhs []ast.Expr
}

// lastAssignBefore returns the last assignment to id that fully
// precedes the identifier. Straight-line reuse of q/query/stmt is
// resolved to that assignment's RHS; += composition is not.
func lastAssignBefore(id *ast.Ident, pf *parsedFile) (assignSite, bool) {
	if id == nil || id.Obj == nil || pf == nil {
		return assignSite{}, false
	}
	use := id.Pos()
	var best assignSite
	found := false
	ast.Inspect(pf.ast, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		switch x := n.(type) {
		case *ast.FuncLit:
			if use < x.Pos() || use >= x.End() {
				return false
			}
		case *ast.FuncDecl:
			if use < x.Pos() || use >= x.End() {
				return false
			}
		case *ast.AssignStmt:
			if x.End() > use {
				return true
			}
			for i, lhs := range x.Lhs {
				lhsID, ok := lhs.(*ast.Ident)
				if !ok || lhsID.Obj != id.Obj {
					continue
				}
				var rhs ast.Expr
				if i < len(x.Rhs) {
					rhs = x.Rhs[i]
				}
				if !found || x.Pos() >= best.pos {
					best = assignSite{
						pos:    x.Pos(),
						tok:    x.Tok,
						rhs:    rhs,
						allRhs: x.Rhs,
					}
					found = true
				}
			}
		case *ast.ValueSpec:
			if x.End() > use {
				return true
			}
			for i, name := range x.Names {
				if name.Obj != id.Obj {
					continue
				}
				var rhs ast.Expr
				if i < len(x.Values) {
					rhs = x.Values[i]
				}
				if !found || x.Pos() >= best.pos {
					best = assignSite{
						pos:    x.Pos(),
						tok:    token.DEFINE,
						rhs:    rhs,
						allRhs: x.Values,
					}
					found = true
				}
			}
		}
		return true
	})
	return best, found
}

func (env *evalEnv) evalSlice(sl *ast.SliceExpr) evalOutcome {
	if sl.High != nil || sl.Max != nil || sl.Low == nil {
		return noneVal()
	}
	if !isIntLit(sl.Low, 1) {
		return noneVal()
	}
	inner := env.eval(sl.X)
	if inner.dynamic {
		return inner
	}
	if inner.val == nil {
		return noneVal()
	}
	return staticVal(inner.val.dropFirstByte())
}

func isIntLit(expr ast.Expr, want int) bool {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return false
	}
	n, err := strconv.Atoi(lit.Value)
	return err == nil && n == want
}

func (env *evalEnv) evalCall(call *ast.CallExpr) evalOutcome {
	name := callName(call.Fun)
	switch name {
	case "Sprintf":
		if len(call.Args) == 0 {
			return noneVal()
		}
		format := env.eval(call.Args[0])
		if format.dynamic {
			return format
		}
		if format.val == nil {
			return dynamicVal("could not reconstruct SQL: fmt.Sprintf format string is not static")
		}
		v := replaceSprintfVerbs(format.val)
		return evalOutcome{val: v}
	case "Join":
		return env.evalJoin(call)
	default:
		return noneVal()
	}
}

func (env *evalEnv) evalJoin(call *ast.CallExpr) evalOutcome {
	if len(call.Args) != 2 {
		return noneVal()
	}
	sep := env.eval(call.Args[1])
	if sep.dynamic {
		return sep
	}
	if sep.val == nil {
		return dynamicVal("could not reconstruct SQL: strings.Join separator is not static")
	}
	lit, ok := call.Args[0].(*ast.CompositeLit)
	if !ok {
		return dynamicVal("could not reconstruct SQL: strings.Join over a runtime slice")
	}
	parts := make([]*evaluated, 0, len(lit.Elts)*2)
	for i, elt := range lit.Elts {
		ev := env.eval(elt)
		if ev.dynamic {
			return ev
		}
		if ev.val == nil {
			return dynamicVal("could not reconstruct SQL: strings.Join element is not static")
		}
		if i > 0 {
			parts = append(parts, sep.val)
		}
		parts = append(parts, ev.val)
	}
	return staticVal(concatEvaluated(parts...))
}

func callName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	default:
		return ""
	}
}

func isStringConstruction(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return isStringConstruction(e.X)
	case *ast.BasicLit:
		return e.Kind == token.STRING
	case *ast.BinaryExpr:
		return e.Op == token.ADD && (isStringConstruction(e.X) || isStringConstruction(e.Y))
	case *ast.SliceExpr:
		return isStringConstruction(e.X)
	case *ast.CallExpr:
		switch callName(e.Fun) {
		case "Sprintf", "Join":
			return true
		}
		return false
	default:
		return false
	}
}

// collectPkgStrings resolves static package-level string constants and
// variables across files. Cross-file identifiers have no ast.Object, so
// this package-wide table is needed to avoid silently dropping their SQL.
func collectPkgStrings(env *evalEnv) {
	env.pkgStrings = map[string]*evaluated{}
	type pending struct {
		name string
		expr ast.Expr
	}
	var items []pending
	for _, pf := range env.files {
		for _, decl := range pf.ast.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || (gd.Tok != token.CONST && gd.Tok != token.VAR) {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					items = append(items, pending{name: name.Name, expr: vs.Values[i]})
				}
			}
		}
	}
	for {
		progress := false
		for _, it := range items {
			if _, ok := env.pkgStrings[it.name]; ok {
				continue
			}
			out := env.eval(it.expr)
			if out.val != nil {
				env.pkgStrings[it.name] = out.val
				progress = true
			}
		}
		if !progress {
			return
		}
	}
}
