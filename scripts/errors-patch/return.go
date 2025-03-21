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
	"slices"
)

const (
	jujuErrorsImportDefaultAlias = "errors"
	jujuErrorsImportPath         = `"github.com/juju/errors"`
)

// parentCollecttor is type used to build a map of every node's direct parent in
// in the AST graph.
type parentCollector struct {
	stack     []ast.Node
	parentMap map[ast.Node]ast.Node
}

// Visit implements the [ast.Visitor] interface. It is called for each node in the
// the graph building a complete map of every node's parents.
func (p *parentCollector) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		// we have reached the end of this branch pop the last node off of the stack
		if len(p.stack) > 0 {
			p.stack = p.stack[:len(p.stack)-1]
		}
		return nil
	}

	// set this nodes parent
	if len(p.stack) > 0 {
		p.parentMap[n] = p.stack[len(p.stack)-1]
	}

	// push this node as the next parent
	p.stack = append(p.stack, n)
	return p
}

// buildParentMap builds a parent map for every child node under a given root
// node.
func buildParentMap(root ast.Node) map[ast.Node]ast.Node {
	pc := &parentCollector{
		stack:     []ast.Node{},
		parentMap: make(map[ast.Node]ast.Node),
	}
	ast.Walk(pc, root)
	return pc.parentMap
}

// detectErrorsPackageAlias is responsible for detecting if the juju errors
// package is in use and if so what the import alias is for the package. True or
// false will be returned if the package is being used along with the alias
// being used if the true.
func detectErrorsPackageAlias(decl ast.Node) (bool, string) {
	importAlias := ""
	ast.Inspect(decl, func(n ast.Node) bool {
		if importAlias != "" {
			return false
		}

		importSpec, ok := n.(*ast.ImportSpec)
		if !ok {
			return true
		}

		if importSpec.Path.Value == jujuErrorsImportPath {
			importAlias = jujuErrorsImportDefaultAlias
			if importSpec.Name != nil {
				importAlias = importSpec.Name.Name
			}
			return false
		}
		return true
	})
	return importAlias != "", importAlias
}

// errorFuncsToFix is a helper function that returns the const set of
// juju/errors funcs that we are looking to fix in return statements.
func errorFuncsToFix() map[string]bool {
	return map[string]bool{
		"Annotate":  true,
		"Annotatef": true,
	}
}

// gatherAllFuncNodes is responsible for traversing a given root node and all
// of it's children finding all function nodes. This used [isFuncDeclaration] to
// determin if a given child node is a function
func gatherAllFuncNodes(root ast.Node) []ast.Node {
	funcNodes := make([]ast.Node, 0)
	ast.Inspect(root, func(n ast.Node) bool {
		if isFuncDeclaration(n) {
			funcNodes = append(funcNodes, n)
		}
		return true
	})
	return funcNodes
}

// isErrReturnProtected takes a return statement node that contains a call to
// the juju/errors package and determines if the error variable name being used
// in the return statement is being checked for nil before being returned. True
// is returned when the error is being checked for nil.
//
// The stop condition for this check is as follows:
// 1. No more parents of returnStmt to traverse
// 2. The error assignment statement is found before any if err != nil checks
// 3. If statement checking the error for nil is found.
func isErrReturnProtected(
	errArgsName string,
	returnStmt *ast.ReturnStmt,
	parentMap map[ast.Node]ast.Node,
) bool {
	var nextNode ast.Node = returnStmt
	for nextNode != nil {
		currentNode := nextNode
		nextNode = parentMap[currentNode]

		assignStmt, ok := currentNode.(*ast.AssignStmt)
		if ok {
			for _, lhs := range assignStmt.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok {
					continue
				}

				// We have found that the error is being assigned and not
				// checked for nil.
				if ident.Name == errArgsName {
					fmt.Printf("found err is assigned and not checked\n")
					return false
				}
			}
		}

		ifStmt, ok := currentNode.(*ast.IfStmt)
		if !ok {
			continue
		}

		binExp, ok := ifStmt.Cond.(*ast.BinaryExpr)
		if !ok {
			continue
		}

		if binExpProcessor(binExp, errArgsName) {
			return true
		}
	}
	// We ran out of nodes to check so the error is not being check for nil.
	return false
}

// binExpProcessor is a recursive function that is responsible for processing
// the binary expressions contained with in an if statement and check if there
// is a nil err check. True is returned if the err is being checked for nil.
func binExpProcessor(binExp *ast.BinaryExpr, errArgsName string) bool {
	subBinExp, is := binExp.X.(*ast.BinaryExpr)
	if is {
		if binExpProcessor(subBinExp, errArgsName) {
			return true
		}
	}

	subBinExp, is = binExp.X.(*ast.BinaryExpr)
	if is {
		if binExpProcessor(subBinExp, errArgsName) {
			return true
		}
	}

	xIdent, ok := binExp.X.(*ast.Ident)
	if ok {
		if xIdent.Name == errArgsName {
			return true
		}
	}

	yIdent, ok := binExp.Y.(*ast.Ident)
	if ok {
		if yIdent.Name == errArgsName {
			return true
		}
	}

	return false
}

// isFuncDeclaration is responsible for determining if a given node is a
// function declaraction by either being one of [ast.FuncDecl] or [ast.FuncLit].
func isFuncDeclaration(n ast.Node) bool {
	switch n.(type) {
	case *ast.FuncDecl, *ast.FuncLit:
		return true
	default:
		return false
	}
	// TODO: check if there is a body or not
}

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

// processFile is responsible for taking an ast file and fixing any error
// return statements in the file that are using the juju/errors pkg. This func
// returns a boolean indicating if the file was modified because a return
// condition was found.
func processFile(file *ast.File) bool {
	// We need to first establish if the file is importing juju/errors and if so
	// what is the import alias.
	errPkgAlias := ""
	ast.Inspect(file, func(n ast.Node) bool {
		if _, ok := n.(*ast.File); ok {
			return true
		}
		decl, ok := n.(*ast.GenDecl)
		if !ok {
			return false
		}

		if found, alias := detectErrorsPackageAlias(decl); found {
			errPkgAlias = alias
		}
		return false
	})

	// If the file isn't importing juju/error we can get out of here.
	if errPkgAlias == "" {
		return false
	}

	modified := false
	// Gather up all of the functions in the file to work on.
	fileFuncs := gatherAllFuncNodes(file)
	for _, funcNode := range fileFuncs {

		if processFuncNode(funcNode, errPkgAlias) {
			modified = true
		}
	}

	return modified
}

func processFuncNode(funcNode ast.Node, errPkgAlias string) bool {
	parentMap := buildParentMap(funcNode)
	modified := false
	ast.Inspect(funcNode, func(node ast.Node) bool {
		if node == funcNode {
			return true
		}
		if isFuncDeclaration(node) {
			return false
		}

		retStmt, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}

		for _, exp := range retStmt.Results {
			callExp, ok := exp.(*ast.CallExpr)
			if !ok {
				continue
			}

			sel, ok := callExp.Fun.(*ast.SelectorExpr)
			if !ok {
				continue
			}

			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				continue
			}
			if ident.Name != errPkgAlias {
				continue
			}

			if !errorFuncsToFix()[sel.Sel.Name] {
				continue
			}

			if len(callExp.Args) < 1 {
				continue
			}

			embedCallExp, is := callExp.Args[0].(*ast.CallExpr)
			if is {
				moveEmbeddedErrorFunc(embedCallExp, callExp, retStmt, parentMap)
			}

			// We always assume that the first argument to the function is the
			// error we want to check.
			errArg, isVar := callExp.Args[0].(*ast.Ident)
			if !isVar {
				continue
			}

			// We use the parent map to traverse upwards from this node to check
			// if the error has been checked for nil. If not we wrap the call
			// in a nil error check.
			if !isErrReturnProtected(errArg.Name, retStmt, parentMap) {
				modified = true
				protectErrReturn(errArg.Name, retStmt, parentMap)
			}
		}

		return true
	})
	return modified
}

// moveEmbeddedErrorFunc is responsible for moving emembbed calls to functions
// that error out of the errors pkg call. Somthing like:
//
//	return errors.Annotate(funcThatErrors(), "error")
//
// becomes:
//
//	    	autoErr := funcThatErrors()
//		   	return errors.Annotate(autoErr, "error")
func moveEmbeddedErrorFunc(
	errCall *ast.CallExpr,
	callExp *ast.CallExpr,
	n *ast.ReturnStmt,
	parentMap map[ast.Node]ast.Node,
) {
	parent := parentMap[n]
	if parent == nil {
		return
	}

	// We always assume that the parent of the return statement is a block
	// statement.
	blkStmt, ok := parent.(*ast.BlockStmt)
	if !ok {
		return
	}

	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent("autoErr")},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{
			errCall,
		},
	}

	callExp.Args[0] = ast.NewIdent("autoErr")

	for i, stmt := range blkStmt.List {
		if stmt != n {
			continue
		}

		blkStmt.List = slices.Insert(
			blkStmt.List,
			i,
			ast.Stmt(assignStmt),
		)
	}
}

func protectErrReturn(
	errArgsName string,
	n *ast.ReturnStmt,
	parentMap map[ast.Node]ast.Node,
) {
	parent := parentMap[n]
	if parent == nil {
		return
	}

	// We always assume that the parent of the return statement is a block
	// statement.
	blkStmt, ok := parent.(*ast.BlockStmt)
	if !ok {
		return
	}

	outerReturnStmt := &ast.ReturnStmt{
		Return:  token.NoPos,
		Results: make([]ast.Expr, 0, len(n.Results)),
	}

	for i, exp := range n.Results {
		outerReturnStmt.Results = append(outerReturnStmt.Results, exp)

		callExp, ok := exp.(*ast.CallExpr)
		if !ok {
			continue
		}

		sel, ok := callExp.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}

		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			continue
		}
		if ident.Name != "errors" {
			continue
		}

		outerReturnStmt.Results[i] = ast.NewIdent("nil")
	}

	for i, stmt := range blkStmt.List {
		if stmt != n {
			continue
		}

		n.Return = token.NoPos
		newIf := &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.Ident{Name: errArgsName},
				Op: token.NEQ,
				Y:  &ast.Ident{Name: "nil"},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{n},
			},
		}

		blkStmt.List[i] = newIf
		blkStmt.List = slices.Insert(
			blkStmt.List,
			i+1,
			ast.Stmt(outerReturnStmt),
		)
	}
}
