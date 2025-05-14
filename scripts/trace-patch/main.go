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

	results := namedReturnedArgs(funcDecl.Name.Name, funcDecl.Type.Results)
	funcDecl.Type.Results = results

	return true
}

func namedReturnedArgs(methodName string, results *ast.FieldList) *ast.FieldList {
	if results == nil || len(results.List) == 0 {
		return nil
	}

	var matches int
	for i, field := range results.List {
		// Don't modify if the field has names
		// (e.g. (ctx context.Context, err error))
		if len(field.Names) == 0 {
			continue
		}

		names := field.Names
		name := names[0]

		if name.Name == "_" {
			matches++
		}
		if i == len(results.List)-1 && name.Name == "err" {
			matches++
		}
	}

	if matches != len(results.List) {
		fmt.Fprintln(os.Stderr, "Skipping method", methodName)
		return results
	}

	for _, field := range results.List {
		field.Names = nil
	}

	return results
}

func isExported(name string) bool {
	if name == "" {
		return false
	}
	return unicode.IsUpper(rune(name[0]))
}
