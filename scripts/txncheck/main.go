// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: txncheck <path>")
		os.Exit(1)
	}

	var foundAssert bool
	err := filepath.WalkDir(os.Args[1], func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if filepath.Ext(path) != ".go" {
			return nil
		}

		file, err := os.OpenFile(path, os.O_RDONLY, 0)
		if err != nil {
			return nil
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {

			if strings.Contains(scanner.Text(), "database/sql") ||
				strings.Contains(scanner.Text(), "github.com/canonical/sqlair") {

				_, _ = file.Seek(0, 0)

				if checkFile(path, file) {
					foundAssert = true
					return nil
				}

			}
		}

		return nil
	})
	check(err)

	// If we found an assert, we should exit with a non-zero status.
	// Any checks, are just warnings.
	if foundAssert {
		os.Exit(1)
	}
}

func checkFile(path string, file io.Reader) bool {
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "", file, 0)
	check(err)

	contextPath, ok := locateImportAlias(parsed.Imports, "context")
	if !ok {
		return false
	}
	sqlPath, sqlFound := locateImportAlias(parsed.Imports, "database/sql")
	sqlairPath, sqlairFound := locateImportAlias(parsed.Imports, "github.com/canonical/sqlair")

	// We need to at least import one of these.
	if !sqlFound && !sqlairFound {
		return false
	}

	// Walk over all the function declarations and look for assignment
	// statements that have a call expression with two arguments.
	// The first argument should be a context.Context and the second
	// argument should be a *sql.Tx or *sqlair.Tx.
	// If that is the case, we look for a call expression that has a
	// selector expression with the name "Assert".

	for _, stmt := range getFuncBodies(parsed) {
		for _, funcLit := range getCallExprs(stmt) {

			funcDecl := funcLit.Type
			list := funcDecl.Params.List
			if len(list) != 2 {
				continue
			}

			// Is the first argument a context.Context?
			if !isContextType(list[0], contextPath) {
				continue
			}

			// Is the scope of the second argument a *sql.Tx or *sqlair.Tx?
			if !isTxnType(list[1], sqlPath, SQLTxnType) && !isTxnType(list[1], sqlairPath, SQLairTxnType) {
				continue
			}

			// Located a potential transaction function, now we need to
			// check if it has a call expression with a selector expression
			// that has the name "Assert" or "Check".
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
						fmt.Printf("found assert: %s:%s\n", path, start)
						return true
					}
					if sel.Sel.Name == "Check" {
						fmt.Printf("found check: %s:%s\n", path, start)
						return true
					}
				}
			}
		}
	}

	return false
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func getFuncBodies(parsed *ast.File) []ast.Stmt {
	var stmts []ast.Stmt
	for _, decl := range parsed.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			stmts = append(stmts, d.Body.List...)
		}
	}
	return stmts
}

func getCallExprs(stmt ast.Stmt) []*ast.FuncLit {
	var callExprs []*ast.FuncLit
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		for _, r := range s.Rhs {
			switch x := r.(type) {
			case *ast.CallExpr:
				// If the call expression doesn't have two arguments,
				// we skip it.
				if len(x.Args) != 2 {
					continue
				}

				// If the second argument is not a function literal, which
				// would correspond to the following signature, we skip it.
				//
				//     func(context.Context, func(context.Context, *sql.Tx) error) error
				//     func(context.Context, func(context.Context, *sqlair.TX) error) error
				//
				funcLit, ok := x.Args[1].(*ast.FuncLit)
				if !ok {
					continue
				}

				callExprs = append(callExprs, funcLit)
			}
		}
	case *ast.ExprStmt:
		// Recursive descent into the expression statement to find the
		// call expression.
		switch x := s.X.(type) {
		case *ast.CallExpr:
			if len(x.Args) == 0 {
				return callExprs
			}

			// Check the function literal arguments.
			for _, arg := range x.Args {
				funcLit, ok := arg.(*ast.FuncLit)
				if !ok {
					continue
				}
				callExprs = append(callExprs, funcLit)

				for _, stmt := range funcLit.Body.List {
					callExprs = append(callExprs, getCallExprs(stmt)...)
				}
			}
		}
	}
	return callExprs
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
