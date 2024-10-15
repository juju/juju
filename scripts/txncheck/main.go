// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("usage: txncheck <path>")
		os.Exit(1)
	}

	path, err := filepath.Abs(os.Args[1])
	check(err)

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, path, nil, 0)
	check(err)

	contextPath, ok := locateImportAlias(parsed.Imports, "context")
	if !ok {
		os.Exit(0)
	}
	sqlPath, sqlFound := locateImportAlias(parsed.Imports, "database/sql")
	sqlairPath, sqlairFound := locateImportAlias(parsed.Imports, "github.com/canonical/sqlair")

	// We need to at least import one of these.
	if !sqlFound && !sqlairFound {
		os.Exit(0)
	}

	// Walk over all the function declarations and look for assignment
	// statements that have a call expression with two arguments.
	// The first argument should be a context.Context and the second
	// argument should be a *sql.Tx or *sqlair.Tx.
	// If that is the case, we look for a call expression that has a
	// selector expression with the name "Assert".
	var foundAssert bool
	for _, decl := range parsed.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			for _, stmt := range d.Body.List {
				switch s := stmt.(type) {
				case *ast.AssignStmt:
					for _, r := range s.Rhs {
						switch x := r.(type) {
						case *ast.CallExpr:
							if len(x.Args) != 2 {
								continue
							}

							funcLit, ok := x.Args[1].(*ast.FuncLit)
							if !ok {
								continue
							}

							funcDecl := funcLit.Type
							list := funcDecl.Params.List
							if len(list) != 2 {
								continue
							}

							if !isContextType(list[0], contextPath) {
								continue
							}

							if !isTxnType(list[1], sqlPath, SQLTxnType) && !isTxnType(list[1], sqlairPath, SQLairTxnType) {
								continue
							}

							for _, items := range funcLit.Body.List {
								switch i := items.(type) {
								case *ast.ExprStmt:
									call, ok := i.X.(*ast.CallExpr)
									if !ok {
										continue
									}

									sel, ok := call.Fun.(*ast.SelectorExpr)
									if !ok {
										continue
									}

									id, ok := sel.X.(*ast.Ident)
									if !ok {
										continue
									}

									// We only care about the "c" variable.
									// TODO (stickupkid): This is a bit naive,
									// we should be checking if the variable is
									// actually gc.C.Assert or gc.C.Check.
									if id.Name != "c" {
										continue
									}

									start := fileSet.Position(sel.Pos())
									if sel.Sel.Name == "Assert" {
										fmt.Println("found assert: ", start)
										foundAssert = true
									}
									if sel.Sel.Name == "Check" {
										fmt.Println("found check: ", start)
									}
								}
							}
						}
					}
				}
			}
		}
	}
	// If we found an assert, we should exit with a non-zero status.
	// Any checks, are just warnings.
	if foundAssert {
		os.Exit(1)
	}
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

	if t.X.(*ast.Ident).Name != importPath {
		return false
	}

	if t.Sel.Name != "Context" {
		return false
	}
	return true
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
