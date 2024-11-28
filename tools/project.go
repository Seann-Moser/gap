package tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// builtInFuncs contains all Go built-in function names.
// This map is used to filter out built-in functions from being classified as external.
var builtInFuncs = map[string]bool{
	"len":     true,
	"cap":     true,
	"append":  true,
	"copy":    true,
	"close":   true,
	"delete":  true,
	"make":    true,
	"new":     true,
	"panic":   true,
	"recover": true,
	"string":  true,
	"println": true,
}

// FunctionInfo holds information about a function
type FunctionInfo struct {
	FileName      string
	FuncName      string
	Line          int
	PkgName       string
	Parameters    []string
	Returns       []string
	Externals     []*External
	FunctionCalls []*FunctionInfo
}

// External holds information about external function calls
type External struct {
	Name       string // Function name
	Type       string // Package alias used in the code
	ImportPath string // Full import path (e.g., "github.com/user/package")
}

// ListFunctions lists all functions in a Go project directory,
// along with their line numbers, parameters, return types,
// external calls (excluding standard packages), and internal function calls.
// It excludes the vendor/ directory.
func ListFunctions(projectDir string) ([]*FunctionInfo, error) {
	var functions []*FunctionInfo

	// If projectDir is empty, use the current working directory
	if projectDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("error getting current working directory: %w", err)
		}
		projectDir = cwd
	}

	// Initialize a new FileSet
	fset := token.NewFileSet()

	// Retrieve all standard library packages
	stdPkgs, err := getStandardPackages()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve standard packages: %w", err)
	}

	// Retrieve the module path from go.mod
	modulePath, err := getModulePath(projectDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get module path: %w", err)
	}

	// Map to store internal functions for quick lookup
	internalFuncs := make(map[string]*FunctionInfo)

	// First pass: Collect all internal functions
	err = filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// If there's an error accessing the path, skip it
			log.Printf("Error accessing path %s: %v", path, err)
			return nil
		}

		// Skip the vendor directory
		if info.IsDir() && info.Name() == "vendor" {
			return filepath.SkipDir
		}

		// Process only .go files excluding test files
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
			// Parse the Go file with comments
			node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				log.Printf("Failed to parse file %s: %v", path, err)
				return nil // Continue with other files
			}

			// Extract the package name from the AST
			pkgName := node.Name.Name

			// Inspect the AST for function declarations
			ast.Inspect(node, func(n ast.Node) bool {
				if fn, ok := n.(*ast.FuncDecl); ok {
					// Get function parameters
					params, err := extractParameters(fn, fset)
					if err != nil {
						log.Printf("Error extracting parameters for function %s in %s: %v", fn.Name.Name, path, err)
						return true // Continue inspecting
					}

					// Get function return types
					returns, err := extractReturns(fn, fset)
					if err != nil {
						log.Printf("Error extracting return types for function %s in %s: %v", fn.Name.Name, path, err)
						return true // Continue inspecting
					}

					// Get the line number
					position := fset.Position(fn.Pos())

					// Create FunctionInfo
					funcInfo := &FunctionInfo{
						FileName:      position.Filename,
						FuncName:      fn.Name.Name,
						Line:          position.Line,
						PkgName:       pkgName, // Assign the package name
						Parameters:    params,
						Returns:       returns,
						Externals:     []*External{},
						FunctionCalls: []*FunctionInfo{},
					}

					// Add to functions slice
					functions = append(functions, funcInfo)

					// Create a unique key for internal functions
					// Key format: "pkgName.funcName"
					key := fmt.Sprintf("%s.%s", pkgName, fn.Name.Name)
					internalFuncs[key] = funcInfo
				}
				return true
			})
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Second pass: Analyze function calls within each function
	for _, fn := range functions {
		err := analyzeFunctionCalls(fn, internalFuncs, fset, stdPkgs, modulePath)
		if err != nil {
			log.Printf("Error analyzing function calls for %s: %v", fn.FuncName, err)
		}
	}

	return functions, nil
}

// getStandardPackages retrieves all standard library package paths using 'go list std'
func getStandardPackages() (map[string]bool, error) {
	cmd := exec.Command("go", "list", "std")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute 'go list std': %w", err)
	}

	pkgs := strings.Split(strings.TrimSpace(string(output)), "\n")
	stdPkgs := make(map[string]bool, len(pkgs))
	for _, pkg := range pkgs {
		stdPkgs[pkg] = true
	}
	return stdPkgs, nil
}

// getModulePath retrieves the module path from go.mod in the specified directory or its parents
func getModulePath(dir string) (string, error) {
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			// Read go.mod
			data, err := os.ReadFile(goModPath)
			if err != nil {
				return "", fmt.Errorf("failed to read go.mod at %s: %w", goModPath, err)
			}
			// Find the module declaration
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "module ") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						return parts[1], nil
					}
				}
			}
			return "", fmt.Errorf("module path not found in go.mod at %s", goModPath)
		}
		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("go.mod not found in any parent directories")
}

// buildImportMap builds a map from package alias to import path for a given file
func buildImportMap(node *ast.File) map[string]string {
	importMap := make(map[string]string)
	for _, imp := range node.Imports {
		// Remove quotes from import path
		importPath := strings.Trim(imp.Path.Value, "\"")
		var alias string
		if imp.Name != nil {
			alias = imp.Name.Name
		} else {
			// Default alias is the base of the import path
			alias = filepath.Base(importPath)
		}
		importMap[alias] = importPath
	}
	return importMap
}

// extractParameters extracts function parameters as a slice of strings using go/printer for accurate type representation
func extractParameters(fn *ast.FuncDecl, fset *token.FileSet) ([]string, error) {
	var params []string
	for _, param := range fn.Type.Params.List {
		typeStr, err := exprToString(param.Type, fset)
		if err != nil {
			return nil, err
		}

		if len(param.Names) > 0 {
			for _, name := range param.Names {
				params = append(params, fmt.Sprintf("%s %s", name.Name, typeStr))
			}
		} else {
			// Handle unnamed parameters
			params = append(params, typeStr)
		}
	}
	return params, nil
}

// extractReturns extracts function return types as a slice of strings using go/printer for accurate type representation
func extractReturns(fn *ast.FuncDecl, fset *token.FileSet) ([]string, error) {
	var returns []string

	// If there are no return types, return an empty slice
	if fn.Type.Results == nil {
		return returns, nil
	}

	for _, result := range fn.Type.Results.List {
		typeStr, err := exprToString(result.Type, fset)
		if err != nil {
			return nil, err
		}

		if len(result.Names) > 0 {
			for _, name := range result.Names {
				returns = append(returns, fmt.Sprintf("%s %s", name.Name, typeStr))
			}
		} else {
			// Handle unnamed return types
			returns = append(returns, typeStr)
		}
	}
	return returns, nil
}

// exprToString converts an ast.Expr to its string representation using go/printer
func exprToString(expr ast.Expr, fset *token.FileSet) (string, error) {

	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name, nil
	case *ast.SelectorExpr:
		es, err := exprToString(e.X, fset)
		if err != nil {
			return "", err
		}
		return es + "." + e.Sel.Name, nil
	case *ast.StarExpr:
		es, err := exprToString(e.X, fset)
		if err != nil {
			return "", err
		}
		return "*" + es, nil
	case *ast.ArrayType:
		es, err := exprToString(e.Elt, fset)
		if err != nil {
			return "", err
		}
		return "[]" + es, nil
	case *ast.MapType:
		es, err := exprToString(e.Key, fset)
		if err != nil {
			return "", err
		}
		es2, err := exprToString(e.Value, fset)
		if err != nil {
			return "", err
		}
		c := "map[" + es + "]" + es2
		return c, nil
	case *ast.FuncType:
		return "func", nil // Simplified
	default:
		var buf bytes.Buffer
		err := printer.Fprint(&buf, fset, expr)
		if err != nil {
			return "", err
		}
		return buf.String(), nil
	}
}

// analyzeFunctionCalls analyzes and populates FunctionCalls and Externals for a given function
func analyzeFunctionCalls(fn *FunctionInfo, internalFuncs map[string]*FunctionInfo, fset *token.FileSet, stdPkgs map[string]bool, modulePath string) error {
	// Read the source file
	src, err := os.ReadFile(fn.FileName)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", fn.FileName, err)
	}

	// Parse the file
	node, err := parser.ParseFile(fset, fn.FileName, src, 0)
	if err != nil {
		return fmt.Errorf("failed to parse file %s: %w", fn.FileName, err)
	}

	// Build a map of import aliases to import paths
	importMap := buildImportMap(node)

	// Find the target function in the AST
	var targetFunc *ast.FuncDecl
	for _, decl := range node.Decls {
		if f, ok := decl.(*ast.FuncDecl); ok {
			pos := fset.Position(f.Pos())
			if f.Name.Name == fn.FuncName && pos.Line == fn.Line {
				targetFunc = f
				break
			}
		}
	}

	if targetFunc == nil {
		return fmt.Errorf("function %s not found in file %s at line %d", fn.FuncName, fn.FileName, fn.Line)
	}

	// Traverse the function body to find all function calls
	ast.Inspect(targetFunc.Body, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true // Continue traversing
		}

		// Determine the function being called
		switch fun := callExpr.Fun.(type) {
		case *ast.Ident:
			// Local function call (same package) or built-in function
			funcName := fun.Name
			if builtInFuncs[funcName] {
				// It's a built-in function; ignore it
				return true
			}

			// Create a unique key for internal functions
			key := fmt.Sprintf("%s.%s", fn.PkgName, funcName)
			if calledFunc, exists := internalFuncs[key]; exists {
				println("FOUND FUNCTION CALL: ", key)
				fn.FunctionCalls = append(fn.FunctionCalls, calledFunc)
			} else {
				// Function not found in internalFuncs, treat as external with unknown import path
				println("MISSGING FUNCTION CALL: ", key)
				external := &External{
					Name:       funcName,
					Type:       "", // Unknown package
					ImportPath: "", // Unknown import path
				}
				fn.Externals = append(fn.Externals, external)
			}
		case *ast.SelectorExpr:
			// Function from a package or struct method
			pkgIdent, ok := fun.X.(*ast.Ident)
			if !ok {
				// Could be a struct method, ignore or handle as needed
				return true
			}
			pkgAlias := pkgIdent.Name
			funcName := fun.Sel.Name

			// Map package alias to import path
			importPath, exists := importMap[pkgAlias]
			if !exists {
				// Package alias not found in imports, possibly a built-in or undefined
				return true
			}

			// Check if the package is standard
			if stdPkgs[importPath] {
				// It's a standard package, ignore as per requirements
				return true
			}

			// Check if the package is internal (part of the module)
			isInternal := strings.HasPrefix(importPath, modulePath)
			if isInternal {
				// Internal package, find the corresponding FunctionInfo
				// Construct key as "base.ImportPath.FuncName"
				baseImportPath := filepath.Base(importPath)
				key := fmt.Sprintf("%s.%s", baseImportPath, funcName)
				if calledFunc, exists := internalFuncs[key]; exists {
					println("FOUND FUNCTION CALL: ", key)
					fn.FunctionCalls = append(fn.FunctionCalls, calledFunc)
				} else {
					println("MISSGING FUNCTION CALL: ", key)

					// Function not found in internalFuncs, treat as external
					external := &External{
						Name:       funcName,
						Type:       pkgAlias,
						ImportPath: importPath,
					}
					fn.Externals = append(fn.Externals, external)
				}
			} else {
				// External package, add to Externals with import path
				external := &External{
					Name:       funcName,
					Type:       pkgAlias,
					ImportPath: importPath,
				}
				fn.Externals = append(fn.Externals, external)
			}
		default:
			// Other types of function calls (e.g., function literals), ignored for this analysis
		}

		return true
	})

	return nil
}

// PrintFunctionInfos prints a slice of FunctionInfo in a user-friendly format
func PrintFunctionInfos(functions []*FunctionInfo) {
	if len(functions) == 0 {
		fmt.Println("No functions found.")
		return
	}

	// Determine the maximum lengths for formatting
	maxFileNameLength := len("File")
	maxFuncNameLength := len("Function")
	maxLineLength := len("Line")
	maxParamsLength := len("Parameters")
	maxReturnsLength := len("Returns")
	maxExternalsLength := len("Externals")
	maxFunctionCallsLength := len("FunctionCalls")

	for _, fn := range functions {
		if len(fn.FileName) > maxFileNameLength {
			maxFileNameLength = len(fn.FileName)
		}
		if len(fn.FuncName) > maxFuncNameLength {
			maxFuncNameLength = len(fn.FuncName)
		}
		lineLen := len(fmt.Sprintf("%d", fn.Line))
		if lineLen > maxLineLength {
			maxLineLength = lineLen
		}
		paramsLen := len(strings.Join(fn.Parameters, ", "))
		if paramsLen > maxParamsLength {
			maxParamsLength = paramsLen
		}
		returnsLen := len(strings.Join(fn.Returns, ", "))
		if returnsLen > maxReturnsLength {
			maxReturnsLength = returnsLen
		}
		externalsLen := 0
		for _, ext := range fn.Externals {
			// Format: ImportPath.FuncName
			if ext.ImportPath != "" {
				externalsLen += len(ext.ImportPath) + 1 + len(ext.Name) + 2 // for dot and comma
			} else {
				externalsLen += len(ext.Name) + 2 // for comma
			}
		}
		if externalsLen > maxExternalsLength {
			maxExternalsLength = externalsLen
		}
		funcCallsLen := 0
		for _, call := range fn.FunctionCalls {
			funcCallsLen += len(call.FuncName) + 2 // for comma and space
		}
		if funcCallsLen > maxFunctionCallsLength {
			maxFunctionCallsLength = funcCallsLen
		}
	}

	// Define a format string for consistent alignment
	format := fmt.Sprintf("%%-%ds | %%-%ds | %%-%ds | %%-%ds | %%-%ds | %%-%ds | %%-%ds\n",
		maxFileNameLength, maxFuncNameLength, maxLineLength, maxParamsLength, maxReturnsLength, maxExternalsLength, maxFunctionCallsLength)

	// Print header
	fmt.Printf(format, "File", "Function", "Line", "Parameters", "Returns", "Externals", "FunctionCalls")
	fmt.Println(strings.Repeat("-", maxFileNameLength+maxFuncNameLength+maxLineLength+maxParamsLength+maxReturnsLength+maxExternalsLength+maxFunctionCallsLength+21))

	// Print each FunctionInfo
	for _, fn := range functions {
		params := "None"
		if len(fn.Parameters) > 0 {
			params = strings.Join(fn.Parameters, ", ")
		}

		returns := "None"
		if len(fn.Returns) > 0 {
			returns = strings.Join(fn.Returns, ", ")
		}

		externals := "None"
		if len(fn.Externals) > 0 {
			var extList []string
			for _, ext := range fn.Externals {
				if ext.ImportPath != "" {
					extList = append(extList, fmt.Sprintf("%s.%s", ext.ImportPath, ext.Name))
				} else {
					extList = append(extList, ext.Name)
				}
			}
			externals = strings.Join(extList, ", ")
		}

		functionCalls := "None"
		if len(fn.FunctionCalls) > 0 {
			var callList []string
			for _, call := range fn.FunctionCalls {
				callList = append(callList, call.FuncName)
			}
			functionCalls = strings.Join(callList, ", ")
		}

		fmt.Printf(format, fn.FileName, fn.FuncName, fmt.Sprintf("%d", fn.Line), params, returns, externals, functionCalls)
	}
}

// isExternalPackage determines if a package is external based on its import path.
// For simplicity, assume that packages outside the project's module are external.
// This function can be enhanced to accurately determine package origins.
func isExternalPackage(pkgName, currentFile string) bool {
	// Example heuristic: packages starting with "github.com/" or standard library packages are external
	// Adjust this logic based on your project's structure and module setup

	// Get the module path from go.mod
	modulePath, err := getModulePath(filepath.Dir(currentFile))
	if err != nil {
		log.Printf("Warning: %v. Assuming package %s is external.", err, pkgName)
		return true
	}

	// If the package name matches the module path, it's internal
	if strings.HasPrefix(pkgName, modulePath) {
		return false
	}

	// Otherwise, consider it external
	return true
}

// PrintFunctionInfos prints a slice of FunctionInfo in a user-friendly format

// ExportToJSON exports the slice of FunctionInfo to a JSON file
func ExportToJSON(functions []*FunctionInfo, filename string) error {
	data, err := json.MarshalIndent(functions, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

// ExportToCSV exports the slice of FunctionInfo to a CSV file
func ExportToCSV(functions []*FunctionInfo, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write header
	header := "File,Function,Line,Parameters,Returns,Externals,FunctionCalls\n"
	if _, err := file.WriteString(header); err != nil {
		return err
	}

	// Write function data
	for _, fn := range functions {
		params := "None"
		if len(fn.Parameters) > 0 {
			params = strings.Join(fn.Parameters, "; ")
		}

		returns := "None"
		if len(fn.Returns) > 0 {
			returns = strings.Join(fn.Returns, "; ")
		}

		externals := "None"
		if len(fn.Externals) > 0 {
			var extList []string
			for _, ext := range fn.Externals {
				extList = append(extList, fmt.Sprintf("%s.%s", ext.Type, ext.Name))
			}
			externals = strings.Join(extList, "; ")
		}

		//functionCalls := "None"
		//if len(fn.FunctionCalls) > 0 {
		//	var callList []string
		//	for _, call := range fn.FunctionCalls {
		//		callList = append(callList, call.FuncName)
		//	}
		//	functionCalls = strings.Join(callList, "; ")
		//}

		record := fmt.Sprintf("\"%s\",\"%s\",\"%d\",\"%s\",\"%s\",\"%s\"\n",
			fn.FileName, fn.FuncName, fn.Line, params, returns, externals)

		if _, err := file.WriteString(record); err != nil {
			return err
		}
	}

	return nil
}

// ExportToJSON exports the slice of FunctionInfo to a JSON file
// (Duplicate function name corrected)
func ExportToJSONUpdated(functions []*FunctionInfo, filename string) error {
	data, err := json.MarshalIndent(functions, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

// GetFunctionWithComments retrieves the full function definition along with its comments
func GetFunctionWithComments(fi FunctionInfo) (string, error) {
	// Check if the file exists
	if _, err := os.Stat(fi.FileName); os.IsNotExist(err) {
		return "", fmt.Errorf("file does not exist: %s", fi.FileName)
	}

	// Read the source file
	src, err := ioutil.ReadFile(fi.FileName)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", fi.FileName, err)
	}

	// Initialize a new FileSet
	fset := token.NewFileSet()

	// Parse the file, including comments
	file, err := parser.ParseFile(fset, fi.FileName, src, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("failed to parse file %s: %w", fi.FileName, err)
	}

	// Iterate through declarations to find the target function
	for _, decl := range file.Decls {
		// We're interested in function declarations
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		// Check if the function name matches
		if funcDecl.Name.Name != fi.FuncName {
			continue
		}

		// Get the position of the function
		pos := fset.Position(funcDecl.Pos())

		// Check if the starting line matches
		if pos.Line != fi.Line {
			continue
		}

		// Extract comments associated with the function
		var comments string
		if funcDecl.Doc != nil {
			comments = strings.TrimSpace(funcDecl.Doc.Text())
		}

		// Extract the function's source code
		var buf bytes.Buffer
		err = printer.Fprint(&buf, fset, funcDecl)
		if err != nil {
			return "", fmt.Errorf("failed to print function %s: %w", fi.FuncName, err)
		}
		funcCode := buf.String()

		// Combine comments and function code
		fullFunc := ""
		if comments != "" {
			// Add comment block before the function
			fullFunc = fmt.Sprintf("%s\n%s", comments, funcCode)
		} else {
			fullFunc = funcCode
		}

		return fullFunc, nil
	}

	return "", errors.New("function not found")
}
