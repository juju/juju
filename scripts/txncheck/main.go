// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: txncheck <path>")
		os.Exit(1)
	}

	root, err := filepath.Abs(os.Args[1])
	check(err)

	var foundIssue bool
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if !shouldCheck(data) {
			return nil
		}

		findings, err := checkFile(path, data)
		if err != nil {
			return err
		}
		for _, finding := range findings {
			fmt.Println(finding)
			foundIssue = true
		}
		return nil
	})
	check(err)

	if foundIssue {
		os.Exit(1)
	}
}

func shouldCheck(data []byte) bool {
	return bytes.Contains(data, []byte("database/sql")) ||
		bytes.Contains(data, []byte("github.com/canonical/sqlair"))
}

func checkFile(path string, data []byte) ([]finding, error) {
	return checkFileWithChecks(path, data, defaultChecks())
}

func checkFileWithChecks(path string, data []byte, checks []nodeCheck) ([]finding, error) {
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, path, data, 0)
	if err != nil {
		return nil, err
	}

	contextPath, ok := locateImportAlias(parsed.Imports, "context")
	if !ok {
		return nil, nil
	}
	sqlPath, sqlFound := locateImportAlias(parsed.Imports, "database/sql")
	sqlairPath, sqlairFound := locateImportAlias(parsed.Imports, "github.com/canonical/sqlair")
	if !sqlFound && !sqlairFound {
		return nil, nil
	}

	fileCtx := &fileContext{
		fileSet:     fileSet,
		contextPath: contextPath,
		sqlPath:     sqlPath,
		sqlairPath:  sqlairPath,
	}
	ast.Walk(&fileVisitor{
		checks: checks,
		file:   fileCtx,
	}, parsed)
	return fileCtx.findings, nil
}

type finding struct {
	pos     token.Position
	message string
}

func (f finding) String() string {
	return fmt.Sprintf("%s: %s", f.message, f.pos)
}

type nodeCheck interface {
	Check(*nodeContext, ast.Node)
}

func defaultChecks() []nodeCheck {
	return []nodeCheck{
		assertCheck{},
		capturedAssignmentCheck{},
		capturedBuiltinMutationCheck{},
	}
}

type fileContext struct {
	fileSet     *token.FileSet
	contextPath string
	sqlPath     string
	sqlairPath  string

	findings []finding
}

func (c *fileContext) isTxnFunc(funcLit *ast.FuncLit) bool {
	params := funcLit.Type.Params
	if params == nil || len(params.List) != 2 {
		return false
	}
	if !isContextType(params.List[0], c.contextPath) {
		return false
	}
	if !isErrorResult(funcLit.Type.Results) {
		return false
	}
	return isTxnType(params.List[1], c.sqlPath, SQLTxnType) ||
		isTxnType(params.List[1], c.sqlairPath, SQLairTxnType)
}

func (c *fileContext) addFinding(pos token.Pos, message string) {
	c.findings = append(c.findings, finding{
		pos:     c.fileSet.Position(pos),
		message: message,
	})
}

type nodeContext struct {
	file       *fileContext
	txn        *ast.FuncLit
	reassigned map[*ast.Object]bool
}

func (c *nodeContext) InTxn() bool {
	return c.txn != nil
}

func (c *nodeContext) IsCaptured(id *ast.Ident) bool {
	if !c.InTxn() || id == nil || id.Name == "_" || id.Obj == nil {
		return false
	}
	return id.Obj.Pos() < c.txn.Pos() || id.Obj.Pos() > c.txn.End()
}

func (c *nodeContext) IsReassigned(id *ast.Ident) bool {
	return id != nil && id.Obj != nil && c.reassigned[id.Obj]
}

func (c *nodeContext) MarkReassigned(id *ast.Ident) {
	if c.IsCaptured(id) {
		c.reassigned[id.Obj] = true
	}
}

func (c *nodeContext) AddFinding(pos token.Pos, message string) {
	c.file.addFinding(pos, message)
}

type fileVisitor struct {
	checks     []nodeCheck
	file       *fileContext
	txn        *ast.FuncLit
	reassigned map[*ast.Object]bool
}

func (v *fileVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}

	ctx := &nodeContext{
		file:       v.file,
		txn:        v.txn,
		reassigned: v.reassigned,
	}
	for _, check := range v.checks {
		check.Check(ctx, node)
	}

	funcLit, ok := node.(*ast.FuncLit)
	if ok {
		if v.file.isTxnFunc(funcLit) {
			return v.withReassigned(funcLit, make(map[*ast.Object]bool))
		}
		if v.txn != nil {
			return v.withReassigned(v.txn, cloneReassigned(v.reassigned))
		}
		return v
	}

	if v.txn != nil && isolatesReassignments(node) {
		return v.withReassigned(v.txn, cloneReassigned(v.reassigned))
	}
	return v
}

func (v *fileVisitor) withReassigned(
	txn *ast.FuncLit,
	reassigned map[*ast.Object]bool,
) *fileVisitor {
	return &fileVisitor{
		checks:     v.checks,
		file:       v.file,
		txn:        txn,
		reassigned: reassigned,
	}
}

func isolatesReassignments(node ast.Node) bool {
	switch node.(type) {
	case *ast.BlockStmt,
		*ast.IfStmt,
		*ast.ForStmt,
		*ast.RangeStmt,
		*ast.SwitchStmt,
		*ast.TypeSwitchStmt,
		*ast.SelectStmt:
		return true
	}
	return false
}

type capturedAssignmentCheck struct{}

func (capturedAssignmentCheck) Check(ctx *nodeContext, node ast.Node) {
	if !ctx.InTxn() {
		return
	}
	switch n := node.(type) {
	case *ast.AssignStmt:
		checkCapturedAssignment(ctx, n)
	case *ast.IncDecStmt:
		checkCapturedIncDec(ctx, n)
	}
}

func checkCapturedAssignment(ctx *nodeContext, stmt *ast.AssignStmt) {
	switch stmt.Tok {
	case token.DEFINE:
		return
	case token.ASSIGN:
		reassigned := capturedReassignments(ctx, stmt.Lhs)
		checkCapturedBuiltinMutations(ctx, stmt.Rhs, reassigned)
		for _, lhs := range stmt.Lhs {
			if _, ok := lhs.(*ast.Ident); ok {
				continue
			}
			if id := rootIdent(lhs); ctx.IsCaptured(id) {
				if ctx.IsReassigned(id) {
					continue
				}
				ctx.AddFinding(lhs.Pos(),
					fmt.Sprintf("found captured mutation in transaction: %q is mutated by assignment", id.Name))
			}
		}
		for _, id := range reassigned {
			ctx.MarkReassigned(id)
		}
	default:
		for _, lhs := range stmt.Lhs {
			if id := rootIdent(lhs); ctx.IsCaptured(id) {
				if ctx.IsReassigned(id) {
					continue
				}
				ctx.AddFinding(lhs.Pos(),
					fmt.Sprintf("found captured mutation in transaction: %q is mutated by compound assignment", id.Name))
			}
		}
	}
}

func checkCapturedIncDec(ctx *nodeContext, stmt *ast.IncDecStmt) {
	if id := rootIdent(stmt.X); ctx.IsCaptured(id) {
		if ctx.IsReassigned(id) {
			return
		}
		ctx.AddFinding(stmt.Pos(),
			fmt.Sprintf("found captured mutation in transaction: %q is mutated by increment/decrement", id.Name))
	}
}

type assertCheck struct{}

func (assertCheck) Check(ctx *nodeContext, node ast.Node) {
	if !ctx.InTxn() {
		return
	}
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != "c" {
		return
	}

	switch sel.Sel.Name {
	case "Assert":
		ctx.AddFinding(sel.Pos(), "found assert in transaction")
	case "Check":
		ctx.AddFinding(sel.Pos(), "found check in transaction")
	}
}

type capturedBuiltinMutationCheck struct{}

func (capturedBuiltinMutationCheck) Check(ctx *nodeContext, node ast.Node) {
	if !ctx.InTxn() {
		return
	}
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return
	}

	name := builtinName(call.Fun)
	switch name {
	case "append":
		if len(call.Args) == 0 {
			return
		}
		if id := rootIdent(call.Args[0]); ctx.IsCaptured(id) {
			if ctx.IsReassigned(id) {
				return
			}
			ctx.AddFinding(call.Pos(),
				fmt.Sprintf("found captured mutation in transaction: %q is mutated by append", id.Name))
		}
	case "copy", "delete", "clear":
		if len(call.Args) == 0 {
			return
		}
		if id := rootIdent(call.Args[0]); ctx.IsCaptured(id) {
			if ctx.IsReassigned(id) {
				return
			}
			ctx.AddFinding(call.Pos(),
				fmt.Sprintf("found captured mutation in transaction: %q is mutated by %s", id.Name, name))
		}
	}
}

func capturedReassignments(ctx *nodeContext, exprs []ast.Expr) map[*ast.Object]*ast.Ident {
	reassigned := make(map[*ast.Object]*ast.Ident)
	for _, expr := range exprs {
		id, ok := expr.(*ast.Ident)
		if !ok || !ctx.IsCaptured(id) {
			continue
		}
		reassigned[id.Obj] = id
	}
	return reassigned
}

func checkCapturedBuiltinMutations(
	ctx *nodeContext,
	exprs []ast.Expr,
	reassigned map[*ast.Object]*ast.Ident,
) {
	for _, expr := range exprs {
		ast.Inspect(expr, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}

			id := capturedBuiltinMutation(call)
			if id == nil || reassigned[id.Obj] == nil || ctx.IsReassigned(id) {
				return true
			}

			name := builtinName(call.Fun)
			ctx.AddFinding(call.Pos(),
				fmt.Sprintf("found captured mutation in transaction: %q is mutated by %s", id.Name, name))
			return true
		})
	}
}

func capturedBuiltinMutation(call *ast.CallExpr) *ast.Ident {
	name := builtinName(call.Fun)
	switch name {
	case "append", "copy", "delete", "clear":
		if len(call.Args) == 0 {
			return nil
		}
		return rootIdent(call.Args[0])
	}
	return nil
}

func cloneReassigned(reassigned map[*ast.Object]bool) map[*ast.Object]bool {
	clone := make(map[*ast.Object]bool, len(reassigned))
	maps.Copy(clone, reassigned)
	return clone
}

func builtinName(expr ast.Expr) string {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return ""
	}
	return id.Name
}

func rootIdent(expr ast.Expr) *ast.Ident {
	switch e := expr.(type) {
	case *ast.Ident:
		return e
	case *ast.IndexExpr:
		return rootIdent(e.X)
	case *ast.IndexListExpr:
		return rootIdent(e.X)
	case *ast.SelectorExpr:
		return rootIdent(e.X)
	case *ast.StarExpr:
		return rootIdent(e.X)
	case *ast.ParenExpr:
		return rootIdent(e.X)
	}
	return nil
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func locateImportAlias(imports []*ast.ImportSpec, path string) (string, bool) {
	for _, i := range imports {
		if i.Path.Value != fmt.Sprintf(`"%s"`, path) {
			continue
		}

		// Use the last part of the path as the package name.
		if i.Name == nil {
			p := strings.Split(path, "/")
			return p[len(p)-1], true
		}

		return i.Name.Name, true
	}
	return "", false
}

func isContextType(expr *ast.Field, importPath string) bool {
	t, ok := expr.Type.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	i, ok := t.X.(*ast.Ident)
	if !ok || i.Name != importPath {
		return false
	}

	if t.Sel.Name != "Context" {
		return false
	}
	return true
}

func isErrorResult(results *ast.FieldList) bool {
	if results == nil || len(results.List) != 1 {
		return false
	}
	id, ok := results.List[0].Type.(*ast.Ident)
	return ok && id.Name == "error"
}

type txnType string

const (
	SQLTxnType    txnType = "Tx"
	SQLairTxnType txnType = "TX"
)

func isTxnType(expr *ast.Field, importPath string, txnType txnType) bool {
	t, ok := expr.Type.(*ast.StarExpr)
	if !ok {
		return false
	}

	s, ok := t.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	i, ok := s.X.(*ast.Ident)
	if !ok {
		return false
	}

	if i.Name != importPath {
		return false
	}

	return s.Sel.Name == string(txnType)
}
