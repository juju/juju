// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package gen

import (
	"go/ast"
	"go/token"
	"go/types"
	"reflect"

	"github.com/juju/errors"
	jsonschema "github.com/juju/jsonschema-gen"
	"github.com/juju/rpcreflect"
	"golang.org/x/tools/go/packages"
	"gopkg.in/errgo.v1"

	"github.com/juju/juju/apiserver/facade"
)

//go:generate mockgen -package gen -destination describeapi_mock.go github.com/juju/juju/generate/schemagen/gen APIServer,Registry,PackageRegistry,Linker
type APIServer interface {
	AllFacades() Registry
}

type Registry interface {
	List() []facade.Description
	ListDetails() []facade.Details
	GetType(name string, version int) (reflect.Type, error)
}

type PackageRegistry interface {
	LoadPackage() (*packages.Package, error)
}

type Linker interface {
	Links(string, facade.Factory) []string
}

// Generate a FacadeSchema from the APIServer
func Generate(pkgRegistry PackageRegistry, linker Linker, client APIServer) ([]FacadeSchema, error) {
	pkg, err := pkgRegistry.LoadPackage()
	if err != nil {
		return nil, errors.Trace(err)
	}

	registry := client.AllFacades()
	facades := registry.ListDetails()
	result := make([]FacadeSchema, len(facades))
	for i, facade := range facades {
		// select the latest version from the facade list
		version := facade.Version

		result[i].Name = facade.Name
		result[i].Version = version
		result[i].AvailableTo = linker.Links(facade.Name, facade.Factory)

		kind, err := registry.GetType(facade.Name, version)
		if err != nil {
			return nil, errors.Annotatef(err, "getting type for facade %s at version %d", facade.Name, version)
		}
		objType := rpcreflect.ObjTypeOf(kind)

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
		result[i].Description = doc

		for _, name := range objType.MethodNames() {
			for propName, prop := range result[i].Schema.Properties {
				if name == propName {
					doc, err := methodDocComment(pkg, pt, name)
					if err != nil {
						return nil, errors.Annotatef(err, "cannot get doc comment for %v: %v", objType.GoType(), name)
					}
					prop.Description = doc
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
	AvailableTo []string
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
		return "", errgo.Mask(err)
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
