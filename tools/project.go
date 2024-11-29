package tools

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type FunctionInfo struct {
	RelativeFilePath string
	PkgName          string
	Name             string
	StructName       string
	Parameters       []ParameterInfo
	Returns          []ReturnInfo
	LineNumberStart  int
	LineNumberEnd    int
}

type ParameterInfo struct {
	Name       string
	Type       string
	ImportPath string
	ImportName string
}

type ReturnInfo struct {
	Type       string
	ImportPath string
	ImportName string
}

func GetFunctionWithComments(fi FunctionInfo, projectRoot string) (string, error) {
	// Construct the full file path
	fullPath := filepath.Join(projectRoot, fi.RelativeFilePath)

	// Check if the file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return "", fmt.Errorf("file does not exist: %s", fullPath)
	}

	// Read the source file
	src, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", fullPath, err)
	}

	// Initialize a new FileSet
	fset := token.NewFileSet()

	// Parse the file, including comments
	file, err := parser.ParseFile(fset, fullPath, src, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("failed to parse file %s: %w", fullPath, err)
	}

	// Iterate through declarations to find the target function
	for _, decl := range file.Decls {
		// We're interested in function declarations
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		// Check if the function name matches
		if funcDecl.Name.Name != fi.Name {
			continue
		}

		// Check if the receiver (struct) matches for methods
		if fi.StructName != "" {
			if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
				continue
			}
			recvType := exprToString(funcDecl.Recv.List[0].Type)

			// Remove pointer indicator (*) if present
			recvType = strings.TrimPrefix(recvType, "*")
			structName := strings.TrimPrefix(fi.StructName, "*")

			if recvType != structName {
				continue
			}
		} else {
			// If fi.StructName is empty, ensure it's not a method
			if funcDecl.Recv != nil {
				continue
			}
		}

		// Extract the function's source code (which includes comments)
		var buf bytes.Buffer
		err = format.Node(&buf, fset, funcDecl)
		if err != nil {
			return "", fmt.Errorf("failed to format function %s: %w", fi.Name, err)
		}
		funcCode := buf.String()

		return funcCode, nil
	}

	return "", fmt.Errorf("function %s not found in file %s", fi.Name, fullPath)
}
func GetFunctions(projectRoot string) ([]FunctionInfo, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if projectRoot == "" {
		projectRoot = cwd
	}
	fmt.Printf("Parsing project root %s\n", projectRoot)
	var functions []FunctionInfo

	err = filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() == "vendor" {
			return filepath.SkipDir
		}
		// Skip directories and non-Go files
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		relPath, err := filepath.Rel(projectRoot, path)
		if err != nil {
			return err
		}
		funcs, err := parseFile(relPath, path)
		if err != nil {
			fmt.Printf("Error parsing file %s: %v\n", path, err)
		} else {
			functions = append(functions, funcs...)
		}
		return nil
	})

	if err != nil {
		fmt.Printf("Error walking the path %s: %v\n", projectRoot, err)
		return nil, err
	}
	return functions, nil
}

func PrintFunctions(functions ...FunctionInfo) {
	for _, fi := range functions {
		fmt.Printf("File: %s\n", fi.RelativeFilePath)
		fmt.Printf("Package: %s\n", fi.PkgName)
		if fi.StructName != "" {
			fmt.Printf("Method: %s.%s\n", fi.StructName, fi.Name)
		} else {
			fmt.Printf("Function: %s\n", fi.Name)
		}
		fmt.Printf("Function Lines: %d-%d\n", fi.LineNumberStart, fi.LineNumberEnd)
		fmt.Println("Parameters:")
		for _, param := range fi.Parameters {
			fmt.Printf("  Name: %s, Type: %s, ImportName: %s, ImportPath: %s\n",
				param.Name, param.Type, param.ImportName, param.ImportPath)
		}
		fmt.Println("Returns:")
		for _, ret := range fi.Returns {
			fmt.Printf("  Type: %s, ImportName: %s, ImportPath: %s\n",
				ret.Type, ret.ImportName, ret.ImportPath)
		}
		fmt.Println("-----------")
	}
}

func parseFile(relPath, fullPath string) ([]FunctionInfo, error) {
	fset := token.NewFileSet()
	fileAst, err := parser.ParseFile(fset, fullPath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	// Collect imports
	imports := make(map[string]string) // import name -> import path
	for _, imp := range fileAst.Imports {
		var importName string
		if imp.Name != nil {
			importName = imp.Name.Name
		} else {
			// Default import name is the base of the import path
			importPath := strings.Trim(imp.Path.Value, `"`)
			importName = filepath.Base(importPath)
		}
		importPath := strings.Trim(imp.Path.Value, `"`)
		imports[importName] = importPath
	}

	var functions []FunctionInfo

	for _, decl := range fileAst.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		fi := FunctionInfo{
			RelativeFilePath: relPath,
			PkgName:          fileAst.Name.Name,
			Name:             funcDecl.Name.Name, // Set the function name here
		}

		// Get struct name if method
		if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
			recvType := funcDecl.Recv.List[0].Type
			fi.StructName = exprToString(recvType)
		}

		// Parameters
		for _, field := range funcDecl.Type.Params.List {
			typeStr := exprToString(field.Type)
			importName, importPath := getImportInfo(field.Type, imports)

			for _, name := range field.Names {
				pi := ParameterInfo{
					Name:       name.Name,
					Type:       typeStr,
					ImportName: importName,
					ImportPath: importPath,
				}
				fi.Parameters = append(fi.Parameters, pi)
			}

			// If parameter has no name (e.g., anonymous parameter)
			if len(field.Names) == 0 {
				pi := ParameterInfo{
					Name:       "",
					Type:       typeStr,
					ImportName: importName,
					ImportPath: importPath,
				}
				fi.Parameters = append(fi.Parameters, pi)
			}
		}

		// Returns
		if funcDecl.Type.Results != nil {
			for _, field := range funcDecl.Type.Results.List {
				typeStr := exprToString(field.Type)
				importName, importPath := getImportInfo(field.Type, imports)

				ri := ReturnInfo{
					Type:       typeStr,
					ImportName: importName,
					ImportPath: importPath,
				}
				fi.Returns = append(fi.Returns, ri)
			}
		}

		// Line numbers
		start := fset.Position(funcDecl.Pos())
		end := fset.Position(funcDecl.End())
		fi.LineNumberStart = start.Line
		fi.LineNumberEnd = end.Line

		functions = append(functions, fi)
	}

	return functions, nil
}

func exprToString(expr ast.Expr) string {
	var buf bytes.Buffer
	err := format.Node(&buf, token.NewFileSet(), expr)
	if err != nil {
		return ""
	}
	return buf.String()
}

func getImportInfo(expr ast.Expr, imports map[string]string) (importName, importPath string) {
	var identList []string

	ast.Inspect(expr, func(n ast.Node) bool {
		if selExpr, ok := n.(*ast.SelectorExpr); ok {
			if ident, ok := selExpr.X.(*ast.Ident); ok {
				identList = append(identList, ident.Name)
			}
		}
		return true
	})

	for _, ident := range identList {
		if importPath, ok := imports[ident]; ok {
			return ident, importPath
		}
	}

	return "", ""
}
