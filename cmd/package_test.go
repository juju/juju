// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
)

var disallowedCalls = map[string]set.Strings{
	"os": set.NewStrings(
		"Chdir",
		"Chmod",
		"Chown",
		"Create",
		"Lchown",
		"Lstat",
		"Mkdir",
		"Open",
		"OpenFile",
		"Remove",
		"RemoveAll",
		"Rename",
		"TempDir",
		"Stat",
		"Symlink",
		"UserCacheDir",
		"UserConfigDir",
		"UserHomeDir",
	),
	"exec": set.NewStrings(
		"Command",
		"LookPath",
	),
	"net": set.NewStrings(
		"Dial",
	),
	"utils": set.NewStrings(
		"RunCommands",
	),
}

var allowedCalls = map[string]set.Strings{
	// Used for checking for new Juju 2 installs.
	"juju/commands/main.go": set.NewStrings("os.Stat"),
	// plugins are not enabled for embedded CLI commands.
	"juju/commands/plugin.go": set.NewStrings("exec.Command"),
	// upgrade-controller is not a whitelisted embedded CLI command.
	"juju/commands/upgradecontroller.go": set.NewStrings("os.Open", "os.RemoveAll"),
	// ssh is not a whitelisted embedded CLI command.
	"juju/ssh/ssh_machine.go": set.NewStrings("os.Remove"),
	// agree is not exposed to shell scripts because it uses PAGER to present terms
	"juju/agree/agree/agree.go": set.NewStrings("exec.Command", "exec.LookPath"),
	// Ignore the actual os calls.
	"modelcmd/filesystem.go": set.NewStrings("*"),
	// signmetadata is not a whitelisted embedded CLI command.
	"plugins/juju-metadata/signmetadata.go": set.NewStrings("os.Open"),
	// containeragent needs to ensure jujud symlinks.
	"containeragent/utils/filesystem.go": set.NewStrings("os.Stat", "os.Symlink", "os.OpenFile", "os.RemoveAll"),
}

var ignoredPackages = set.NewStrings(
	"jujuc", "jujud", "jujud-controller", "containeragent", "juju-bridge", "service", "internal",
)

type OSCallTest struct{}

func TestOSCallTest(t *stdtesting.T) {
	tc.Run(t, &OSCallTest{})
}

// TestNoRestrictedCalls ensures Juju CLI commands do
// not make restricted os level calls, namely:
// - directly access the filesystem via the "os" package
// - directly execute commands via the "exec" package
// This ensures embedded commands do not accidentally bypass
// the restrictions to filesystem or exec access.
func (s *OSCallTest) TestNoRestrictedCalls(c *tc.C) {
	fset := token.NewFileSet()
	calls := make(map[string]set.Strings)

	err := filepath.Walk(".",
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() || info.Name() == "mocks" {
				return nil
			}
			s.parseDir(fset, calls, path)
			return nil
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(calls, tc.HasLen, 0)
}

type callCheckContext struct {
	pkgName         string
	disallowedCalls set.Strings
	calls           map[string]set.Strings
}

func (s *OSCallTest) parseDir(fset *token.FileSet, calls map[string]set.Strings, dir string) {
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		return
	}

	for _, pkg := range pkgs {
		for fName, f := range pkg.Files {
			for pkgName, funcs := range disallowedCalls {
				ctx := &callCheckContext{
					pkgName:         pkgName,
					disallowedCalls: funcs,
					calls:           calls,
				}
				s.parsePackageFunctions(ctx, fName, f)
			}
		}
	}
}

func (s *OSCallTest) parsePackageFunctions(ctx *callCheckContext, fName string, f *ast.File) {
	osImportAliases := set.NewStrings(ctx.pkgName)
	// Ensure we also capture os calls where the import is aliased.
	for _, imp := range f.Imports {
		if imp.Name == nil {
			continue
		}
		if imp.Name.Name != "" && imp.Path.Value == fmt.Sprintf(`%q`, ctx.pkgName) {
			osImportAliases.Add(imp.Name.Name)
		}
	}
	s.parseFile(ctx, fName, f, osImportAliases)
}

func (*OSCallTest) parseFile(ctx *callCheckContext, fName string, f *ast.File, osImportAliases set.Strings) {
	for _, decl := range f.Decls {
		if decl, ok := decl.(*ast.FuncDecl); ok {
			ast.Inspect(decl.Body, func(n ast.Node) bool {
				switch n.(type) {
				case *ast.SelectorExpr:
				default:
					return true
				}
				expr := n.(*ast.SelectorExpr)
				switch expr.X.(type) {
				case *ast.CallExpr:
					return true
				case *ast.Ident:
				default:
					return false
				}

				if !ctx.disallowedCalls.Contains(expr.Sel.Name) {
					return false
				}
				if allowed, ok := allowedCalls[fName]; ok {
					if allowed.Contains("*") || allowed.Contains(ctx.pkgName+"."+expr.Sel.Name) {
						return false
					}
				}
				pkg := strings.Split(fName, string(os.PathSeparator))[0]
				if ignoredPackages.Contains(pkg) {
					return false
				}
				exprIdent := expr.X.(*ast.Ident)
				if osImportAliases.Contains(exprIdent.Name) {
					funcs := ctx.calls[fName]
					if funcs == nil {
						funcs = set.NewStrings()
					}
					funcs.Add(fmt.Sprintf("%v.%v()", exprIdent.Name, expr.Sel.Name))
					ctx.calls[fName] = funcs
				}
				return false
			})
		}
	}
}
