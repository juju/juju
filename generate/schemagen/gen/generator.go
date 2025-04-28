// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gen

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"sort"
	"strings"

	"github.com/juju/errors"
	"golang.org/x/tools/go/packages"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/generate/schemagen/jsonschema-gen"
	"github.com/juju/juju/internal/rpcreflect"
)

type APIServer interface {
	AllFacades() Registry
	AdminFacadeDetails() []facade.Details
}

type Registry interface {
	List() []facade.Description
	ListDetails() []facade.Details
	GetType(name string, version int) (reflect.Type, error)
}

type PackageRegistry interface {
	LoadPackage() (*packages.Package, error)
}

// Option to be passed to Connect to customize the resulting instance.
type Option func(*options)

type options struct {
	adminFacades bool
	facadeGroups []FacadeGroup
}

func newOptions() *options {
	return &options{
		adminFacades: false,
		facadeGroups: []FacadeGroup{Latest},
	}
}

// WithAdminFacades sets the adminFacades on the option
func WithAdminFacades(adminFacades bool) Option {
	return func(options *options) {
		options.adminFacades = adminFacades
	}
}

// WithFacadeGroups sets the facadeGroups on the option
func WithFacadeGroups(facadeGroups []FacadeGroup) Option {
	return func(options *options) {
		options.facadeGroups = facadeGroups
	}
}

var (
	structType = reflect.TypeOf(struct{}{})
)

// Generate a FacadeSchema from the APIServer
func Generate(pkgRegistry PackageRegistry, client APIServer, options ...Option) ([]FacadeSchema, error) {
	opts := newOptions()
	for _, option := range options {
		option(opts)
	}

	pkg, err := pkgRegistry.LoadPackage()
	if err != nil {
		return nil, errors.Trace(err)
	}

	registry := client.AllFacades()
	facades := registry.ListDetails()

	if opts.adminFacades {
		adminFacades := client.AdminFacadeDetails()
		facades = append(facades, adminFacades...)
	}

	// Compose all the facade groups together.
	var groupFacades [][]facade.Details
	for _, group := range opts.facadeGroups {
		groupFacades = append(groupFacades, Filter(group, facades, registry))
	}

	unique := make(map[string]facade.Details)
	for _, list := range groupFacades {
		for _, f := range list {
			// Ensure that we create a unique namespace so that any facades that
			// are composed together are repeated.
			unique[fmt.Sprintf("%s:%d", f.Name, f.Version)] = f
		}
	}
	facades = make([]facade.Details, 0, len(unique))
	for _, f := range unique {
		facades = append(facades, f)
	}
	sort.Slice(facades, func(i, j int) bool {
		if facades[i].Name < facades[j].Name {
			return true
		}
		if facades[i].Name > facades[j].Name {
			return false
		}
		return facades[i].Version < facades[j].Version
	})

	result := make([]FacadeSchema, len(facades))
	for i, facade := range facades {
		// select the latest version from the facade list
		version := facade.Version

		result[i].Name = facade.Name
		result[i].Version = version

		var objType *rpcreflect.ObjType
		kind, err := registry.GetType(facade.Name, version)
		if err == nil {
			objType = rpcreflect.ObjTypeOf(kind)
		} else {
			objType = rpcreflect.ObjTypeOf(facade.Type)
			if objType == nil {
				return nil, errors.Annotatef(err, "getting type for facade %s at version %d", facade.Name, version)
			}
		}

		if objType != nil {
			for _, method := range objType.MethodNames() {
				m, err := objType.Method(method)
				if err != nil {
					continue
				}
				if m.Params == structType && m.Result == nil {
					return nil, errors.Errorf("method %q on facade %q has unexpected params. If you're trying to hide the method, use `func (_, _ struct{})`.", method, facade.Name)
				}
			}
		}
		result[i].Schema = jsonschema.ReflectFromObjType(objType)

		if pkg == nil {
			continue
		}

		// Attempt to get the documentation from the code.

		pt, err := progType(pkg, objType.GoType())
		if err != nil {
			return nil, errors.Trace(err)
		}
		doc, err := typeDocComment(pkg, pt)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot get doc comment for %v", objType.GoType())
		}
		result[i].Description = strings.TrimSpace(doc)

		for _, name := range objType.MethodNames() {
			for propName, prop := range result[i].Schema.Properties {
				if name == propName {
					doc, err := methodDocComment(pkg, pt, name)
					if err != nil {
						return nil, errors.Annotatef(err, "cannot get doc comment for %v: %v", objType.GoType(), name)
					}
					prop.Description = strings.TrimSpace(doc)
				}
			}
		}
	}

	return result, nil
}

type FacadeSchema struct {
	Name        string
	Description string
	Version     int
	Schema      *jsonschema.Schema
}

func progType(pkg *packages.Package, t reflect.Type) (*types.TypeName, error) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	typeName := t.Name()
	if typeName == "" {
		return nil, errors.Errorf("type %s is not named", t)
	}
	pkgPath := t.PkgPath()
	if pkgPath == "" {
		// TODO could return types.Basic type here if we needed to.
		return nil, errors.Errorf("type %s not declared in package", t)
	}

	var found *packages.Package
	packages.Visit([]*packages.Package{pkg}, func(pkg *packages.Package) bool {
		if pkg.PkgPath == pkgPath {
			found = pkg
			return false
		}
		return true
	}, nil)
	if found == nil {
		return nil, errors.Errorf("cannot find %q in imported code", pkgPath)
	}

	obj := found.Types.Scope().Lookup(typeName)
	if obj == nil {
		return nil, errors.Errorf("type %s not found in %s", typeName, pkgPath)
	}
	objTypeName, ok := obj.(*types.TypeName)
	if !ok {
		return nil, errors.Errorf("%s is not a type", typeName)
	}
	return objTypeName, nil
}

func typeDocComment(pkg *packages.Package, t *types.TypeName) (string, error) {
	decl, err := findDecl(pkg, t.Pos())
	if err != nil {
		return "", errors.Trace(err)
	}
	tdecl, ok := decl.(*ast.GenDecl)
	if !ok || tdecl.Tok != token.TYPE {
		return "", errors.Errorf("found non-type decl %#v", decl)
	}
	for _, spec := range tdecl.Specs {
		tspec := spec.(*ast.TypeSpec)
		if tspec.Name.Pos() == t.Pos() {
			if tspec.Doc != nil {
				return tspec.Doc.Text(), nil
			}
			return tdecl.Doc.Text(), nil
		}
	}
	return "", errors.Errorf("cannot find type declaration")
}

func methodDocComment(pkg *packages.Package, tname *types.TypeName, methodName string) (string, error) {
	t := tname.Type()
	if !types.IsInterface(t) {
		// Use the pointer type to get as many methods as possible.
		t = types.NewPointer(t)
	}

	mset := types.NewMethodSet(t)
	sel := mset.Lookup(nil, methodName)
	if sel == nil {
		return "", errors.Errorf("cannot find method %v on %v", methodName, t)
	}
	obj := sel.Obj()
	decl, err := findDecl(pkg, obj.Pos())
	if err != nil {
		return "", errors.Trace(err)
	}
	switch decl := decl.(type) {
	case *ast.GenDecl:
		if decl.Tok != token.TYPE {
			return "", errors.Errorf("found non-type decl %#v", decl)
		}
		for _, spec := range decl.Specs {
			tspec := spec.(*ast.TypeSpec)
			it := tspec.Type.(*ast.InterfaceType)
			for _, m := range it.Methods.List {
				for _, id := range m.Names {
					if id.Pos() == obj.Pos() {
						return m.Doc.Text(), nil
					}
				}
			}
		}
		return "", errors.Errorf("method definition not found in type")
	case *ast.FuncDecl:
		if decl.Name.Pos() != obj.Pos() {
			return "", errors.Errorf("method definition not found (at %#v)", pkg.Fset.Position(obj.Pos()))
		}
		return decl.Doc.Text(), nil
	default:
		return "", errors.Errorf("unexpected declaration %T found", decl)
	}
}

// findDecl returns the top level declaration that contains the
// given position.
func findDecl(pkg *packages.Package, pos token.Pos) (ast.Decl, error) {
	tokFile := pkg.Fset.File(pos)
	if tokFile == nil {
		return nil, errors.Errorf("no file found for object")
	}
	filename := tokFile.Name()
	var found ast.Decl
	packages.Visit([]*packages.Package{pkg}, func(pkg *packages.Package) bool {
		for _, f := range pkg.Syntax {
			if tokFile := pkg.Fset.File(f.Pos()); tokFile == nil || tokFile.Name() != filename {
				continue
			}
			// We've found the file we're looking for. Now traverse all
			// top level declarations looking for the right function declaration.
			for _, decl := range f.Decls {
				if decl.Pos() <= pos && pos <= decl.End() {
					found = decl
					return false
				}
			}
		}
		return true
	}, nil)
	if found == nil {
		return nil, errors.Errorf("declaration not found")
	}
	return found, nil
}
