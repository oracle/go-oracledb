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
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// Node aliases the descriptor node type now defined by oracle/config. Keeping
// this alias preserves the legacy naming iterator API while both paths share
// the same parser and tree representation.
type Node = oracleconfig.Node

// Parse parses a TNS descriptor using the normalized configuration parser.
//
// Input:
//   - connectString: TNS descriptor text to convert into a normalized node tree.
//
// Output:
//   - *Node: root of the parsed descriptor tree with normalized upper-case names.
//   - error: descriptor syntax or validation error when parsing fails.
func Parse(connectString string) (*Node, error) {
	return oracleconfig.ParseConnectorDescriptor(connectString)
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

func resolveConnectStringURL(connectStr string) (*conversionResult, error) {
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
			return nil, common.NewOracleError(oracleErrors.NamingDSNInvalid, nil, "credentials")
		}
		dsn = parts[1]
	}

	converted, err := resolveConnectStringURL(dsn)
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
		return nil, common.NewOracleError(oracleErrors.NamingParseFailed, err)
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
		result.WriteString(spaces + n.Name + " = " + n.Value + "\n")
	} else {
		result.WriteString(spaces + n.Name + "\n")
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
