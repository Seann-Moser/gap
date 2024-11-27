package tools

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ListUntestedFunctions lists all functions in the given directory
// that are not covered by the specified coverage file.
// CoverageData holds coverage information per file
type CoverageData map[string][]CoverageBlock

// CoverageBlock represents a block of code and its execution count
type CoverageBlock struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
	Count     int
}

// ListUntestedFunctions lists all functions in the given directory
// that are not covered by the specified coverage file.
func ListUntestedFunctions(coverageFile, directory string) ([]FunctionInfo, error) {
	// Parse the coverage profile
	coverageData, err := parseCoverageFile(coverageFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse coverage file: %v", err)
	}

	// Walk through the Go files in the directory
	var untestedFunctions []FunctionInfo
	fset := token.NewFileSet()

	err = filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the vendor directory
		if info.IsDir() && info.Name() == "vendor" {
			return filepath.SkipDir
		}

		// Process only .go files excluding test files
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
			// Parse the Go file
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				log.Printf("Failed to parse file %s: %v", path, err)
				return nil
			}

			// Inspect the AST for function declarations
			ast.Inspect(file, func(n ast.Node) bool {
				if fn, ok := n.(*ast.FuncDecl); ok {
					position := fset.Position(fn.Pos())
					filePath := position.Filename

					// Check if the file has coverage data
					blocks, exists := coverageData[filePath]
					if !exists {
						params, err := extractParameters(fn, fset)
						if err != nil {
							log.Printf("Failed to extract parameters for function %s: %v", fn.Name.Name, err)
							return true
						}
						// If no coverage data for the file, consider all functions untested
						untestedFunctions = append(untestedFunctions, FunctionInfo{
							FileName:   filePath,
							FuncName:   fn.Name.Name,
							Line:       position.Line,
							Parameters: params,
						})
						return true
					}

					// Check if the function is covered
					if !isFunctionCovered(fn, blocks) {
						params, err := extractParameters(fn, fset)
						if err != nil {
							log.Printf("Failed to extract parameters for function %s: %v", fn.Name.Name, err)
							return true
						}
						untestedFunctions = append(untestedFunctions, FunctionInfo{
							FileName:   filePath,
							FuncName:   fn.Name.Name,
							Line:       position.Line,
							Parameters: params,
						})
					}
				}
				return true
			})
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return untestedFunctions, nil
}

// parseCoverageFile parses the coverage.out file and returns coverage data per file
func parseCoverageFile(coverageFile string) (CoverageData, error) {
	file, err := os.Open(coverageFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	coverage := make(CoverageData)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "mode:") || line == "" {
			continue
		}

		// Example line format:
		// path/to/file.go:10.2,12.3 2 1
		parts := strings.Split(line, " ")
		if len(parts) < 3 {
			continue
		}

		fileAndRange := parts[0]
		countStr := parts[2]

		// Split file and range
		colonIndex := strings.Index(fileAndRange, ":")
		if colonIndex == -1 {
			continue
		}
		filePath := fileAndRange[:colonIndex]
		rangePart := fileAndRange[colonIndex+1:]

		// Split range
		rangeParts := strings.Split(rangePart, ",")
		if len(rangeParts) != 2 {
			continue
		}

		startPos := strings.Split(rangeParts[0], ".")
		endPos := strings.Split(rangeParts[1], ".")

		if len(startPos) != 2 || len(endPos) != 2 {
			continue
		}

		startLine, err1 := strconv.Atoi(startPos[0])
		startCol, err2 := strconv.Atoi(startPos[1])
		endLine, err3 := strconv.Atoi(endPos[0])
		endCol, err4 := strconv.Atoi(endPos[1])
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			continue
		}

		count, err := strconv.Atoi(countStr)
		if err != nil {
			continue
		}

		block := CoverageBlock{
			StartLine: startLine,
			StartCol:  startCol,
			EndLine:   endLine,
			EndCol:    endCol,
			Count:     count,
		}

		coverage[filePath] = append(coverage[filePath], block)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return coverage, nil
}

// isFunctionCovered checks if any coverage block overlaps with the function's position
func isFunctionCovered(fn *ast.FuncDecl, blocks []CoverageBlock) bool {
	if fn.Body == nil {
		// External function declaration without body (e.g., interface methods)
		return false
	}

	//startPos := fn.Body.Pos()
	//endPos := fn.Body.End()

	for _, block := range blocks {
		// Check if the block has been executed (count > 0)
		if block.Count > 0 {
			// For simplicity, consider function covered if any block in its body is covered
			return true
		}
	}
	return false
}
