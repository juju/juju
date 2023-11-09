// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

// bidirectional mapping between key and constant name
// e.g. "agent-ratelimit-max" <--> "AgentRateLimitMax"
// These are filled by iterating through the AST of config.go
var (
	keyForConstantName = map[string]string{}
	constantNameForKey = map[string]string{}
)

// Generate Markdown documentation based on the contents of the
// github.com/juju/juju/environs/config package.
func main() {
	outputDir := mustEnv("DOCS_DIR") // directory to write output to
	//jujuSrcRoot := mustEnv("JUJU_SRC_ROOT") // root of Juju source tree

	data := map[string]*keyInfo{}

	// Gather information from various places
	fillFromSchema(data)

	_ = os.MkdirAll(outputDir, 0755)
	render(filepath.Join(outputDir, "model-config-keys.md"), data)
}

// keyInfo contains information about a config key.
type keyInfo struct {
	Key          string `yaml:"key"`           // e.g. "agent-ratelimit-max"
	ConstantName string `yaml:"constant-name"` // e.g. "AgentRateLimitMax"
	Type         string `yaml:"type,omitempty"`
	Doc          string `yaml:"doc,omitempty"` // from parsing comments in config.go
	Mutable      bool   `yaml:"mutable"`       // from AllowedUpdateConfigAttributes
	Deprecated   bool   `yaml:"deprecated,omitempty"`

	// Several ways of getting the default value
	Default  string `yaml:"default,omitempty"`  // from instantiating NewConfig
	Default2 string `yaml:"default2,omitempty"` // from reflection on Config type

	SetByJuju   bool
	ValidValues []string
}

// render turns the input data into a Markdown document
func render(filepath string, data map[string]*keyInfo) {
	// Generate table of contents and main doc separately
	var tableOfContents, mainDoc string

	anchorForKey := func(key string) string {
		return "heading--" + key
	}
	headingForKey := func(key string) string {
		return fmt.Sprintf(`<a href="#%[1]s"><h2 id="%[1]s"><code>%[2]s</code></h2></a>`,
			anchorForKey(key), key)
	}

	// Sort keys
	var keys []string
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		info := data[key]

		tableOfContents += fmt.Sprintf("- [`%s`](#%s)\n", key, anchorForKey(key))
		mainDoc += headingForKey(key) + "\n"
		if info.Deprecated {
			mainDoc += "> This key is deprecated.\n"
		}
		mainDoc += "\n"

		if info.SetByJuju {
			mainDoc += "*Note: This value is set by Juju.*\n\n"
		}

		if info.Doc != "" {
			// Ensure doc has fullstop/newlines at end
			mainDoc += strings.TrimRight(info.Doc, ".\n") + ".\n\n"
		}
		if info.Type != "" {
			mainDoc += "**Type:** " + info.Type + "\n\n"
		}
		if defaultVal, ok := firstNonzero(info.Default, info.Default2); ok {
			mainDoc += "**Default value:** " + defaultVal + "\n\n"
		}
		if len(info.ValidValues) > 0 {
			mainDoc += "**Valid values:** " + strings.Join(info.ValidValues, ", ") + "\n\n"
		}

		if !info.Mutable {
			mainDoc += "*Note: This value cannot be changed after model creation.* "
		}
		mainDoc += "\n\n\n"
	}

	err := os.WriteFile(filepath, []byte(fmt.Sprintf(`
> <small> [Configuration > Model configuration](/t/1155#heading--configure-a-model) > List of model configuration keys</small>
>
> See also: [Model](/t/5456), [How to configure a model](/t/1155#heading--configure-a-model)

%s

%s
`[1:], tableOfContents, mainDoc)), 0644)
	check(err)
}

// Get data from config.Schema.
func fillFromSchema(data map[string]*keyInfo) {
	schema, err := config.Schema(nil)
	check(err)

	for key, attr := range schema {
		ensureDefined(data, key)

		if attr.Group == environschema.JujuGroup {
			data[key].SetByJuju = true
		}

		data[key].Doc = attr.Description
		data[key].Type = string(attr.Type)
		data[key].Mutable = !attr.Immutable

		for _, val := range attr.Values {
			data[key].ValidValues = append(data[key].ValidValues, fmt.Sprint(val))
		}
	}
}

// Gather information from the AST parsed from the Go files:
// ConstantName, Doc, Deprecated, Type
func fillFromAST(data map[string]*keyInfo, jujuSrcRoot string) {
	configPkgPath := filepath.Join(jujuSrcRoot, "environs/config")

	// Parse model config package into ASTs
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, configPkgPath, nil, parser.ParseComments)
	check(err)

	configDotGo := pkgs["config"].Files[filepath.Join(configPkgPath, "config.go")]
	var configKeys *ast.GenDecl
out:
	for _, v := range configDotGo.Decls {
		decl, ok := v.(*ast.GenDecl)
		if !ok {
			continue
		}
		if decl.Doc == nil {
			continue
		}
		for _, comment := range decl.Doc.List {
			if strings.Contains(comment.Text, "docs:model-config-keys") {
				configKeys = decl
				break out
			}
		}
	}
	if configKeys == nil {
		panic("unable to find const block with comment docs:model-config-keys")
	}

	comments := map[string]string{}
	// "keys" which should be ignored
	ignoreKeys := map[string]struct{}{"ReadOnlyMethods": {}}

	for _, spec := range configKeys.Specs {
		valueSpec := spec.(*ast.ValueSpec)
		key := strings.Trim(valueSpec.Values[0].(*ast.BasicLit).Value, `"`)
		if _, ok := ignoreKeys[key]; ok {
			continue
		}

		var comment string
		for _, astComment := range valueSpec.Doc.List {
			comment += strings.TrimPrefix(astComment.Text, "// ") + "\n"
		}

		constantName := valueSpec.Names[0].Name
		keyForConstantName[constantName] = key
		constantNameForKey[key] = constantName
		comments[key] = comment
	}

	// Put information in data map
	for key, comment := range comments {
		// Replace constant names with their actual values
		// e.g. AgentRateLimitMax --> `agent-ratelimit-max`

		// Some constant names are substrings of others. To ensure we replace
		// correctly, sort the names in descending length order first.
		constantNames := getKeysInDescLenOrder(keyForConstantName)
		for _, constantName := range constantNames {
			replaceKey := keyForConstantName[constantName]
			comment = strings.ReplaceAll(comment, constantName, fmt.Sprintf("`%s`", replaceKey))
		}

		ensureDefined(data, key)
		data[key].ConstantName = constantNameForKey[key]
		data[key].Doc = comment

		if strings.Contains(comment, "deprecated") || strings.Contains(comment, "Deprecated") {
			data[key].Deprecated = true
		}
	}

	// Pass over to configChecker AST to get types
	fillFromConfigCheckerAST(data,
		pkgs["config"].Files[filepath.Join(configPkgPath, "configschema.go")].Decls[1].(*ast.GenDecl),
	)
}

// Get key types from parsed configChecker in configschema.go
func fillFromConfigCheckerAST(data map[string]*keyInfo, configChecker *ast.GenDecl) {
	v := configChecker.Specs[0].(*ast.ValueSpec).Values[0].(*ast.CallExpr).Args
	schemaFields := v[0].(*ast.CompositeLit)

	// get key types from schemaFields
	for _, elt := range schemaFields.Elts {
		kvExpr := elt.(*ast.KeyValueExpr)
		constantName := kvExpr.Key.(*ast.Ident).Name
		key := keyForConstantName[constantName]

		ensureDefined(data, key)
		data[key].Type = typeForExpr(kvExpr.Value)
	}
}

// get type from configChecker expressions
func typeForExpr(expr ast.Expr) string {
	niceNames := map[string]string{
		"Bool":         "boolean",
		"ForceInt":     "integer",
		"List":         "list",
		"String":       "string",
		"TimeDuration": "duration",
	}
	niceNameFor := func(rawType string) string {
		if nn, ok := niceNames[rawType]; ok {
			return nn
		}
		return rawType
	}

	callExpr := expr.(*ast.CallExpr)
	rawType := callExpr.Fun.(*ast.SelectorExpr).Sel.Name
	dataType := niceNameFor(rawType)

	if len(callExpr.Args) > 0 {
		// add parameter types
		dataType += "["
		for i, arg := range callExpr.Args {
			if i > 0 {
				dataType += ", "
			}
			dataType += typeForExpr(arg)
		}
		dataType += "]"
	}

	return dataType
}

// Check whether key is mutable in AllowedUpdateConfigAttributes slice
func fillFromAllowedUpdateConfigAttributes(data map[string]*keyInfo) {
	for key := range controller.AllowedUpdateConfigAttributes {
		ensureDefined(data, key)
		data[key].Mutable = true
	}
}

// keys for which a default value doesn't make sense
var skipDefault = set.NewStrings(
// controller.AuditLogExcludeMethods, // "[ReadOnlyMethods]" - not useful
// controller.CACertKey,
// controller.ControllerUUIDKey,
)

// Get default values using a new on config.Config type
func fillFromNewConfig(data map[string]*keyInfo) {
	config, err := config.New(true, nil)
	check(err)
	_ = config
	for key, defaultVal := range map[string]any{} /*config.defined*/ {
		if skipDefault.Contains(key) {
			continue
		}

		ensureDefined(data, key)
		data[key].Default = fmt.Sprint(defaultVal)
	}
}

// Get default values using reflection on controller.Config type
// Used as a fallback where fillFromNewConfig can't produce a value
func fillFromConfigType(data map[string]*keyInfo) {
	// Don't get defaults from these methods - generally bogus values
	skipMethods := set.NewStrings(
	//"CAASImageRepo",
	//"CAASOperatorImagePath",
	//"ControllerAPIPort",
	//"Features",
	//"IdentityPublicKey",
	//"Validate", // not a config key
	)

	constantNameForMethod := func(methodName string) string {
		name := strings.TrimSuffix(methodName, "MB")

		rename := map[string]string{
			"NUMACtlPreference": "SetNUMAControlPolicyKey",
		}
		if rn, ok := rename[name]; ok {
			name = rn
		}

		return name
	}

	config, err := controller.NewConfig(testing.ControllerTag.Id(), testing.CACert, nil)
	check(err)
	t := reflect.TypeOf(config)
	v := reflect.ValueOf(config)

	for i := 0; i < t.NumMethod(); i++ {
		method := t.Method(i)
		methodValue := v.Method(i)

		if skipMethods.Contains(method.Name) {
			continue
		}
		if method.Type.NumIn() == 1 {
			defaultVal := methodValue.Call([]reflect.Value{})[0]

			constantName := constantNameForMethod(method.Name)
			key, ok := keyForConstantName[constantName]
			if !ok {
				// Try adding "Key" suffix
				key, ok = keyForConstantName[constantName+"Key"]
				if !ok {
					panic(method.Name)
				}
			}
			if skipDefault.Contains(key) {
				continue
			}

			ensureDefined(data, key)
			data[key].Default2 = fmt.Sprint(defaultVal)
		}
	}
}

// UTILITY FUNCTIONS

// Returns the value of the given environment variable, panicking if the var
// is not set.
func mustEnv(key string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		panic(fmt.Sprintf("env var %q not set", key))
	}
	return val
}

// Return the first value that is defined / not-zero, and "true"
// if such a value is found.
func firstNonzero(vals ...string) (string, bool) {
	for _, val := range vals {
		if val != "" {
			return val, true
		}
	}
	return "", false
}

// Ensure that the data map has an entry for key.
func ensureDefined(data map[string]*keyInfo, key string) {
	if data[key] == nil {
		data[key] = &keyInfo{
			Key: key,
		}
	}
}

// check panics if the provided error is nil.
func check(err error) {
	if err != nil {
		panic(err)
	}
}

// return keys of the given map in descending length order
func getKeysInDescLenOrder[T any](m map[string]T) (keys []string) {
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})
	return
}
