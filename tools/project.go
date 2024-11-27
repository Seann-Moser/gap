package tools

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// FunctionInfo holds information about a function
type FunctionInfo struct {
	FileName      string
	FuncName      string
	Line          int
	Parameters    []string
	Returns       []string
	Externals     *External
	FunctionCalls []*FunctionInfo
}

type External struct {
	Name string
	Type string
}

// ListFunctions lists all functions in a Go project directory,
// along with their line numbers and parameters, excluding the vendor/ directory.
// ListFunctions lists all functions in a Go project directory,
// along with their line numbers, parameters, and return types, excluding the vendor/ directory.
func ListFunctions(projectDir string) ([]FunctionInfo, error) {
	var functions []FunctionInfo

	// If projectDir is empty, use the current working directory
	if projectDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("error getting current working directory: %w", err)
		}
		projectDir = cwd
	}

	fset := token.NewFileSet()

	// Walk through the project directory
	err := filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
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
			// Parse the Go file
			node, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				log.Printf("Failed to parse file %s: %v", path, err)
				return nil // Continue with other files
			}

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

					// Add function info to the list
					functions = append(functions, FunctionInfo{
						FileName:   position.Filename,
						FuncName:   fn.Name.Name,
						Line:       position.Line,
						Parameters: params,
						Returns:    returns, // Populate return types
					})
				}
				return true
			})
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return functions, nil
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

// PrintFunctionInfos prints a slice of FunctionInfo in a user-friendly format
func PrintFunctionInfos(functions []FunctionInfo) {
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
	}

	// Define a format string for consistent alignment
	format := fmt.Sprintf("%%-%ds | %%-%ds | %%-%ds | %%-%ds | %%-%ds\n",
		maxFileNameLength, maxFuncNameLength, maxLineLength, maxParamsLength, maxReturnsLength)

	// Print header
	fmt.Printf(format, "File", "Function", "Line", "Parameters", "Returns")
	fmt.Println(strings.Repeat("-", maxFileNameLength+maxFuncNameLength+maxLineLength+maxParamsLength+maxReturnsLength+13))

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

		fmt.Printf(format, fn.FileName, fn.FuncName, fmt.Sprintf("%d", fn.Line), params, returns)
	}
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
