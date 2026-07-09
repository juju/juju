// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

// generateTransforms walks adjacent pairs in versions and produces a
// transform package per pair plus the top-level registered.go file that
// wires them together. Existing deltas.go files are left untouched.
func generateTransforms(versions []string) error {
	for i := 0; i < len(versions)-1; i++ {
		from, to := versions[i], versions[i+1]
		if err := generateTransformPair(from, to); err != nil {
			return fmt.Errorf("transform %s -> %s: %w", from, to, err)
		}
	}
	return writeRegisteredFile(versions)
}

// generateTransformPair emits transform.go (always overwritten) and an
// initial deltas.go stub (only if no deltas.go exists) for a single
// (from -> to) step.
func generateTransformPair(from, to string) error {
	srcStructs, err := parseModelTypes(modelPathFor(from))
	if err != nil {
		return fmt.Errorf("parsing source types: %w", err)
	}
	tgtStructs, err := parseModelTypes(modelPathFor(to))
	if err != nil {
		return fmt.Errorf("parsing target types: %w", err)
	}

	c := classifyFields(srcStructs, tgtStructs)

	if err := writeTransformFile(from, to, c); err != nil {
		return fmt.Errorf("writing transform.go: %w", err)
	}
	if err := writeTransformDocFile(from, to); err != nil {
		return fmt.Errorf("writing doc.go: %w", err)
	}
	return writeDeltasFileIfAbsent(from, to)
}

// structMap indexes a package's struct types by name.
type structMap map[string]*ast.StructType

// parseModelTypes parses a generated model.go file and returns its
// struct types keyed by name.
func parseModelTypes(path string) (structMap, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	out := structMap{}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			out[ts.Name.Name] = st
		}
	}
	return out, nil
}

// structSignature returns a canonical "name#type;name#type;..." string
// for a struct. A signature mismatch between two versions of the same
// struct is treated as a schema change. This detects additions,
// removals, reorders, type renames, and ptr<->non-ptr flips.
func structSignature(st *ast.StructType) string {
	if st == nil {
		return ""
	}
	var parts []string
	for _, fld := range st.Fields.List {
		typeStr := renderTypeExpr(fld.Type)
		if len(fld.Names) == 0 {
			// Embedded field; not used in export types, but handle it
			// gracefully by including an anonymous slot.
			parts = append(parts, "_#"+typeStr)
			continue
		}
		for _, name := range fld.Names {
			parts = append(parts, name.Name+"#"+typeStr)
		}
	}
	return strings.Join(parts, ";")
}

// renderTypeExpr pretty-prints a type expression. The go/printer output
// is stable enough across runs to use as a comparison key.
func renderTypeExpr(expr ast.Expr) string {
	var buf bytes.Buffer
	fset := token.NewFileSet()
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		return fmt.Sprintf("%T", expr)
	}
	return buf.String()
}

// fieldRef names a ModelExport field and the element type of its slice.
type fieldRef struct {
	name string
	elem string
}

// modelExportFields returns the ModelExport field list in declaration
// order. Non-slice fields or slices whose element is not an identifier
// are skipped (not expected in generated types, but defensive).
func modelExportFields(st *ast.StructType) []fieldRef {
	if st == nil {
		return nil
	}
	var out []fieldRef
	for _, fld := range st.Fields.List {
		arr, ok := fld.Type.(*ast.ArrayType)
		if !ok {
			continue
		}
		ident, ok := arr.Elt.(*ast.Ident)
		if !ok {
			continue
		}
		for _, name := range fld.Names {
			out = append(out, fieldRef{name: name.Name, elem: ident.Name})
		}
	}
	return out
}

// classified describes the per-field decisions the generator makes when
// bridging two schema format versions.
type classified struct {
	// Unchanged names ModelExport fields whose element struct shape is
	// identical in both versions. They are copied via Go type conversion.
	Unchanged []string

	// Changed names fields that exist in both versions but whose element
	// struct shape changed. The engineer must implement a Deltas method
	// for each.
	Changed []changedField

	// New names fields that exist only in the target. The engineer must
	// implement a Deltas method that synthesises them (possibly from
	// other source tables via the full ModelExport, or via deps).
	New []newField

	// Removed names fields that exist only in the source. They appear as
	// comments in the generated transform.
	Removed []string
}

type changedField struct {
	// Name is the field name in ModelExport (== element type name).
	Name    string
	SrcType string // element type name in source package
	DstType string // element type name in target package
}

type newField struct {
	Name    string
	DstType string
}

func classifyFields(src, tgt structMap) classified {
	var c classified
	srcMx := src["ModelExport"]
	tgtMx := tgt["ModelExport"]
	if srcMx == nil || tgtMx == nil {
		return c
	}

	tgtFields := modelExportFields(tgtMx)
	srcFields := modelExportFields(srcMx)

	srcByName := make(map[string]string, len(srcFields))
	for _, f := range srcFields {
		srcByName[f.name] = f.elem
	}
	tgtByName := make(map[string]bool, len(tgtFields))
	for _, f := range tgtFields {
		tgtByName[f.name] = true
	}

	for _, tf := range tgtFields {
		srcElem, inSource := srcByName[tf.name]
		if !inSource {
			c.New = append(c.New, newField{Name: tf.name, DstType: tf.elem})
			continue
		}
		if structSignature(src[srcElem]) == structSignature(tgt[tf.elem]) {
			c.Unchanged = append(c.Unchanged, tf.name)
		} else {
			c.Changed = append(c.Changed, changedField{
				Name: tf.name, SrcType: srcElem, DstType: tf.elem,
			})
		}
	}
	for _, sf := range srcFields {
		if !tgtByName[sf.name] {
			c.Removed = append(c.Removed, sf.name)
		}
	}
	return c
}

// writeTransformFile renders and writes the transform.go file for the
// (from -> to) pair. The file is always overwritten; it is generator-owned.
func writeTransformFile(from, to string, c classified) error {
	dir := transformDirFor(from, to)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmplBytes, err := os.ReadFile(filepath.Join(generatorDir(), "transform.tmpl"))
	if err != nil {
		return err
	}

	data := struct {
		FromToken  string
		ToToken    string
		From       string
		To         string
		UsesDeltas bool
		Unchanged  []string
		Removed    []string
		Changed    []changedField
		New        []newField
	}{
		FromToken:  versionToken(from),
		ToToken:    versionToken(to),
		From:       from,
		To:         to,
		UsesDeltas: len(c.Changed) > 0 || len(c.New) > 0,
		Unchanged:  c.Unchanged,
		Removed:    c.Removed,
		Changed:    c.Changed,
		New:        c.New,
	}

	t := template.Must(template.New("transform").Parse(string(tmplBytes)))
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return err
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("gofmt transform.go: %w\n--- source ---\n%s", err, buf.String())
	}

	path := filepath.Join(dir, "transform.go")
	fmt.Printf("writing to %s\n", path)
	return os.WriteFile(path, formatted, 0644)
}

// writeTransformDocFile renders and writes the doc.go file for the
// (from -> to) pair. The file is always overwritten; it is generator-owned.
func writeTransformDocFile(from, to string) error {
	dir := transformDirFor(from, to)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmplBytes, err := os.ReadFile(filepath.Join(generatorDir(), "transform_doc.tmpl"))
	if err != nil {
		return err
	}

	data := struct {
		FromToken string
		ToToken   string
		From      string
		To        string
	}{
		FromToken: versionToken(from),
		ToToken:   versionToken(to),
		From:      from,
		To:        to,
	}

	t := template.Must(template.New("transform_doc").Parse(string(tmplBytes)))
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return err
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("gofmt doc.go: %w\n--- source ---\n%s", err, buf.String())
	}

	path := filepath.Join(dir, "doc.go")
	fmt.Printf("writing to %s\n", path)
	return os.WriteFile(path, formatted, 0644)
}

// writeDeltasFileIfAbsent writes the initial deltas.go stub only if the
// file does not already exist. deltas.go is engineer-owned; the
// generator never overwrites it.
func writeDeltasFileIfAbsent(from, to string) error {
	dir := transformDirFor(from, to)
	path := filepath.Join(dir, "deltas.go")
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("skipping existing %s\n", path)
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	tmplBytes, err := os.ReadFile(filepath.Join(generatorDir(), "deltas.tmpl"))
	if err != nil {
		return err
	}

	data := struct {
		FromToken string
		ToToken   string
		From      string
		To        string
	}{
		FromToken: versionToken(from),
		ToToken:   versionToken(to),
		From:      from,
		To:        to,
	}

	t := template.Must(template.New("deltas").Parse(string(tmplBytes)))
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return err
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("gofmt deltas.go: %w", err)
	}

	fmt.Printf("writing to %s\n", path)
	return os.WriteFile(path, formatted, 0644)
}

// writeRegisteredFile emits the domain/modelimport/registered.go file
// listing every (from, to) pair registration. This file is fully
// generated; the hand-written modelimport.go provides NewTransformer.
func writeRegisteredFile(versions []string) error {
	dir := modelimportDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmplBytes, err := os.ReadFile(filepath.Join(generatorDir(), "registered.tmpl"))
	if err != nil {
		return err
	}

	type pairData struct {
		FromToken string
		ToType    string
		From      string
		To        string
		PairPkg   string
	}

	// Collect unique non-final type-version tokens (deduped) for imports.
	// The final transform registers its destination as latest.ModelExport,
	// creating a compile-time guard that latest tracks the transformer target.
	seen := map[string]bool{}
	var typeVersions []string
	for i, v := range versions {
		if i == len(versions)-1 {
			break
		}
		tok := versionToken(v)
		if !seen[tok] {
			seen[tok] = true
			typeVersions = append(typeVersions, tok)
		}
	}

	var pairs []pairData
	for i := 0; i < len(versions)-1; i++ {
		from, to := versions[i], versions[i+1]
		fromTok, toTok := versionToken(from), versionToken(to)
		toType := toTok + ".ModelExport"
		if i == len(versions)-2 {
			toType = "latest.ModelExport"
		}
		pairs = append(pairs, pairData{
			FromToken: fromTok,
			ToType:    toType,
			From:      from,
			To:        to,
			PairPkg:   "to_" + toTok,
		})
	}

	data := struct {
		TypeVersions []string
		Pairs        []pairData
		HasPairs     bool
	}{
		TypeVersions: typeVersions,
		Pairs:        pairs,
		HasPairs:     len(pairs) > 0,
	}

	t := template.Must(template.New("registered").Parse(string(tmplBytes)))
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return err
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("gofmt registered.go: %w\n--- source ---\n%s", err, buf.String())
	}

	path := filepath.Join(dir, "registered.go")
	fmt.Printf("writing to %s\n", path)
	return os.WriteFile(path, formatted, 0644)
}

// versionToken converts "4.0.12" to "v4_0_12" (matches directory and
// package naming of generated types packages).
func versionToken(v string) string {
	return "v" + strings.ReplaceAll(v, ".", "_")
}

// generatorDir returns the directory containing the generator source files.
func generatorDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filename)
}

// repoRoot returns the repository root based on this file's location.
func repoRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	// generate/export/transforms.go -> generate/export -> generate -> repo root
	return filepath.Dir(filepath.Dir(filepath.Dir(filename)))
}

func modelPathFor(version string) string {
	return filepath.Join(repoRoot(), "domain", "export", "types", versionToken(version), "model.go")
}

// transformDirFor returns the directory for the transform package that
// targets the given destination version. The source version is implicit
// from its position in ExportVersions, so it is omitted from the name.
func transformDirFor(_, to string) string {
	return filepath.Join(repoRoot(), "domain", "modelimport", "transformer", "transforms",
		"to_"+versionToken(to))
}

func modelimportDir() string {
	return filepath.Join(repoRoot(), "domain", "modelimport")
}
