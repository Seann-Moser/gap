package tools

import (
	"fmt"
	"strings"
)

// FunctionCallInfo represents a function call within a function.
type FunctionCallInfo struct {
	Function   string             // The function being called, including receiver if any.
	Line       int                // Line number where the call occurs.
	Calls      []FunctionCallInfo // Nested function calls within arguments.
	FullExpr   string             // The full expression of the function call.
	Package    string             // Package name if available.
	Receiver   string             // Receiver type or variable name if it's a method call.
	Arguments  []string           // Argument expressions as strings.
	FilePath   string             // The file where the function call is located.
	StructName string             // Struct name if it's a method within a struct.
}

func (fci *FunctionCallInfo) String(indent int) string {
	prefix := strings.Repeat("  ", indent)
	output := fmt.Sprintf("%sFunction Call: %s %d %s\n", prefix, fci.Function, fci.Line, fci.Package)
	if len(fci.Arguments) > 0 {
		output += fmt.Sprintf("%sArguments: %v\n", prefix, fci.Arguments)
	}
	if len(fci.Calls) > 0 {
		output += fmt.Sprintf("%sNested Calls:\n", prefix)
		for _, call := range fci.Calls {
			output += fmt.Sprintf("%s%s\n", prefix, call.String(indent+1))
		}
	}
	return output

}

type CallGraph struct {
	Nodes map[string]*FunctionNode
}

type FunctionNode struct {
	Name     string
	Calls    map[string]*FunctionNode
	CalledBy map[string]*FunctionNode
}
