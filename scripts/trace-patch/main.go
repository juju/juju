// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

func main() {
	fset := token.NewFileSet()
	dir := os.Args[1]

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			return nil
		}

		dirPkgs, err := parser.ParseDir(fset, path, nil, parser.ParseComments)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing dir %q: %v", dir, err)
			os.Exit(1)
		}

		for _, pkg := range dirPkgs {
			for filePath, file := range pkg.Files {
				if strings.HasSuffix(filePath, "_test.go") {
					continue
				} else if ast.IsGenerated(file) {
					continue
				}

				modified := processFile(file)
				if err != nil {
					fmt.Fprintf(
						os.Stderr,
						"error processing file %q in pkg %q: %v",
						filePath,
						pkg.Name,
						err,
					)
					os.Exit(1)
				}

				if !modified {
					fmt.Printf("file %q in pkg %q not modified\n", filePath, pkg.Name)
					continue
				}

				fileIO, err := os.Create(filePath)
				if err != nil {
					fmt.Fprintf(
						os.Stderr,
						"error opening file %q for writing: %v",
						filePath,
						err,
					)
					os.Exit(1)
				}
				defer fileIO.Close()

				err = Write(fset, file, filePath)
				if err != nil {
					fmt.Fprintf(
						os.Stderr,
						"error writing file %q: %v",
						filePath,
						err,
					)
					os.Exit(1)
				}

				fmt.Printf("file %q in pkg %q modified\n", filePath, pkg.Name)
			}
		}
		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "error walking dir %q: %v", dir, err)
		os.Exit(1)
	}
}

func Write(
	fs *token.FileSet,
	file *ast.File,
	filePath string,
) error {
	buf := &bytes.Buffer{}
	err := printer.Fprint(buf, fs, file)
	if err != nil {
		return err
	}

	gofmt, _ := exec.LookPath("gofmt")
	if gofmt != "" {
		outBuf := &bytes.Buffer{}
		cmd := exec.Command(gofmt, "-s")
		cmd.Stdin = buf
		cmd.Stderr = os.Stderr
		cmd.Stdout = outBuf
		err := cmd.Run()
		if err != nil {
			return err
		}
		buf = outBuf
	}

	err = os.WriteFile(filePath, buf.Bytes(), 0600)
	if err != nil {
		return err
	}

	return nil
}

// processFile is responsible for taking an ast file and injecting the
// necessary changes to it. It returns true if the file was modified.
func processFile(file *ast.File) bool {
	// Only import "fmt" if needed
	var hasTrace bool
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) == "github.com/juju/juju/core/trace" {
			hasTrace = true
			break
		}
	}
	if !hasTrace {
		newImport := &ast.ImportSpec{
			Path: &ast.BasicLit{Kind: token.STRING, Value: `"github.com/juju/juju/core/trace"`},
		}
		decl := &ast.GenDecl{
			Tok:    token.IMPORT,
			Specs:  []ast.Spec{newImport},
			Lparen: token.NoPos,
		}
		file.Decls = append([]ast.Decl{decl}, file.Decls...)
	}

	var modified bool
	ast.Inspect(file, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			// Check if method has a receiver and is exported
			if fn.Recv != nil && isExported(fn.Name.Name) {
				m := processMethod(fn)
				modified = modified || m
			}
		}
		return true
	})

	return modified
}

// processMethod is responsible for taking a method and injecting the
// necessary changes to it. It returns true if the method was modified.
func processMethod(funcDecl *ast.FuncDecl) bool {
	// We must have a body.
	if funcDecl.Body == nil {
		return false
	}

	fmt.Fprintln(os.Stderr, "Processing method", funcDecl.Name.Name)

	stmts := traceExpr(funcDecl.Name.Name)
	funcDecl.Body.List = append(stmts, funcDecl.Body.List...)

	results := namedReturnedArgs(funcDecl.Type.Results)
	funcDecl.Type.Results = results

	return true
}

func traceExpr(name string) []ast.Stmt {
	// 1. ctx, span := trace.Start(ctx, trace.NameFromFunc())
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			ast.NewIdent("ctx"),
			ast.NewIdent("span"),
		},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{
			&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("trace"),
					Sel: ast.NewIdent("Start"),
				},
				Args: []ast.Expr{
					ast.NewIdent("ctx"),
					&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent("trace"),
							Sel: ast.NewIdent("NameFromFunc"),
						},
					},
				},
			},
		},
	}

	// 2. defer func() { span.RecordError(err); span.End() }()
	deferStmt := &ast.DeferStmt{
		Call: &ast.CallExpr{
			Fun: &ast.FuncLit{
				Type: &ast.FuncType{
					Params: &ast.FieldList{},
				},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.ExprStmt{
							X: &ast.CallExpr{
								Fun: &ast.SelectorExpr{
									X:   ast.NewIdent("span"),
									Sel: ast.NewIdent("RecordError"),
								},
								Args: []ast.Expr{ast.NewIdent("err")},
							},
						},
						&ast.ExprStmt{
							X: &ast.CallExpr{
								Fun: &ast.SelectorExpr{
									X:   ast.NewIdent("span"),
									Sel: ast.NewIdent("End"),
								},
							},
						},
					},
				},
			},
		},
	}

	return []ast.Stmt{
		assignStmt,
		deferStmt,
	}
}

func namedReturnedArgs(results *ast.FieldList) *ast.FieldList {
	if results == nil || len(results.List) == 0 {
		return nil
	}

	for _, field := range results.List {
		// Don't modify if the field has names
		// (e.g. (ctx context.Context, err error))
		if len(field.Names) > 0 {
			continue
		}

		name := "_"
		if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == "error" {
			name = "err"
		}

		field.Names = []*ast.Ident{
			ast.NewIdent(name),
		}
	}

	return results
}

func isExported(name string) bool {
	if name == "" {
		return false
	}
	return unicode.IsUpper(rune(name[0]))
}
