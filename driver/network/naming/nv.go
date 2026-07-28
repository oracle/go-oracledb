/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and any and all patent rights owned or freely
** licensable by each licensor hereunder covering either (i) the unmodified
** Software as contributed to or provided by such licensor, or (ii) the Larger
** Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors),
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software and make,
** use, sell, offer for sale, import, export, have made, and have sold the
** Software and the Larger Work(s), and to sublicense the foregoing rights on
** either these or other terms.
**
** This license is subject to the following condition:
** The above copyright notice and either this complete permission notice or at
** a minimum a reference to the UPL must be included in all copies or
** substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

package naming

import (
	"context"
	"fmt"
	"strings"

	"github.com/oracle/go-driver/driver/common"
)

// Node represents a single name-value pair where value can be string or nested nodes
type Node struct {
	Name     string
	Value    string // non-empty for leaf nodes
	Children []Node // non-empty for branch nodes
}

// Parse parses a connection string and returns the root node using iterative stack-based parsing
func Parse(connectString string) (*Node, error) {
	connectString = strings.TrimSpace(connectString)
	if connectString == "" {
		return nil, common.NewOracleError(common.NamingParseFailed, fmt.Errorf("empty connection string"))
	}

	tokens := tokenize(connectString)

	if len(tokens) == 0 {
		return nil, common.NewOracleError(common.NamingParseFailed, fmt.Errorf("no valid tokens found"))
	}

	return parseIterative(tokens)
}

// tokenize breaks the connection string into meaningful tokens
// Separates parentheses and name/value pairs while handling whitespace (could be used to avoid comments)
func tokenize(input string) []string {
	var tokens []string
	var current strings.Builder
	current.Grow(32) // Pre-allocate buffer for typical token size
	inQuotes := false
	currentHasEquals := false

	// Helper function to flush current token - eliminates code duplication
	flushToken := func() {
		if current.Len() > 0 {
			token := strings.TrimSpace(current.String())
			if token != "" {
				tokens = append(tokens, token)
			}
			current.Reset()
			currentHasEquals = false
		}
	}

	for i, char := range input {
		switch {
		case char == '"':
			// Toggle quote mode
			inQuotes = !inQuotes
			current.WriteRune(char)
		case (char == '(' || char == ')') && !inQuotes:
			flushToken()
			tokens = append(tokens, string(char))
		case (char == ' ' || char == '\t' || char == '\n' || char == '\r') && !inQuotes:
			// Preserve whitespace inside unquoted NAME=VALUE tokens.
			// Since we can receive without quotes with space value after ezconnect parsing
			// Ex: (WALLET_LOCATION=C:\\my_wallet with space)
			// If we split on whitespace here, we'd produce extra tokens ("with", "space")
			// and the parser would fail.
			if currentHasEquals {
				current.WriteRune(char)
			} else {
				flushToken()
			}
		default:
			if char == '=' {
				currentHasEquals = true
			}
			current.WriteRune(char)
		}

		if i == len(input)-1 {
			flushToken()
		}
	}

	return tokens
}

// parseIterative uses stack-based parsing to build the node tree
func parseIterative(tokens []string) (*Node, error) {
	if len(tokens) == 0 {
		return nil, common.NewOracleError(common.NamingParseFailed, fmt.Errorf("no tokens to parse"))
	}

	// Stack to track nodes being built, root node reference
	var stack []*Node
	var root *Node

	// Helper function to parse name=value pairs
	// Normalizes NAME to uppercase, preserves VALUE casing
	parseNameValue := func(token string) (name, value string) {
		if strings.Contains(token, "=") {
			parts := strings.SplitN(token, "=", 2)
			value := parts[1]
			// Remove a single pair of wrapping double quotes, but preserve whitespace
			// inside them.
			if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
				value = value[1 : len(value)-1]
			}
			return strings.ToUpper(parts[0]), value
		}
		return strings.ToUpper(token), "" // Uppercase name
	}

	i := 0
	for i < len(tokens) {
		token := tokens[i]

		switch token {
		case "(":
			// Start of new node - expect name token next
			if i+1 >= len(tokens) {
				return nil, common.NewOracleError(common.NamingParseFailed, fmt.Errorf("expected name after '(' at position %d", i))
			}

			// Create new node with parsed name/value
			nameToken := tokens[i+1]
			if nameToken == "(" || nameToken == ")" {
				return nil, common.NewOracleError(common.NamingParseFailed, fmt.Errorf("expected name after '(', got '%s'", nameToken))
			}
			name, value := parseNameValue(nameToken)
			node := &Node{Name: name, Value: value}

			// Set as root if this is the first node
			if root == nil {
				root = node
			}

			// Push onto stack for building children
			stack = append(stack, node)
			i += 2 // Skip both '(' and name token

		case ")":
			// End of current node
			if len(stack) == 0 {
				return nil, common.NewOracleError(common.NamingParseFailed, fmt.Errorf("unexpected ')' at position %d", i))
			}

			// Pop node from stack to currentNode and reduce stack by 1
			currentNode := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			// Add as child to parent if parent exists
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, *currentNode)
			}
			i++

		default:
			return nil, common.NewOracleError(common.NamingParseFailed, fmt.Errorf("invalid syntax: unexpected token '%s'", token))
			/*
				This code can handle cases in case there are comma separated values, that can be included as well
					// Create child node with parsed name/value
					name, value := parseNameValue(token)
					childNode := Node{Name: name, Value: value}

					// Add to current node's children
					currentNode := stack[len(stack)-1]
					currentNode.Children = append(currentNode.Children, childNode)
					i++ */
		}
	}

	// Validate parsing completed successfully
	if len(stack) != 0 {
		return nil, common.NewOracleError(common.NamingParseFailed, fmt.Errorf("unmatched parentheses - %d unclosed nodes", len(stack)))
	}

	if root == nil {
		return nil, common.NewOracleError(common.NamingParseFailed, fmt.Errorf("no root node found"))
	}

	return root, nil
}

// GetValue retrieves a value by path (e.g., "DESCRIPTION/CONNECT_DATA/SERVICE_NAME")
// Path is case-insensitive for node names. Returns value and error.
func (n *Node) GetValue(path string) (string, error) {
	node, err := n.GetNode(path)
	if err != nil {
		return "", err
	}

	// Check if the found node actually has a value (is a leaf node)
	if node.Value == "" {
		return "", common.NewOracleError(common.NamingParseFailed, fmt.Errorf("node at path '%s' has no value (not a leaf node)", path))
	}

	return node.Value, nil
}

// GetNode retrieves a node by path (e.g., "DESCRIPTION/ADDRESS_LIST/ADDRESS")
// Path is case-insensitive for node names. Returns node and error.
func (n *Node) GetNode(path string) (*Node, error) {
	if path == "" {
		return nil, common.NewOracleError(common.NamingParseFailed, fmt.Errorf("empty path provided"))
	}

	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return nil, common.NewOracleError(common.NamingParseFailed, fmt.Errorf("invalid path format"))
	}

	// Normalize path parts to uppercase for case-insensitive comparison
	for i := range parts {
		parts[i] = strings.ToUpper(parts[i])
	}

	// First part must match the root node name
	if parts[0] != n.Name {
		return nil, common.NewOracleError(common.NamingParseFailed, fmt.Errorf("path must start with root node name '%s', but got '%s'", n.Name, parts[0]))
	}

	// If path is just the root name, return root
	if len(parts) == 1 {
		return n, nil
	}

	// Navigate through the remaining path parts
	current := n
	for i := 1; i < len(parts); i++ {
		part := parts[i]
		if part == "" {
			// Check for empty slashes
			return nil, common.NewOracleError(common.NamingParseFailed, fmt.Errorf("empty path segment at position %d in path '%s'", i, path))
		}

		found := false
		for j := range current.Children {
			if current.Children[j].Name == part {
				current = &current.Children[j]
				found = true
				break
			}
		}

		if !found {
			return nil, common.NewOracleError(common.NamingParseFailed, fmt.Errorf("node '%s' not found in path '%s' (at segment %d)", part, path, i))
		}
	}

	return current, nil
}

func (n *Node) ChildCount() int {
	return len(n.Children)
}

func (n *Node) GetChild(index int) (*Node, error) {
	if index < 0 || index >= len(n.Children) {
		return nil, common.NewOracleError(common.NamingParseFailed, fmt.Errorf("index %d out of bounds - node has %d children", index, len(n.Children)))
	}
	return &n.Children[index], nil
}

// ToString converts the node back to its original connection string format
// This recreates the parenthetical structure that was originally parsed
func (n *Node) ToString() string {
	var result strings.Builder

	result.WriteString("(")
	result.WriteString(n.Name)

	// Add "=" for both leaf nodes (with values) and branch nodes (with children)
	if n.Value != "" || len(n.Children) > 0 {
		result.WriteString("=")
	}

	// If it's a leaf node, add the value
	if n.Value != "" {
		result.WriteString(n.Value)
	}

	// Add all children without spaces
	for i := range n.Children {
		result.WriteString(n.Children[i].ToString())
	}

	result.WriteString(")")

	return result.String()
}

// _prepareExpectedValues prepares expected values.
// It takes the expected values as define in the tag and return an array of trimmed / lowered strings
func _prepareExpectedValues(values string) []string {
	subValues := strings.Split(values, ",")
	result := make([]string, len(subValues))
	for i, s := range subValues {
		result[i] = strings.ToLower(strings.TrimSpace(s))
	}
	return result
}

type ParsedConfig struct {
	ConnectString string

	// rootNode is the parsed TNS structure tree
	rootNode          *Node
	connectionContext *ConnectionContext
}

func resolveConnectStringUrl(connectStr string) (*conversionResult, error) {
	connStr := strings.TrimSpace(connectStr)

	// TNS Descriptor (long format)
	if strings.HasPrefix(connStr, "(") {
		return &conversionResult{
			tns:                connStr,
			unrecognizedParams: make(map[string]string),
		}, nil
	}

	// Try tnsnames.ora alias (future support)

	// Otherwise, parse as EZConnect
	return parseEzConnect(connStr)
}

// ParseDSNString parses a full Data Source Name string into ParsedConfig.
// The returned config can be used to create multiple independent iterators
// for parallel connection attempts via the NewConnectionAttemptIterator() method.
//
// Data Source Name Format:
//   - tcps://host:port/service?key=value&key=value (EZConnect)
//   - (DESCRIPTION=...) (TNS descriptor)
//   - alias (TNS alias, future support)
//
// Query parameters after '?' are parsed as connection properties.
func ParseDSNString(dsn string) (*ParsedConfig, error) {
	parts := strings.SplitN(dsn, "@", 2)
	if len(parts) == 2 {
		cred := strings.SplitN(parts[0], "/", 2)
		if len(cred) != 2 {
			return nil, common.NewOracleError(common.NamingDSNInvalid, nil, "credentials")
		}
		dsn = parts[1]
	}

	converted, err := resolveConnectStringUrl(dsn)
	if err != nil {
		return nil, err
	}
	connectStr := converted.tns

	connectionParts := strings.SplitN(connectStr, "?", 2)

	if len(connectionParts) == 2 {
		connectStr = connectionParts[0]
	}

	// Parse the TNS string into a node tree
	root, err := Parse(connectStr)
	if err != nil {
		return nil, common.NewOracleError(common.NamingParseFailed, err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		return nil, err
	}

	// Return parsed config with root and context
	// Users call NewIterator() to create iterators as needed
	return &ParsedConfig{
		ConnectString:     connectStr,
		rootNode:          root,
		connectionContext: ctx,
	}, nil

}

// Example usage:
//
//	config, _ := ParseDSNString(dsn)
//	iter := config.NewConnectionAttemptIterator()
//	for iter.HasNext() {
//	    opt := iter.Next()
//	    // Attempt connection with opt.ConnectString
//	}
func (pc *ParsedConfig) NewConnectionAttemptIterator(ctx context.Context) *ConnectionIterator {
	return NewConnectionIterator(ctx, pc.rootNode, pc.connectionContext)
}

/*========================Print=========================*/
/*
// String returns a string representation of the node tree
// In the data form being stored
func (n *Node) String() string {
	return n.stringWithIndent(0)
}

func (n *Node) stringWithIndent(indent int) string {
	spaces := strings.Repeat("  ", indent)
	var result strings.Builder

	if n.Value != "" {
		result.WriteString(fmt.Sprintf("%s%s = %s\n", spaces, n.Name, n.Value))
	} else {
		result.WriteString(fmt.Sprintf("%s%s\n", spaces, n.Name))
		for i := range n.Children {
			result.WriteString(n.Children[i].stringWithIndent(indent + 1))
		}
	}

	return result.String()
}

// CountNodes counts all nodes with the given name (searches entire subtree)
func (n *Node) CountNodes(name string) int {
	count := 0

	// Check current node
	if n.Name == name {
		count++
	}

	// Recursively check all children
	for i := range n.Children {
		count += n.Children[i].CountNodes(name)
	}

	return count
}
*/
/*
cond= desc.cond


cond.tostring()
*/
