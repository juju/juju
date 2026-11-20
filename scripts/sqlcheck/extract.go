// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strconv"
	"strings"
	"unicode"
)

type parsedFile struct {
	path string
	src  []byte
	ast  *ast.File
	tf   *token.File
}

type parsedPackage struct {
	fset  *token.FileSet
	files map[string]*parsedFile
	order []*parsedFile
}

type ignoreDir struct {
	all   bool
	rules map[string]bool
}

func parsePackage(fset *token.FileSet, files []goFile) (*parsedPackage, error) {
	pkg := &parsedPackage{
		fset:  fset,
		files: map[string]*parsedFile{},
	}
	for _, gf := range files {
		af, err := parser.ParseFile(fset, gf.Path, gf.Src, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		tf := fset.File(af.Pos())
		pf := &parsedFile{path: gf.Path, src: gf.Src, ast: af, tf: tf}
		pkg.files[gf.Path] = pf
		pkg.order = append(pkg.order, pf)
	}
	return pkg, nil
}

func extractPackage(pkg *parsedPackage, nextID *int) ([]Statement, []Skip) {
	env := &evalEnv{
		fset:  pkg.fset,
		files: pkg.files,
	}
	collectPkgStrings(env)

	ignores := map[string]map[int]ignoreDir{}
	for _, pf := range pkg.order {
		ignores[pf.path] = collectIgnores(pkg.fset, pf.ast)
	}

	seen := map[string]int{}
	var stmts []Statement
	var skips []Skip
	covered := map[string]bool{}

	addStmt := func(st Statement) {
		key := st.File + ":" + strconv.Itoa(st.Start) + ":" + strconv.Itoa(st.End)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = len(stmts)
		st.ID = *nextID
		*nextID++
		attachIgnores(&st, pkg.fset, ignores)
		stmts = append(stmts, st)
		if st.Eval != nil {
			for _, ob := range st.Eval.Map {
				if ob.File == "" || ob.Kind == KindVirtual {
					continue
				}
				covered[ob.File+":"+strconv.Itoa(ob.Offset)] = true
			}
		}
	}

	for _, pf := range pkg.order {
		ast.Inspect(pf.ast, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if callName(call.Fun) == "Sprintf" && len(call.Args) > 0 {
				out := env.eval(call)
				if out.val != nil && looksLikeSQLStrict(out.val.Content) {
					sql := toSQLite(out.val)
					file, start, end := sql.span()
					if file == "" {
						file = pf.path
						start = pf.tf.Offset(call.Pos())
						end = pf.tf.Offset(call.End())
					}
					addStmt(Statement{
						File:        file,
						Start:       start,
						End:         end,
						Eval:        sql,
						FromSprintf: true,
						RelatedPos:  []token.Pos{call.Pos()},
					})
					coverExpr(call, env, covered)
				}
			}
			arg, sink := sqlArg(call)
			if !sink || arg == nil {
				return true
			}
			out := env.eval(arg)
			if out.dynamic {
				skips = append(skips, makeSkip(pkg.fset, arg, out.reason))
				coverExpr(arg, env, covered)
				return true
			}
			if out.val == nil {
				if knownSQLSink(call) && isStringConstruction(arg) {
					skips = append(skips, makeSkip(pkg.fset, arg, "could not reconstruct SQL statically"))
				}
				return true
			}
			// A recognised sink establishes SQL-ness. Once its static
			// argument starts with a SQL keyword, lint it even when the
			// statement has no clause keyword (for example SELECT 1).
			if !looksLikeSQL(out.val.Content) &&
				!(knownSQLSink(call) && hasSQLLead(out.val.Content)) {
				return true
			}
			out.val = toSQLite(out.val)
			file, start, end := out.val.span()
			if file == "" {
				file = pf.path
				start = pf.tf.Offset(arg.Pos())
				end = pf.tf.Offset(arg.End())
			}
			st := Statement{
				File:        file,
				Start:       start,
				End:         end,
				Eval:        out.val,
				FromSprintf: callName(call.Fun) == "Sprintf" || callName(argFun(arg)) == "Sprintf",
				RelatedPos:  []token.Pos{call.Pos(), arg.Pos()},
			}
			addStmt(st)
			return true
		})
	}

	for _, pf := range pkg.order {
		ast.Inspect(pf.ast, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			off := pf.tf.Offset(lit.Pos())
			if covered[pf.path+":"+strconv.Itoa(off+1)] || covered[pf.path+":"+strconv.Itoa(off)] {
				return true
			}
			out := env.evalBasicLit(lit)
			if out.val == nil || !looksLikeSQLStrict(out.val.Content) {
				return true
			}
			out.val = toSQLite(out.val)
			file, start, end := out.val.span()
			addStmt(Statement{
				File:       file,
				Start:      start,
				End:        end,
				Eval:       out.val,
				RelatedPos: []token.Pos{lit.Pos()},
			})
			return true
		})
	}
	return stmts, skips
}

func coverExpr(expr ast.Expr, env *evalEnv, covered map[string]bool) {
	if expr == nil {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.BasicLit:
			if e.Kind != token.STRING {
				return true
			}
			pf := env.fileFor(e.Pos())
			if pf == nil {
				return true
			}
			off := pf.tf.Offset(e.Pos())
			covered[pf.path+":"+strconv.Itoa(off)] = true
			covered[pf.path+":"+strconv.Itoa(off+1)] = true
		case *ast.Ident:
			if e.Obj == nil {
				return true
			}
			switch d := e.Obj.Decl.(type) {
			case *ast.ValueSpec:
				for _, v := range d.Values {
					coverExpr(v, env, covered)
				}
			case *ast.AssignStmt:
				for _, v := range d.Rhs {
					coverExpr(v, env, covered)
				}
			}
		}
		return true
	})
}

func argFun(expr ast.Expr) ast.Expr {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil
	}
	return call.Fun
}

func sqlArg(call *ast.CallExpr) (ast.Expr, bool) {
	name := callName(call.Fun)
	if name == "" {
		return nil, false
	}
	args := call.Args
	switch name {
	case "MustPrepare", "MakePatch":
		if len(args) == 0 {
			return nil, true
		}
		return args[0], true
	case "PrepareContext", "ExecContext", "QueryContext", "QueryRowContext":
		if len(args) < 2 {
			return nil, false
		}
		return args[1], true
	case "Prepare", "Exec", "Query", "QueryRow":
		if len(args) == 0 {
			return nil, false
		}
		if len(args) > 1 && looksLikeContext(args[0]) {
			return args[1], true
		}
		return args[0], true
	default:
		return nil, false
	}
}

func knownSQLSink(call *ast.CallExpr) bool {
	switch callName(call.Fun) {
	case "Prepare", "MustPrepare", "MakePatch",
		"PrepareContext", "ExecContext", "QueryContext", "QueryRowContext",
		"Exec", "Query", "QueryRow":
		return true
	}
	return false
}

func looksLikeContext(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		n := strings.ToLower(e.Name)
		return n == "ctx" || n == "context" || strings.HasSuffix(n, "ctx")
	case *ast.CallExpr:
		n := callName(e.Fun)
		if n == "Background" || n == "TODO" {
			return true
		}
		// Any call qualified by the context package yields a context
		// value: context.WithTimeout, context.WithValue, and friends.
		// A context argument is never the SQL itself.
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "context" {
				return true
			}
		}
		return false
	case *ast.SelectorExpr:
		return looksLikeContext(e.Sel)
	}
	return false
}

func makeSkip(fset *token.FileSet, expr ast.Expr, reason string) Skip {
	pos := fset.Position(expr.Pos())
	return Skip{
		File:   pos.Filename,
		Line:   pos.Line,
		Column: pos.Column,
		Reason: reason,
		Expr:   shorten(exprString(fset, expr), 80),
	}
}

func exprString(fset *token.FileSet, expr ast.Expr) string {
	var b strings.Builder
	if err := printer.Fprint(&b, fset, expr); err != nil {
		return ""
	}
	return b.String()
}

// shorten collapses whitespace and returns s with the tail replaced by
// "..." when it exceeds n bytes. The result is at most n bytes long.
func shorten(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

var leadSQL = map[string]bool{
	"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true,
	"WITH": true, "CREATE": true, "ALTER": true, "DROP": true,
	"REPLACE": true, "PRAGMA": true, "EXPLAIN": true, "BEGIN": true,
	"COMMIT": true, "ROLLBACK": true, "VACUUM": true, "ATTACH": true,
	"DETACH": true, "SAVEPOINT": true, "RELEASE": true, "ANALYZE": true,
	"REINDEX": true, "TRUNCATE": true,
}

var clauseSQL = map[string]bool{
	"FROM": true, "WHERE": true, "INTO": true, "VALUES": true, "SET": true,
	"TABLE": true, "JOIN": true, "ON": true, "AS": true, "GROUP": true,
	"ORDER": true, "HAVING": true, "LIMIT": true, "RETURNING": true,
	"UNION": true, "EXCEPT": true, "INTERSECT": true, "INDEX": true,
	"TRIGGER": true, "VIEW": true, "USING": true, "EXISTS": true,
	"DISTINCT": true, "INNER": true, "LEFT": true, "RIGHT": true,
	"FULL": true, "CROSS": true, "NATURAL": true, "PRIMARY": true,
	"FOREIGN": true, "REFERENCES": true, "CASCADE": true, "OFFSET": true,
	"RECURSIVE": true, "WINDOW": true, "PARTITION": true, "OVER": true,
	"AND": true, "OR": true, "NOT": true, "IN": true, "BETWEEN": true,
	"LIKE": true, "IS": true, "NULL": true, "DEFAULT": true,
	"CONSTRAINT": true, "UNIQUE": true, "CHECK": true, "COLUMN": true,
	"ADD": true, "RENAME": true, "AFTER": true, "BEFORE": true,
	"EACH": true, "ROW": true, "BEGIN": true, "END": true, "WHEN": true,
}

var leadOnlySQL = map[string]bool{
	"PRAGMA": true, "EXPLAIN": true, "VACUUM": true,
}

// minSQLSniffLen is the shortest string worth sniffing for SQL: a shorter
// fragment after stripping leading noise cannot carry the keyword plus one
// clause word that the heuristics below look for.
const minSQLSniffLen = 8

func looksLikeSQL(s string) bool {
	t := stripLeadingSQLNoise(s)
	if len(t) < minSQLSniffLen {
		return false
	}
	lead, rest, ok := firstSQLWord(t)
	if !ok || !leadSQL[lead] {
		return false
	}
	if leadOnlySQL[lead] {
		return true
	}
	for _, w := range sqlWords(rest) {
		if clauseSQL[w] {
			return true
		}
	}
	return false
}

// hasSQLLead reports whether s starts with a SQL statement keyword after
// leading comments and whitespace. It deliberately does not require a
// clause keyword: recognised SQL sinks must not silently lose statements
// such as SELECT last_insert_rowid().
func hasSQLLead(s string) bool {
	lead, _, ok := firstSQLWord(stripLeadingSQLNoise(s))
	return ok && leadSQL[lead]
}

var strongClauseSQL = map[string]bool{
	"FROM": true, "WHERE": true, "INTO": true, "VALUES": true,
	"TABLE": true, "JOIN": true, "SET": true, "INDEX": true,
	"TRIGGER": true, "VIEW": true, "RETURNING": true, "UNION": true,
	"GROUP": true, "ORDER": true, "HAVING": true, "LIMIT": true,
	"REFERENCES": true, "CONSTRAINT": true, "PRIMARY": true,
}

func looksLikeSQLStrict(s string) bool {
	t := stripLeadingSQLNoise(s)
	if len(t) < minSQLSniffLen {
		return false
	}
	origLead, rest, ok := firstSQLWordRaw(t)
	if !ok {
		return false
	}
	lead := strings.ToUpper(origLead)
	if origLead != lead {
		return false
	}
	if !leadSQL[lead] {
		return false
	}
	if !leadOnlySQL[lead] {
		strong := false
		for _, w := range sqlWords(rest) {
			if strongClauseSQL[w] {
				strong = true
				break
			}
		}
		if !strong {
			return false
		}
	}
	if strings.Contains(s, "$") || strings.Contains(s, "&") {
		return true
	}
	return strings.ContainsAny(s, "()*=")
}

func stripLeadingSQLNoise(s string) string {
	s = strings.TrimSpace(s)
	for {
		switch {
		case strings.HasPrefix(s, "--"):
			if i := strings.IndexByte(s, '\n'); i >= 0 {
				s = strings.TrimSpace(s[i+1:])
				continue
			}
			return ""
		case strings.HasPrefix(s, "/*"):
			if i := strings.Index(s, "*/"); i >= 0 {
				s = strings.TrimSpace(s[i+2:])
				continue
			}
			return ""
		}
		return s
	}
}

func firstSQLWord(s string) (string, string, bool) {
	orig, rest, ok := firstSQLWordRaw(s)
	if !ok {
		return "", s, false
	}
	return strings.ToUpper(orig), rest, true
}

func firstSQLWordRaw(s string) (string, string, bool) {
	i := 0
	for i < len(s) && unicode.IsSpace(rune(s[i])) {
		i++
	}
	j := i
	for j < len(s) {
		r := rune(s[j])
		if !unicode.IsLetter(r) && r != '_' {
			break
		}
		j++
	}
	if j == i {
		return "", s, false
	}
	return s[i:j], s[j:], true
}

func sqlWords(s string) []string {
	var words []string
	start := -1
	for i, r := range s {
		if unicode.IsLetter(r) || r == '_' {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			words = append(words, strings.ToUpper(s[start:i]))
			start = -1
		}
	}
	if start >= 0 {
		words = append(words, strings.ToUpper(s[start:]))
	}
	return words
}

func collectIgnores(fset *token.FileSet, file *ast.File) map[int]ignoreDir {
	out := map[int]ignoreDir{}
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			dir, ok := parseIgnoreComment(c.Text)
			if !ok {
				continue
			}
			line := fset.Position(c.Pos()).Line
			out[line] = dir
		}
	}
	return out
}

func parseIgnoreComment(text string) (ignoreDir, bool) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "//")
	text = strings.TrimPrefix(text, "/*")
	text = strings.TrimSuffix(text, "*/")
	text = strings.TrimSpace(text)
	if text == "sqlcheck-ignore" {
		return ignoreDir{all: true}, true
	}
	rest, ok := strings.CutPrefix(text, "sqlcheck-ignore ")
	if !ok {
		return ignoreDir{}, false
	}
	rules := map[string]bool{}
	for r := range strings.SplitSeq(rest, ",") {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		rules[r] = true
	}
	if len(rules) == 0 {
		return ignoreDir{all: true}, true
	}
	return ignoreDir{rules: rules}, true
}

func attachIgnores(st *Statement, fset *token.FileSet, ignores map[string]map[int]ignoreDir) {
	lines := map[string]map[int]bool{}
	add := func(file string, line int) {
		if file == "" || line <= 0 {
			return
		}
		if lines[file] == nil {
			lines[file] = map[int]bool{}
		}
		lines[file][line] = true
		if line > 1 {
			lines[file][line-1] = true
		}
	}
	add(st.File, offsetLine(fset, st.File, st.Start))
	for _, pos := range st.RelatedPos {
		p := fset.Position(pos)
		add(p.Filename, p.Line)
	}
	if st.Eval != nil {
		for _, ob := range st.Eval.Map {
			if ob.File == "" {
				continue
			}
			add(ob.File, offsetLine(fset, ob.File, ob.Offset))
		}
	}
	for file, ls := range lines {
		byLine := ignores[file]
		if byLine == nil {
			continue
		}
		for line := range ls {
			dir, ok := byLine[line]
			if !ok {
				continue
			}
			if dir.all {
				st.IgnoreAll = true
				return
			}
			if st.IgnoreRules == nil {
				st.IgnoreRules = map[string]bool{}
			}
			for r := range dir.rules {
				st.IgnoreRules[r] = true
			}
		}
	}
}

func offsetLine(fset *token.FileSet, file string, offset int) int {
	var found *token.File
	fset.Iterate(func(f *token.File) bool {
		if f.Name() == file {
			found = f
			return false
		}
		return true
	})
	if found == nil || offset < 0 || offset > found.Size() {
		return 0
	}
	return found.Position(found.Pos(offset)).Line
}
