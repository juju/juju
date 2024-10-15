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

	contextPath, ok := locatePath(parsed.Imports, "context")
	if !ok {
		os.Exit(0)
	}
	sqlPath, sqlFound := locatePath(parsed.Imports, "database/sql")
	sqlairPath, sqlairFound := locatePath(parsed.Imports, "github.com/canonical/sqlair")

	// We need to at least import one of these.
	if !sqlFound && !sqlairFound {
		os.Exit(0)
	}

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
							if len(funcDecl.Params.List) != 2 {
								continue
							}

							if !isContextType(funcDecl.Params.List[0], contextPath) {
								continue
							}

							if !isStdTxnType(funcDecl.Params.List[1], sqlPath) && !isTxnType(funcDecl.Params.List[1], sqlairPath) {
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
									if sel.Sel.Name == "Assert" {
										start := fileSet.Position(sel.Pos())

										fmt.Println("found assert: ", start)
										os.Exit(1)
									}
								}
							}
						}
					}
				}
			}
		}
	}
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func locatePath(imports []*ast.ImportSpec, path string) (string, bool) {
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

func isStdTxnType(expr *ast.Field, importPath string) bool {
	return isTxnImportType(expr, importPath, "Tx")
}

func isTxnType(expr *ast.Field, importPath string) bool {
	return isTxnImportType(expr, importPath, "TX")
}

func isTxnImportType(expr *ast.Field, importPath string, txnType string) bool {
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

	return s.Sel.Name == txnType
}
