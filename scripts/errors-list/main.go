// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrorInfo represents information about an error constant
type ErrorInfo struct {
	Name          string
	Domain        string
	FilePath      string
	Documentation string
}

func main() {
	// Define and parse command-line flags
	sortFlag := flag.String("sort", "alph", "Sort errors by: 'alph' (alphabetically, default) or 'domain'")
	flag.Parse()

	// Determine the project directory (from remaining args after flags)
	projectDir := "."
	if flag.NArg() > 0 {
		projectDir = flag.Arg(0)
	}

	// Find all error files in the domain directory
	domainDir := filepath.Join(projectDir, "domain")
	errorFiles, err := findErrorFiles(domainDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding error files: %v\n", err)
		os.Exit(1)
	}

	// Parse each file and extract error constants
	var allErrors []ErrorInfo
	for _, file := range errorFiles {
		errors, err := parseErrorFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing file %s: %v\n", file, err)
			continue
		}
		allErrors = append(allErrors, errors...)
	}

	// Sort errors based on the sort flag
	switch *sortFlag {
	case "domain":
		// Sort by domain first, then by name
		sort.Slice(allErrors, func(i, j int) bool {
			if allErrors[i].Domain != allErrors[j].Domain {
				return allErrors[i].Domain < allErrors[j].Domain
			}
			return allErrors[i].Name < allErrors[j].Name
		})
	case "alph", "":
		// Sort alphabetically by name (default)
		sort.Slice(allErrors, func(i, j int) bool {
			return allErrors[i].Name < allErrors[j].Name
		})
	default:
		fmt.Fprintf(os.Stderr, "Invalid sort option: %s. Using default (alphabetical).\n", *sortFlag)
		sort.Slice(allErrors, func(i, j int) bool {
			return allErrors[i].Name < allErrors[j].Name
		})
	}

	// Generate markdown table
	markdown := generateMarkdownTable(allErrors)

	// Write to file
	outputPath := filepath.Join(projectDir, "errors-list.md")
	err = os.WriteFile(outputPath, []byte(markdown), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing markdown file: %v\n", err)
		os.Exit(1)
	}

	// Print absolute path for clarity
	absPath, _ := filepath.Abs(outputPath)
	fmt.Printf("Generated %s successfully\n", absPath)
}

// findErrorFiles finds all error files matching the pattern ./domain/**/errors/*.go
func findErrorFiles(root string) ([]string, error) {
	var files []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") &&
			strings.Contains(path, "/errors/") &&
			!strings.HasSuffix(path, "_test.go") &&
			!strings.HasSuffix(path, "doc.go") {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

// parseErrorFile parses a Go file and extracts error constants
func parseErrorFile(filePath string) ([]ErrorInfo, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var errors []ErrorInfo
	domain := extractDomain(filePath)

	// Find all const blocks
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}

		// Process each constant in the block
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			// Check if this is an error constant
			for i, name := range valueSpec.Names {
				if i >= len(valueSpec.Values) {
					continue
				}

				// Check if the value is a call to errors.ConstError
				callExpr, ok := valueSpec.Values[i].(*ast.CallExpr)
				if !ok {
					continue
				}

				// Check if it's calling errors.ConstError
				selectorExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
				if !ok {
					continue
				}

				ident, ok := selectorExpr.X.(*ast.Ident)
				if !ok || ident.Name != "errors" || selectorExpr.Sel.Name != "ConstError" {
					continue
				}

				// Extract documentation from comments
				var doc string
				if valueSpec.Doc != nil {
					doc = valueSpec.Doc.Text()
				} else if genDecl.Doc != nil && i == 0 {
					doc = genDecl.Doc.Text()
				}

				// Clean up documentation
				doc = cleanDocumentation(doc)

				// Add to errors list
				errors = append(errors, ErrorInfo{
					Name:          name.Name,
					Domain:        domain,
					FilePath:      filePath,
					Documentation: doc,
				})
			}
		}
	}

	return errors, nil
}

// extractDomain extracts the domain name from the file path
func extractDomain(filePath string) string {
	// Extract domain from path like ./domain/application/errors/errors.go
	parts := strings.Split(filePath, "/")
	for i, part := range parts {
		if part == "domain" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return "unknown"
}

// cleanDocumentation cleans up the documentation string
func cleanDocumentation(doc string) string {
	// Remove leading and trailing whitespace
	doc = strings.TrimSpace(doc)

	// Remove comment markers
	doc = strings.ReplaceAll(doc, "// ", "")
	doc = strings.ReplaceAll(doc, "//", "")

	// Join multiple lines with spaces
	lines := strings.Split(doc, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.Join(lines, " ")
}

// generateMarkdownTable generates a markdown table from the errors
func generateMarkdownTable(errors []ErrorInfo) string {
	var sb strings.Builder

	// Write table header
	sb.WriteString("# Juju Error Constants\n\n")
	sb.WriteString("| Error | Domain | Documentation |\n")
	sb.WriteString("| ---- | ---- | ---- |\n")

	// Write table rows
	for _, err := range errors {
		// Create relative path for the link
		relPath := err.FilePath
		// Make the path relative to the domain directory
		if strings.Contains(relPath, "/domain/") {
			// Extract the path starting from "domain/"
			index := strings.Index(relPath, "/domain/")
			if index != -1 {
				relPath = relPath[index+1:] // +1 to remove the leading slash
			}
		}

		// Format the row
		sb.WriteString(fmt.Sprintf("| %s | [%s](%s) | %s |\n",
			err.Name,
			err.Domain,
			relPath,
			err.Documentation))
	}

	return sb.String()
}
