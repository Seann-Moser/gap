package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GenerateGraphviz generates a DOT file representing the function call graph.
// outputPath is the path where the DOT file will be saved.
func GenerateGraphviz(functions []*FunctionInfo, outputPath string) error {
	// Create or truncate the output file
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create DOT file: %w", err)
	}
	defer file.Close()

	// Write the header
	fmt.Fprintln(file, "digraph FunctionCallGraph {")
	fmt.Fprintln(file, "\trankdir=LR;")                                      // Left to right layout
	fmt.Fprintln(file, "\tnode [shape=box, style=filled, color=lightblue];") // Default node style

	// Keep track of already defined nodes to avoid duplicates
	definedNodes := make(map[string]bool)

	// Function to sanitize node names (remove characters that are invalid in DOT)
	sanitize := func(name string) string {
		// Replace non-alphanumeric characters with underscores
		return strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') ||
				(r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') ||
				r == '_' {
				return r
			}
			return '_'
		}, name)
	}

	// Iterate over all functions to define nodes and edges
	for _, fn := range functions {
		// Define the node for the function if not already defined
		funcID := sanitize(fn.ImportPathIdentifier())
		if !definedNodes[funcID] {
			fmt.Fprintf(file, "\t%s [label=\"%s\\n%s:%d\"];\n", funcID, fn.FullName(), filepath.Base(fn.FileName), fn.Line)
			definedNodes[funcID] = true
		}

		// Add edges for internal function calls
		for _, calledFunc := range fn.FunctionCalls {
			calledFuncID := sanitize(calledFunc.ImportPathIdentifier())
			if !definedNodes[calledFuncID] {
				fmt.Fprintf(file, "\t%s [label=\"%s\\n%s:%d\"];\n", calledFuncID, calledFunc.FullName(), filepath.Base(calledFunc.FileName), calledFunc.Line)
				definedNodes[calledFuncID] = true
			}
			// Draw edge from fn to calledFunc
			fmt.Fprintf(file, "\t%s -> %s;\n", funcID, calledFuncID)
		}

		// Add edges for external function calls
		for _, ext := range fn.Externals {
			// Define a unique node for the external function based on import path and function name
			externalNodeName := sanitize(ext.ImportPath + "." + ext.Name)
			if !definedNodes[externalNodeName] {
				fmt.Fprintf(file, "\t%s [label=\"%s\\n%s\", shape=ellipse, style=filled, color=lightgray];\n", externalNodeName, ext.Name, ext.ImportPath)
				definedNodes[externalNodeName] = true
			}
			// Draw edge from fn to external function
			fmt.Fprintf(file, "\t%s -> %s [style=dashed];\n", funcID, externalNodeName)
		}
	}

	// Write the footer
	fmt.Fprintln(file, "}")

	return nil
}

// FullName returns the full name of the function including package and function name.
// Modify this method based on how you want to represent the function's identity.
func (fn *FunctionInfo) FullName() string {
	// Assuming ImportPathIdentifier uniquely identifies the function's package.
	return fmt.Sprintf("%s.%s", fn.ImportPathIdentifier(), fn.FuncName)
}

// ImportPathIdentifier returns a unique identifier based on the import path and file.
func (fn *FunctionInfo) ImportPathIdentifier() string {
	// You can modify this to include package name if available.
	// For simplicity, we'll use the relative file path without extension.
	cwd, _ := os.Getwd()
	if strings.Contains(cwd, "/") {
		cwd = strings.Join(strings.Split(cwd, "/")[:len(strings.Split(cwd, "/"))-1], "/")
	}
	fn.FileName = strings.ReplaceAll(fn.FileName, cwd, "")
	relPath, err := filepath.Rel(".", fn.FileName)
	if err != nil {
		// Fallback to absolute path if relative path fails
		relPath = fn.FileName
	}
	withoutExt := strings.TrimSuffix(relPath, filepath.Ext(relPath))
	return sanitizePath(withoutExt)
}

// sanitizePath replaces path separators with underscores to create valid DOT node names.
func sanitizePath(path string) string {
	return strings.ReplaceAll(path, string(os.PathSeparator), "_")
}

// sanitize replaces invalid characters with underscores.
// This is to ensure the node names are valid in DOT.
func sanitize(name string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '_' {
			return r
		}
		return '_'
	}, name)
}
