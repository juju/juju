// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/juju/collections/set"
	"github.com/juju/loggo"
)

var (
	jujuStatePkg  = flag.String("juju-pkg-name", "github.com/juju/juju/state", "the pkg name to scan for mongo doc structs")
	erdOutputFile = flag.String("output", "-", "the file (or - for STDOUT) to write the generated ER diagram")

	logger = loggo.GetLogger("infer_schema")
)

// structAST represents a parsed struct representing a mongo document.
type structAST struct {
	// The name of the struct identifier.
	Name string

	// The file where the struct was defined.
	SrcFile string

	// The AST for the struct.
	Decl *ast.StructType
}

func main() {
	flag.Parse()

	if err := inferSchema(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func inferSchema() error {
	structASTs, fset, err := extractMongoDocStructASTs()
	if err != nil {
		return err
	}

	// Cluster structs by type
	clusters := clusterASTS(structASTs)

	// Render ERD
	var w io.Writer
	if *erdOutputFile == "-" {
		w = os.Stdout
	} else {
		of, err := os.Open(*erdOutputFile)
		if err != nil {
			return fmt.Errorf("unable to open %q for writing: %v", *erdOutputFile, err)
		}
		w = of
		defer func() { _ = of.Close() }()
	}
	renderERD(w, clusters, fset)

	return nil
}

func extractMongoDocStructASTs() ([]structAST, *token.FileSet, error) {
	logger.Infof("parsing files in package %q", *jujuStatePkg)
	fset, pkgInfo, err := loadPkg(*jujuStatePkg)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to resolve type information from package %q: %w", *jujuStatePkg, err)
	}

	logger.Infof("extracting mongo doc mapping structs")
	structASTs, fset := extractStructASTs(fset, pkgInfo.Files, isMongoDocMapping)
	logger.Infof("extracted %d mongo doc mapping structs", len(structASTs))

	return structASTs, fset, nil
}

// loadPkg uses go/loader to compile pkgName (including any of its
// direct and indirect dependencies) and returns back the obtained ASTs and type
// information.
func loadPkg(pkgName string) (*token.FileSet, *ast.Package, error) {
	pathToPkg := filepath.Join(os.Getenv("GOPATH"), "src", pkgName)

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, pathToPkg, func(fi os.FileInfo) bool {
		// Ignore test files
		return !strings.Contains(fi.Name(), "_test")
	}, parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}

	pkg, found := pkgs[filepath.Base(pkgName)]
	if !found {
		for k := range pkgs {
			fmt.Printf("pkg: %q\n", k)
		}
		return nil, nil, fmt.Errorf("unable to identify package %q contents", pkgName)
	}

	return fset, pkg, nil
}

// extractASTs returns a list of struct ASTs within a package that satisfy the
// provided selectFn func.
func extractStructASTs(fset *token.FileSet, fileASTs map[string]*ast.File, selectFn func(*ast.TypeSpec) bool) ([]structAST, *token.FileSet) {
	stripPrefix := filepath.Join(os.Getenv("GOPATH"), "src") + string(filepath.Separator)

	var structASTs []structAST
	for _, fileAST := range fileASTs {
		for _, decl := range fileAST.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}

			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}

				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}

				if !selectFn(typeSpec) {
					continue
				}

				structPos := fset.Position(fileAST.Pos())
				structASTs = append(structASTs,
					structAST{
						Name:    normalizeName(typeSpec.Name.Name),
						SrcFile: strings.TrimPrefix(structPos.Filename, stripPrefix),
						Decl:    structType,
					},
				)
			}
		}
	}

	return structASTs, fset
}

func isMongoDocMapping(tspec *ast.TypeSpec) bool {
	name := tspec.Name.Name
	lcIdent := name[0] >= 'a' && name[0] <= 'z'
	return lcIdent && strings.HasSuffix(name, "Doc")
}

func clusterASTS(structASTs []structAST) map[string][]structAST {
	// Construct a set of possible prefix names
	prefixSet := set.NewStrings()
	for _, str := range structASTs {
		if strings.ContainsRune(str.Name, '_') {
			continue // assume this can't be a prefix
		}

		prefixSet.Add(str.Name)
	}

	// Construct a set of possible foreign names
	foreignSet := make(map[string]string)
	for _, str := range structASTs {
		for _, field := range str.Decl.Fields.List {
			if ident, ok := field.Type.(*ast.Ident); ok {
				if !strings.HasSuffix(ident.Name, "Status") {
					continue
				}

				value := strings.ToLower(strings.TrimSuffix(ident.Name, "Status"))
				if value == "" {
					continue
				}
				foreignSet[str.Name] = value
			}
		}
	}

	// Group structs sharing each prefix
	clusters := make(map[string][]structAST)
nextStruct:
	for _, str := range structASTs {
		var added bool
		if name, ok := foreignSet[str.Name]; ok {
			clusters[name] = append(clusters[name], str)
			added = true
		}
		if prefixSet.Contains(str.Name) {
			clusters[str.Name] = append(clusters[str.Name], str)
			added = true
		}

		if added {
			continue
		}

		// Can we cluster it with any of the prefixes?
		for prefix := range prefixSet {
			if strings.HasPrefix(str.Name, prefix) {
				clusters[prefix] = append(clusters[prefix], str)
				continue nextStruct
			}
		}

		// Add as a standalone cluster
		clusters[str.Name] = append(clusters[str.Name], str)
	}

	// Co-locate ASTs that don't have any other ASTs in their cluster
	for key, asts := range clusters {
		if len(asts) != 1 {
			continue
		}

		clusters[""] = append(clusters[""], asts...)
		delete(clusters, key)
	}

	return clusters
}

func renderERD(w io.Writer, clusters map[string][]structAST, fset *token.FileSet) {
	braceEscaper := strings.NewReplacer(
		"{", "\\{",
		"}", "\\}",
	)

	fmt.Fprintln(w, "graph {")
	for clusterName, structASTs := range clusters {
		if clusterName == "" {
			fmt.Fprintln(w, "  subgraph {")
		} else {
			fmt.Fprintf(w, "  subgraph cluster_%s {\n", clusterName)
			fmt.Fprintf(w, "    color=blue;")
			fmt.Fprintf(w, "    label=\"%s group\";\n", clusterName)
		}

		prefix := strings.Repeat(" ", 4)
		for _, str := range structASTs {
			fmt.Fprintf(w, "%s# %s\n", prefix, str.SrcFile)
			fmt.Fprintf(w, "%s%s [shape=record, label=<{<b>%s</b><br/>%s", prefix, str.Name, str.Name, str.SrcFile)
			for _, field := range str.Decl.Fields.List {
				if ignoreField(field.Names[0].Name) {
					continue // not needed
				}
				fieldName := normalizeName(field.Names[0].Name)
				fmt.Fprintf(w, ` | %s (%s)`, fieldName, braceEscaper.Replace(fmtType(field.Type, fset)))
			}
			fmt.Fprintf(w, "}>];\n")
		}

		fmt.Fprintln(w, "  }")
	}
	fmt.Fprintln(w, "}")
}

func fmtType(typ interface{}, fset *token.FileSet) string {
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, fset, typ)
	return buf.String()
}

var skipFieldList = []string{
	"modeluuid",
	"revno",
}

func ignoreField(name string) bool {
	name = strings.ToLower(name)

	for _, skipField := range skipFieldList {
		if strings.Contains(name, strings.ToLower(skipField)) {
			return true
		}
	}

	return false
}

func normalizeName(name string) string {
	var buf bytes.Buffer

	in := strings.TrimSuffix(name, "Doc")
	for i, r := range in {
		if i > 0 && unicode.IsUpper(r) && !unicode.IsUpper(rune(in[i-1])) {
			buf.WriteRune('_')
		}
		buf.WriteRune(unicode.ToLower(r))
	}

	return buf.String()
}
