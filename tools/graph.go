package tools

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"
)

func Analyze(project string, outputName string) error {
	foundFunctions, err := GetFunctions(project)
	if err != nil {
		return err
	}
	graph, err := BuildCallGraph(foundFunctions, project)
	if err != nil {
		return err
	}
	if outputName == "" {
		if project == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			project = cwd
		}
		outputName = strings.Split(project, "/")[len(strings.Split(project, "/"))-1]
	}
	err = GenerateDOT(graph, outputName+".dot")
	if err != nil {
		return fmt.Errorf("error generating DOT file: %w", err)
	}

	fmt.Println("Call graph generated in " + outputName + ".dot")
	return nil
}
func GenerateDOT(graph *CallGraph, filename string) error {
	var buf bytes.Buffer
	buf.WriteString("digraph G {\n")
	buf.WriteString("    rankdir=LR;\n")
	buf.WriteString("    node [style=filled, fillcolor=lightgray];\n")
	buf.WriteString("    edge [color=gray50];\n")

	// Map to keep track of clusters
	packageClusters := make(map[string][]*FunctionNode)
	structClusters := make(map[string][]*FunctionNode)
	otherNodes := []*FunctionNode{}

	// Organize nodes into clusters
	for _, node := range graph.Nodes {
		// Determine if the function is associated with a struct or package
		if strings.Contains(node.Name, ".") {
			parts := strings.Split(node.Name, ".")
			prefix := parts[0]
			if isUpperCase(prefix[0]) {
				// Likely a package
				packageClusters[prefix] = append(packageClusters[prefix], node)
			} else {
				// Likely a struct
				structClusters[prefix] = append(structClusters[prefix], node)
			}
		} else {
			otherNodes = append(otherNodes, node)
		}
	}

	// Function to write nodes within clusters
	writeNodes := func(nodes []*FunctionNode, clusterName string, clusterLabel string, color string) {
		if len(nodes) == 0 {
			return
		}
		// Sanitize the cluster name
		safeClusterName := sanitizeIdentifier(clusterName)
		buf.WriteString(fmt.Sprintf("    subgraph cluster_%s {\n", safeClusterName))
		buf.WriteString("        style=filled;\n")
		buf.WriteString(fmt.Sprintf("        color=\"%s\";\n", color))
		buf.WriteString(fmt.Sprintf("        label=\"%s\";\n", escapeStringForDOT(clusterLabel)))
		for _, node := range nodes {
			nodeID := sanitizeIdentifier(node.Name)
			label := node.Name
			buf.WriteString(fmt.Sprintf("        \"%s\" [label=\"%s\", shape=rectangle];\n", nodeID, escapeStringForDOT(label)))
		}
		buf.WriteString("    }\n")
	}

	// Define colors for clusters
	packageColor := "#AED6F1" // Light blue
	structColor := "#F9E79F"  // Light yellow

	// Write package clusters
	for pkg, nodes := range packageClusters {
		clusterLabel := fmt.Sprintf("Package: %s", pkg)
		writeNodes(nodes, "pkg_"+pkg, clusterLabel, packageColor)
	}

	// Write struct clusters
	for structName, nodes := range structClusters {
		clusterLabel := fmt.Sprintf("Struct: %s", structName)
		writeNodes(nodes, "struct_"+structName, clusterLabel, structColor)
	}

	// Write other nodes
	for _, node := range otherNodes {
		nodeID := sanitizeIdentifier(node.Name)
		label := node.Name
		buf.WriteString(fmt.Sprintf("    \"%s\" [label=\"%s\", shape=oval];\n", nodeID, escapeStringForDOT(label)))
	}

	// Write edges
	for _, node := range graph.Nodes {
		nodeID := sanitizeIdentifier(node.Name)
		for _, calledNode := range node.Calls {
			calledNodeID := sanitizeIdentifier(calledNode.Name)
			buf.WriteString(fmt.Sprintf("    \"%s\" -> \"%s\";\n", nodeID, calledNodeID))
		}
	}

	buf.WriteString("}\n")

	// Write to file
	return os.WriteFile(filename, buf.Bytes(), 0644)
}

// Helper function to check if a character is uppercase
func isUpperCase(c byte) bool {
	return c >= 'A' && c <= 'Z'
}

// Helper function to sanitize identifiers for DOT format
func sanitizeIdentifier(name string) string {
	// Replace invalid characters with underscores
	safeName := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			return r
		}
		return '_'
	}, name)
	// Append length of original name to ensure uniqueness
	return fmt.Sprintf("%s_%d", safeName, len(name))
}

// Helper function to escape strings for DOT labels
func escapeStringForDOT(s string) string {
	// Escape backslashes, double quotes, and special characters
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// Helper function to escape labels for DOT format
func escapeLabel(label string) string {
	// Use strconv.Quote to escape special characters
	return strconv.Quote(label)
}

// Helper function to sanitize node names for DOT format
func sanitizeNodeName(name string) string {
	return strings.ReplaceAll(name, "\"", "\\\"")
}

func BuildCallGraph(functions []FunctionInfo, projectRoot string) (*CallGraph, error) {
	graph := &CallGraph{Nodes: make(map[string]*FunctionNode)}

	// Create nodes for each function
	for _, fi := range functions {
		funcFullName := getFunctionFullName(fi)
		if _, exists := graph.Nodes[funcFullName]; !exists {
			graph.Nodes[funcFullName] = &FunctionNode{
				Name:     funcFullName,
				Calls:    make(map[string]*FunctionNode),
				CalledBy: make(map[string]*FunctionNode),
			}
		}
	}

	// Parse each function to find its calls
	for _, fi := range functions {
		funcFullName := getFunctionFullName(fi)
		node := graph.Nodes[funcFullName]

		// Get the function code
		funcCode, err := GetFunctionWithComments(fi, projectRoot)
		if err != nil {
			return nil, err
		}

		// Get function calls within this function
		calls, err := GetFunctionCalls(funcCode, fi, projectRoot)
		if err != nil {
			return nil, err
		}

		// For each function call, add an edge in the graph
		for _, call := range calls {
			calledFuncName := call.Function
			if call.Receiver != "" {
				calledFuncName = call.Receiver + "." + call.Function
			} else if call.Package != "" {
				calledFuncName = call.Package + "." + call.Function
			}

			// Ensure the called function node exists
			if _, exists := graph.Nodes[calledFuncName]; !exists {
				graph.Nodes[calledFuncName] = &FunctionNode{
					Name:     calledFuncName,
					Calls:    make(map[string]*FunctionNode),
					CalledBy: make(map[string]*FunctionNode),
				}
			}
			calledNode := graph.Nodes[calledFuncName]

			// Add the relationship
			node.Calls[calledFuncName] = calledNode
			calledNode.CalledBy[funcFullName] = node
		}
	}

	return graph, nil
}

func getFunctionFullName(fi FunctionInfo) string {
	if fi.StructName != "" {
		return fi.StructName + "." + fi.Name
	}
	return fi.Name
}
