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
	"runtime"
	"strings"
	stdtesting "testing"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

var disallowedCalls = set.NewStrings(
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
)

var allowedCalls = map[string]set.Strings{
	// Used for checking for new Juju 2 installs.
	"juju/commands/main.go": set.NewStrings("Stat"),
	// upgrade-model is not a whitelisted embedded CLI command.
	"juju/commands/upgrademodel.go": set.NewStrings("Open", "RemoveAll"),
	// ssh is not a whitelisted embedded CLI command.
	"juju/commands/ssh_machine.go": set.NewStrings("Remove"),
	// upgrade-gui is not a whitelisted embedded CLI command.
	"juju/gui/upgradegui.go": set.NewStrings("Remove"),
	// Ignore the actual os calls.
	"modelcmd/filesystem.go": set.NewStrings("*"),
	// signmetadata is not a whitelisted embedded CLI command.
	"plugins/juju-metadata/signmetadata.go": set.NewStrings("Open"),
}

var ignoredPackages = set.NewStrings(
	"jujuc", "jujud", "ks8agent", "juju-bridge", "service")

type OSCallTest struct{}

var _ = gc.Suite(&OSCallTest{})

// TestNoDirectFilesystemAccess ensures Juju CLI commands do
// not directly access the filesystem va the "os" package.
// This ensures embedded commands do not accidentally bypass
// the restrictions to filesystem access.
func (s *OSCallTest) TestNoDirectFilesystemAccess(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("not needed on Windows, checking for imports on Ubuntu sufficient")
	}
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(calls, gc.HasLen, 0)
}

func (s *OSCallTest) parseDir(fset *token.FileSet, calls map[string]set.Strings, dir string) {
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		fmt.Println(err)
		return
	}

	for _, pkg := range pkgs {
		for fName, f := range pkg.Files {
			osImportAliases := set.NewStrings("os")
			// Ensure we also capture os calls where the import tis aliased.
			for _, imp := range f.Imports {
				if imp.Name == nil {
					continue
				}
				if imp.Name.Name != "" && imp.Path.Value == `"os"` {
					osImportAliases.Add(imp.Name.Name)
				}
			}
			s.parseFile(f, fName, osImportAliases, calls)
		}
	}
}

func (*OSCallTest) parseFile(f *ast.File, fName string, osImportAliases set.Strings, calls map[string]set.Strings) {
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

				if !disallowedCalls.Contains(expr.Sel.Name) {
					return false
				}
				if allowed, ok := allowedCalls[fName]; ok {
					if allowed.Contains("*") || allowed.Contains(expr.Sel.Name) {
						return false
					}
				}
				pkg := strings.Split(fName, string(os.PathSeparator))[0]
				if ignoredPackages.Contains(pkg) {
					return false
				}
				exprIdent := expr.X.(*ast.Ident)
				if osImportAliases.Contains(exprIdent.Name) {
					funcs := calls[fName]
					if funcs == nil {
						funcs = set.NewStrings()
					}
					funcs.Add(fmt.Sprintf("%v.%v()", exprIdent.Name, expr.Sel.Name))
					calls[fName] = funcs
				}
				return false
			})
		}
	}
}
