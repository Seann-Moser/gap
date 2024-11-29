package tools

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// Helper function to print function calls with indentation.
func PrintFunctionCalls(calls []FunctionCallInfo, indent int) {
	prefix := strings.Repeat("  ", indent)
	for _, call := range calls {
		fmt.Printf("%sFunction Call: %s %d %s\n", prefix, call.Function, call.Line, call.Package)
		if len(call.Arguments) > 0 {
			fmt.Printf("%sArguments: %v\n", prefix, call.Arguments)
		}
		if len(call.Calls) > 0 {
			fmt.Printf("%sNested Calls:\n", prefix)
			PrintFunctionCalls(call.Calls, indent+1)
		}
	}
}

// GetFunctionCalls retrieves all function calls within a given function source code.
func GetFunctionCalls(functionCode string, fi FunctionInfo, projectRoot string) ([]FunctionCallInfo, error) {
	// Create a full source code with a package declaration to make it parsable.
	src := fmt.Sprintf("package %s\n%s", fi.PkgName, functionCode)
	if fi.RelativeFilePath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		if projectRoot == "" {
			projectRoot = cwd
		}
		// Construct the full file path
		fullPath := filepath.Join(projectRoot, fi.RelativeFilePath)

		// Check if the file exists
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("file does not exist: %s", fullPath)
		}

		// Read the source file
		srcByte, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", fullPath, err)
		}
		src = string(srcByte)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.AllErrors)
	if err != nil {
		return nil, fmt.Errorf("failed to parse function code: %w", err)
	}

	// Collect imports
	imports := make(map[string]struct{}) // alias -> struct{}
	for _, imp := range file.Imports {
		var importName string
		if imp.Name != nil {
			importName = imp.Name.Name
		} else {
			// Default import name is the base of the import path
			importPath := strings.Trim(imp.Path.Value, `"`)
			importName = filepath.Base(importPath)
		}
		imports[importName] = struct{}{}
	}

	// Find the function declaration that matches the given FunctionInfo.
	var funcDecl *ast.FuncDecl
	for _, decl := range file.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok {
			if fd.Name.Name == fi.Name {
				// If it's a method, ensure the receiver matches.
				if fi.StructName != "" {
					if fd.Recv != nil && len(fd.Recv.List) > 0 {
						recvType := exprToString(fd.Recv.List[0].Type)
						// Normalize receiver type by removing pointers.
						recvType = strings.TrimPrefix(recvType, "*")
						structName := strings.TrimPrefix(fi.StructName, "*")
						if recvType == structName {
							funcDecl = fd
							break
						}
					}
				} else {
					// It's a function, not a method.
					if fd.Recv == nil {
						funcDecl = fd
						break
					}
				}
			}
		}
	}
	if funcDecl == nil {
		return nil, fmt.Errorf("function %s not found", fi.Name)
	}

	// Now walk the function body to collect function calls.
	var calls []FunctionCallInfo
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if callExpr, ok := n.(*ast.CallExpr); ok {
			callInfo := extractCallInfo(callExpr, fset, projectRoot, fi.RelativeFilePath, imports)
			calls = append(calls, callInfo)
			// Do not traverse into this callExpr's arguments, as extractCallInfo already handles that.
			return false
		}
		return true
	})

	return calls, nil
}

// extractCallInfo extracts information from a CallExpr and returns a FunctionCallInfo.
func extractCallInfo(callExpr *ast.CallExpr, fset *token.FileSet, projectRoot, filePath string, imports map[string]struct{}) FunctionCallInfo {
	// Extract the function being called.
	functionName, packageName, receiverName := getFunctionName(callExpr.Fun, imports)

	// Get line number.
	pos := fset.Position(callExpr.Pos())
	line := pos.Line

	// Extract arguments.
	var argExprs []string
	for _, arg := range callExpr.Args {
		argExprs = append(argExprs, exprToString(arg))
	}

	// Recursively get function calls within arguments.
	var nestedCalls []FunctionCallInfo
	for _, arg := range callExpr.Args {
		nestedCalls = append(nestedCalls, extractNestedCalls(arg, fset, projectRoot, filePath, imports)...)
	}

	// Get full expression.
	fullExpr := exprToString(callExpr)

	return FunctionCallInfo{
		Function:  functionName,
		Package:   packageName,
		Receiver:  receiverName,
		Line:      line,
		Calls:     nestedCalls,
		FullExpr:  fullExpr,
		Arguments: argExprs,
		FilePath:  filepath.Join(projectRoot, filePath),
	}
}

// getFunctionName extracts the function name, package name, and receiver from a function expression.
func getFunctionName(fun ast.Expr, imports map[string]struct{}) (functionName, packageName, receiverName string) {
	switch expr := fun.(type) {
	case *ast.Ident:
		// Simple function call, e.g., FooBar()
		functionName = expr.Name
	case *ast.SelectorExpr:
		// Selector expression, e.g., pkg.Func() or obj.Method()
		selector := expr.Sel.Name
		switch x := expr.X.(type) {
		case *ast.Ident:
			// Could be a package name or a variable name
			if _, ok := imports[x.Name]; ok {
				// It's a package name
				packageName = x.Name
			} else {
				// It's a variable (receiver)
				receiverName = x.Name
			}
		default:
			// Could be a complex receiver expression
			receiverName = exprToString(expr.X)
		}
		functionName = selector
	case *ast.FuncLit:
		// Function literal (anonymous function)
		functionName = "func"
	default:
		// Other cases
		functionName = exprToString(fun)
	}
	return
}

// extractNestedCalls recursively extracts function calls within an expression.
func extractNestedCalls(expr ast.Expr, fset *token.FileSet, projectRoot, filePath string, imports map[string]struct{}) []FunctionCallInfo {
	var calls []FunctionCallInfo
	ast.Inspect(expr, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if callExpr, ok := n.(*ast.CallExpr); ok {
			callInfo := extractCallInfo(callExpr, fset, projectRoot, filePath, imports)
			calls = append(calls, callInfo)
			// Do not traverse into this callExpr's arguments, as extractCallInfo already handles that.
			return false
		}
		return true
	})
	return calls
}
