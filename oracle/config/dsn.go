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

package config

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// Node represents a single name-value pair. Names are normalized to uppercase
// while parsing. Branch nodes contain child nodes, while leaf nodes contain a value.
type Node struct {
	// Name is the upper-case descriptor parameter or section name.
	Name string
	// Value contains the scalar value for leaf nodes.
	Value string
	// Children contains nested descriptor parameters for section nodes.
	Children []Node
}

// ParseConnectorDescriptor parses a TNS connection string into a name-value tree.
//
// Input:
//   - connectString: TNS descriptor text, including its nested parentheses.
//
// Output:
//   - *Node: root node of the descriptor tree with upper-case node names.
//   - error: tokenization or descriptor syntax error.
func ParseConnectorDescriptor(connectString string) (*Node, error) {

	tokens := tokenize(connectString)
	if len(tokens) == 0 {
		return nil, common.NewOracleError(oracleErrors.NamingTokensMissing, nil)
	}

	return parseIterative(tokens)
}

// tokenize splits a TNS string into parentheses and name-value tokens while
// retaining whitespace inside quoted values and unquoted values after '='.
//
// Input:
//   - input: raw TNS descriptor text.
//
// Output:
//   - []string: ordered tokens used by parseIterative to build the node tree.
func tokenize(input string) []string {
	var tokens []string
	var current strings.Builder
	current.Grow(32)
	inQuotes := false
	currentHasEquals := false

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
			inQuotes = !inQuotes
			current.WriteRune(char)
		case (char == '(' || char == ')') && !inQuotes:
			flushToken()
			tokens = append(tokens, string(char))
		case (char == ' ' || char == '\t' || char == '\n' || char == '\r') && !inQuotes:
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

// parseIterative builds the node tree from the TNS token sequence.
//
// Input:
//   - tokens: ordered tokens produced by tokenize.
//
// Output:
//   - *Node: root node containing the parsed descriptor tree.
//   - error: malformed token sequence, unbalanced parentheses, or missing root.
func parseIterative(tokens []string) (*Node, error) {
	if len(tokens) == 0 {
		return nil, common.NewOracleError(oracleErrors.NamingTokensMissing, nil)
	}

	var stack []*Node
	var root *Node

	parseNameValue := func(token string) (name, value string) {
		if strings.Contains(token, "=") {
			parts := strings.SplitN(token, "=", 2)
			value := parts[1]
			if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
				value = value[1 : len(value)-1]
			}
			return strings.ToUpper(parts[0]), value
		}
		return strings.ToUpper(token), ""
	}

	i := 0
	for i < len(tokens) {
		token := tokens[i]
		switch token {
		case "(":
			if i+1 >= len(tokens) {
				return nil, common.NewOracleError(oracleErrors.NamingParsePosition, nil, i)
			}
			nameToken := tokens[i+1]
			if nameToken == "(" || nameToken == ")" {
				return nil, common.NewOracleError(oracleErrors.NamingParseValue, nil, nameToken)
			}
			name, value := parseNameValue(nameToken)
			node := &Node{Name: name, Value: value}
			if root == nil {
				root = node
			}
			stack = append(stack, node)
			i += 2
		case ")":
			if len(stack) == 0 {
				return nil, common.NewOracleError(oracleErrors.NamingUnexpectedClosingParenthesis, nil, i)
			}
			currentNode := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, *currentNode)
			}
			i++
		default:
			return nil, common.NewOracleError(oracleErrors.NamingParseValue, nil, token)
		}
	}

	if len(stack) != 0 {
		return nil, common.NewOracleError(oracleErrors.NamingParseBounds, nil, len(stack), 0)
	}
	if root == nil {
		return nil, common.NewOracleError(oracleErrors.NamingParseFailed, nil)
	}
	return root, nil
}

// GetValue retrieves a scalar leaf value by a slash-separated path. Path
// components are matched without regard to case.
//
// Input:
//   - n: descriptor tree root used to start the lookup.
//   - path: slash-separated names beginning with the root node name.
//
// Output:
//   - string: value stored on the matching leaf node.
//   - error: path lookup failure or when the matching node has no scalar value.
func (n *Node) GetValue(path string) (string, error) {
	node, err := n.GetNode(path)
	if err != nil {
		return "", err
	}
	if node.Value == "" {
		return "", common.NewOracleError(oracleErrors.NamingParsePath, nil, path)
	}
	return node.Value, nil
}

// GetNode retrieves a node by a slash-separated path. Path components are
// matched without regard to case.
//
// Input:
//   - n: descriptor tree root used to start the lookup.
//   - path: slash-separated names beginning with the root node name.
//
// Output:
//   - *Node: the node matched by the path.
//   - error: missing, malformed, or unmatched path component.
func (n *Node) GetNode(path string) (*Node, error) {
	if path == "" {
		return nil, common.NewOracleError(oracleErrors.NamingInputMissing, nil, "path")
	}

	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return nil, common.NewOracleError(oracleErrors.NamingParsePath, nil, path)
	}
	for i := range parts {
		parts[i] = strings.ToUpper(parts[i])
	}
	if parts[0] != n.Name {
		return nil, common.NewOracleError(oracleErrors.NamingParseValues, nil, parts[0], n.Name)
	}
	if len(parts) == 1 {
		return n, nil
	}

	current := n
	for i := 1; i < len(parts); i++ {
		part := parts[i]
		if part == "" {
			return nil, common.NewOracleError(oracleErrors.NamingParsePathSegment, nil, "", path, i)
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
			return nil, common.NewOracleError(oracleErrors.NamingParsePathSegment, nil, part, path, i)
		}
	}
	return current, nil
}

// ChildCount returns the number of direct child nodes.
//
// Input:
//   - n: descriptor node whose children are counted.
//
// Output:
//   - int: number of direct child nodes; zero when there are no children.
func (n *Node) ChildCount() int {
	return len(n.Children)
}

// GetChild returns the direct child at index.
//
// Input:
//   - n: descriptor node whose child is requested.
//   - index: zero-based child position.
//
// Output:
//   - *Node: pointer to the requested child.
//   - error: non-nil when index is outside the range of n's children.
func (n *Node) GetChild(index int) (*Node, error) {
	if index < 0 || index >= len(n.Children) {
		return nil, common.NewOracleError(oracleErrors.NamingParseBounds, nil, index, len(n.Children))
	}
	return &n.Children[index], nil
}

// ToString serializes the node and its descendants in TNS parenthetical form.
//
// Input:
//   - n: descriptor node whose subtree is serialized.
//
// Output:
//   - string: TNS descriptor text with values quoted when required by syntax.
func (n *Node) ToString() string {
	var result strings.Builder
	result.WriteString("(")
	result.WriteString(n.Name)
	if n.Value != "" || len(n.Children) > 0 {
		result.WriteString("=")
	}
	if n.Value != "" {
		result.WriteString(descriptorValue(n.Value))
	}
	for i := range n.Children {
		result.WriteString(n.Children[i].ToString())
	}
	result.WriteString(")")
	return result.String()
}

const (
	defaultOraclePort = "1521"
	defaultProtocol   = driverCommon.ProtocolTCP
)

// hostPort represents a database host and port combination.
type hostPort struct {
	host string
	port string
}

// addressGroup represents a group of addresses for an ADDRESS_LIST.
type addressGroup struct {
	hosts []hostPort
}

// remoteAccessPoints contains the parsed components of an EZConnect DSN.
type remoteAccessPoints struct {
	protocol      driverCommon.Protocol
	addressGroups []addressGroup
	serviceName   string
	sid           string
	instanceName  string
}

// ConnectionPlans contains the resolved base configuration and normalized
// description-list plan produced from a DSN.
type ConnectionPlans struct {
	// BaseConfig is the owned working configuration with connector defaults, driver
	// flags, environment values, DSN credentials, and DSN-wide query settings
	// already applied. DESCRIPTION and ADDRESS settings are overlaid per attempt.
	BaseConfig *OracleDriverConfig
	// queryConnectDataSet records query-supplied CONNECT_DATA fields, including
	// explicitly empty values needed for precedence decisions.
	queryConnectDataSet map[string]bool
	// Plan contains the normalized DESCRIPTION_LIST or EZConnect alternatives.
	Plan ConnectionDescriptionPlan
}

// ConnectionDescriptionPlan contains the settings and alternatives found at
// the DESCRIPTION_LIST level. A single DESCRIPTION or EZConnect DSN is
// represented as a plan with one Description.
type ConnectionDescriptionPlan struct {
	// Failover controls whether connection attempts continue to later descriptions.
	Failover *bool
	// LoadBalance controls the initial order of descriptions.
	LoadBalance *bool
	// SourceRoute indicates that the connection routes through multiple addresses.
	SourceRoute *bool
	// AddressGroups holds plan-level address groups when the descriptor provides them.
	AddressGroups []ConnectionAddressGroup
	// Descriptions contains each DESCRIPTION or the normalized EZConnect target.
	Descriptions []ConnectionDescription
	// AdditionalSettings preserves unrecognized scalar settings at the plan level.
	AdditionalSettings map[string]string
}

// ConnectionDescription contains the settings scoped to one DESCRIPTION.
// Pointer fields preserve the distinction between an omitted setting and an
// explicitly supplied zero value.
type ConnectionDescription struct {
	// Failover controls whether later addresses in this description are tried.
	Failover *bool
	// LoadBalance controls the initial order of this description's addresses.
	LoadBalance *bool
	// SourceRoute enables source routing through this description's address groups.
	SourceRoute *bool
	// UseSNI enables TLS Server Name Indication for this description.
	UseSNI *bool
	// ConnectTimeout limits the overall connection establishment time.
	ConnectTimeout *time.Duration
	// RetryCount is the number of additional passes over the selected addresses.
	RetryCount *int
	// RetryDelay is the wait between retry passes.
	RetryDelay *time.Duration
	// TransportConnectTimeout limits each socket or proxy transport connection.
	TransportConnectTimeout *time.Duration
	// ExpireTime is the interval for network liveness probes, in minutes.
	ExpireTime *int
	// RecvTimeout limits reads from the established Oracle Net connection.
	RecvTimeout *time.Duration
	// SendTimeout limits writes to the established Oracle Net connection.
	SendTimeout *time.Duration
	// SDU is the Oracle Net Session Data Unit size in bytes.
	SDU *int
	// RecvBufSize is the socket receive buffer size in bytes.
	RecvBufSize *int
	// SendBufSize is the socket send buffer size in bytes.
	SendBufSize *int
	// Enable contains the Oracle Net ENABLE setting, such as BROKEN.
	Enable *string
	// ConnectionIDPrefix prefixes generated Oracle Net connection identifiers.
	ConnectionIDPrefix *string
	// Compression enables Oracle Net network compression.
	Compression *bool
	// CompressionLevels lists compression algorithms or levels requested by the descriptor.
	CompressionLevels []string
	// TypeOfService contains the requested Oracle Net service type.
	TypeOfService *string
	// Security contains TLS and wallet settings scoped to this description.
	Security *Security
	// SecuritySet records which Security members were explicitly specified.
	SecuritySet map[string]bool
	// ConnectData contains the database service and DRCP settings for this description.
	ConnectData *ConnectData
	// ConnectDataSet records which ConnectData members were explicitly specified.
	ConnectDataSet map[string]bool
	// AddressGroups contains this description's ordered ADDRESS_LIST or EZConnect groups.
	AddressGroups []ConnectionAddressGroup
	// AdditionalSettings preserves unrecognized scalar settings at description scope.
	AdditionalSettings map[string]string
}

// ConnectionAddressGroup represents either an ADDRESS_LIST or the implicit
// group formed by direct ADDRESS children of a DESCRIPTION.
type ConnectionAddressGroup struct {
	// Failover controls whether later access points in this group are tried.
	Failover *bool
	// LoadBalance controls the initial order of access points in this group.
	LoadBalance *bool
	// SourceRoute enables source routing for this group.
	SourceRoute *bool
	// AccessPoints contains the protocol endpoints in the group.
	AccessPoints []RemoteAccessPoint
	// AdditionalSettings preserves unrecognized scalar settings at group scope.
	AdditionalSettings map[string]string
}

// RemoteAccessPoint is one protocol endpoint from an ADDRESS node.
type RemoteAccessPoint struct {
	// Protocol is the transport protocol, normally TCP or TCPS.
	Protocol driverCommon.Protocol
	// Host is the hostname or IP address of the database listener.
	Host string
	// Port is the database listener port.
	Port int
	// HTTPSProxy optionally selects an HTTP CONNECT proxy for TCPS.
	HTTPSProxy *string
	// HTTPSProxyPort is the HTTP CONNECT proxy port when HTTPSProxy is set.
	HTTPSProxyPort *int
	// RecvBufSize overrides the socket receive buffer for this endpoint.
	RecvBufSize *int
	// SendBufSize overrides the socket send buffer for this endpoint.
	SendBufSize *int
	// AdditionalSettings preserves unrecognized scalar settings at address scope.
	AdditionalSettings map[string]string
}

// ConnectData contains database identification and connection parameters from
// the CONNECT_DATA section of a descriptor.
type ConnectData struct {
	// ServiceName identifies the database service requested by the client.
	ServiceName string
	// SID identifies an instance by Oracle system identifier.
	SID string
	// Server selects the server mode, such as DEDICATED or POOLED.
	Server string
	// PoolName selects a DRCP pool with the TNS POOL_NAME parameter.
	PoolName string
	// PoolConnectionClass selects the DRCP connection class.
	PoolConnectionClass string
	// PoolPurity selects the requested DRCP session purity.
	PoolPurity string
	// PoolBoundary selects the DRCP pool boundary behavior.
	PoolBoundary string
	// ServiceTag carries the service tag used by Oracle Net connect data.
	ServiceTag string
	// InstanceName identifies a particular database instance.
	InstanceName string
	// FailoverMode contains the requested client-side failover mode.
	FailoverMode string
	// FailoverModeSettings contains nested TAF settings such as TYPE, METHOD,
	// RETRIES, and DELAY when FAILOVER_MODE is represented as a section.
	FailoverModeSettings map[string]string
	// HS enables heterogeneous services when set by the descriptor.
	HS string
	// RDBDatabase identifies an Rdb database when applicable.
	RDBDatabase string
	// GlobalName identifies the requested global database name.
	GlobalName string
	// ConnectionIDPrefix prefixes generated Oracle Net connection identifiers.
	ConnectionIDPrefix string
	// AdditionalSettings preserves other scalar CONNECT_DATA parameters.
	AdditionalSettings map[string]string
	// StructuredSettings preserves CONNECT_DATA sections whose values are
	// themselves scalar name/value pairs, such as client identifier data.
	StructuredSettings map[string]map[string]string
}

// Security contains TLS and wallet parameters from a SECURITY section.
type Security struct {
	// SSLServerCertDN is the distinguished name expected in the server certificate.
	SSLServerCertDN string
	// SSLServerDNMatch controls matching the server certificate to SSLServerCertDN.
	SSLServerDNMatch bool
	// SSLAllowWeakDNMatch permits less strict server distinguished-name matching.
	SSLAllowWeakDNMatch bool
	// WalletLocation identifies the directory containing client wallet material.
	WalletLocation string
	// MyWalletDirectory is the Oracle Net alias for WalletLocation.
	MyWalletDirectory string
	// AdditionalSettings preserves other scalar SECURITY parameters.
	AdditionalSettings map[string]string
	// StructuredSettings preserves nested SECURITY parameters as serialized TNS
	// subtrees. WALLET_LOCATION commonly uses this form.
	StructuredSettings map[string]string
}

var protocolPattern = regexp.MustCompile(`^([a-zA-Z0-9]+)://`)

// parseEazyConnectDescriptor parses the core EZConnect format.
// Format: [protocol://]host[:port][,host2[:port2]][;host3][/service][:server][/instance]
//
// Input:
//   - url: EZConnect target without user-supplied query settings.
//
// Output:
//   - *remoteAccessPoints: protocol, host groups, service/SID, and instance data.
//   - string: explicit server mode, such as DEDICATED, SHARED, or POOLED; empty
//     when the URL does not specify one.
//   - error: invalid protocol, host, or port syntax.
func parseEazyConnectDescriptor(url string) (*remoteAccessPoints, string, error) {
	result := &remoteAccessPoints{}

	var err error
	if matches := protocolPattern.FindStringSubmatch(url); matches != nil {
		result.protocol, err = driverCommon.NormalizeProtocol(matches[1])
		if err != nil {
			return nil, "", common.NewOracleError(oracleErrors.NamingEzConnectError, nil, matches[1], "protocol")
		}
		url = protocolPattern.ReplaceAllString(url, "")
	} else {
		result.protocol = defaultProtocol
	}

	parts := strings.Split(url, "/")
	hostPart := parts[0]

	addressGroups, err := parseAddressGroups(hostPart)
	if err != nil {
		return nil, "", err
	}
	result.addressGroups = addressGroups
	var serverMode = ""
	if len(parts) > 1 && parts[1] != "" {
		serviceAndMode := parts[1]
		colonIdx := strings.Index(serviceAndMode, ":")
		if colonIdx != -1 {
			result.serviceName = serviceAndMode[:colonIdx]
			serverMode = strings.ToUpper(serviceAndMode[colonIdx+1:])
		} else {
			result.serviceName = serviceAndMode
		}
	}

	if len(parts) > 2 && parts[2] != "" {
		result.instanceName = parts[2]
	}

	return result, serverMode, nil
}

// parseAddressGroups parses semicolon-separated address groups. Each group
// contains comma-separated hosts.
//
// Input:
//   - hostStr: host portion of EZConnect, with semicolons separating groups.
//
// Output:
//   - []addressGroup: parsed address groups in source order.
//   - error: missing host or invalid host/port syntax in any group.
func parseAddressGroups(hostStr string) ([]addressGroup, error) {
	if hostStr == "" {
		return nil, common.NewOracleError(oracleErrors.NamingEzConnectError, nil, "", "host")
	}

	groupStrs := strings.Split(hostStr, ";")
	groups := make([]addressGroup, 0, len(groupStrs))
	for _, groupStr := range groupStrs {
		groupStr = strings.TrimSpace(groupStr)
		if groupStr == "" {
			continue
		}
		hosts, err := parseHostList(groupStr)
		if err != nil {
			return nil, err
		}
		groups = append(groups, addressGroup{hosts: hosts})
	}

	if len(groups) == 0 {
		return nil, common.NewOracleError(oracleErrors.NamingEzConnectError, nil, hostStr, "host")
	}
	return groups, nil
}

// parseHostList parses comma-separated host:port combinations and supports
// IPv6 addresses in brackets, such as [::1]:1521.
//
// Input:
//   - hostStr: one group of comma-separated hosts, optionally with ports.
//
// Output:
//   - []hostPort: hosts with explicit or default listener ports.
//   - error: missing host or invalid port.
func parseHostList(hostStr string) ([]hostPort, error) {
	if hostStr == "" {
		return nil, common.NewOracleError(oracleErrors.NamingEzConnectError, nil, "", "host")
	}

	var hosts []hostPort
	var currentHost strings.Builder
	var currentPort strings.Builder
	inBrackets := false
	parsingPort := false

	for i := 0; i < len(hostStr); i++ {
		ch := hostStr[i]
		if ch == '[' {
			inBrackets = true
			currentHost.WriteByte(ch)
		} else if ch == ']' {
			inBrackets = false
			currentHost.WriteByte(ch)
		} else if ch == ':' {
			if inBrackets {
				currentHost.WriteByte(ch)
			} else {
				parsingPort = true
			}
		} else if ch == ',' {
			host := strings.TrimSpace(currentHost.String())
			port := strings.TrimSpace(currentPort.String())
			if port == "" {
				port = defaultOraclePort
			}
			if err := validatePort(port); err != nil {
				return nil, err
			}
			hosts = append(hosts, hostPort{host: host, port: port})
			currentHost.Reset()
			currentPort.Reset()
			parsingPort = false
		} else if parsingPort {
			currentPort.WriteByte(ch)
		} else {
			currentHost.WriteByte(ch)
		}
	}

	host := strings.TrimSpace(currentHost.String())
	port := strings.TrimSpace(currentPort.String())
	if host == "" {
		return nil, common.NewOracleError(oracleErrors.NamingEzConnectError, nil, "", "host")
	}
	if port == "" {
		port = defaultOraclePort
	}
	if err := validatePort(port); err != nil {
		return nil, err
	}

	hosts = append(hosts, hostPort{host: host, port: port})
	return hosts, nil
}

// validatePort checks that port is a decimal TCP port in the usable range.
//
// Input:
//   - port: decimal port string to validate.
//
// Output:
//   - error: non-nil when port is not an integer from 1 through 65535.
func validatePort(port string) error {
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return common.NewOracleError(oracleErrors.NamingEzConnectError, err, port, "port")
	}
	if portNum < 1 || portNum > 65535 {
		return common.NewOracleError(oracleErrors.NamingEzConnectError, nil, port, "port")
	}
	return nil
}

// _isConnectDescriptorFormat reports whether dsn begins with a TNS descriptor.
//
// Input:
//   - dsn: connection target to classify.
//
// Output:
//   - bool: true when the target begins with an opening parenthesis.
func _isConnectDescriptorFormat(dsn string) bool {
	return strings.HasPrefix(dsn, "(")
}

// HandleDsn parses an EZConnect or TNS descriptor DSN into normalized
// connection plans. It clones the caller's configuration and applies all
// DSN-wide query settings to that owned base. DESCRIPTION and ADDRESS settings
// are resolved only when the iterator selects an attempt.
//
// Input:
//   - dsn: EZConnect or TNS descriptor, optionally including credentials and
//     query parameters.
//   - drvConfig: working configuration owned by the caller. DSN credentials,
//     query settings, and the normalized descriptor are applied to it in place.
//
// Output:
//   - *ConnectionPlans: normalized target alternatives, base configuration, and
//     query settings retained for inspection.
//   - error: invalid DSN, conflicting credentials, query parsing, or descriptor
//     validation error.
func HandleDsn(dsn string, drvConfig *OracleDriverConfig) (*ConnectionPlans, error) {
	// The caller supplies an owned working copy. The public driver clones its
	// user-provided configuration before calling HandleDsn.
	baseConfig := drvConfig
	dsnToUse := dsn
	if len(dsnToUse) == 0 {
		dsnToUse = baseConfig.ConnectDescriptor
	}
	if len(dsnToUse) == 0 {
		return nil, common.NewOracleError(oracleErrors.NamingDSNInvalid, nil, "descriptor")
	}
	// Step 2: normalize whitespace without changing quoted values, then separate
	// optional credentials from the connection target.
	cleanDSN := driverCommon.StripSpacesOutsideQuotes(dsnToUse)
	querySettings := make(map[string]string)
	effectiveDSN := cleanDSN

	// Extract credentials when they are present in the DSN authority.
	parts := strings.SplitN(effectiveDSN, "@", 2)
	effectiveDSN = parts[0]
	if len(parts) == 2 {
		credentials := strings.SplitN(parts[0], "/", 2)
		if len(credentials) != 2 || credentials[0] == "" {
			return nil, common.NewOracleError(oracleErrors.NamingDSNInvalid, nil, dsn)
		}
		if baseConfig.Credentials.User != "" || baseConfig.Credentials.Password != "" {
			return nil, common.NewOracleError(oracleErrors.ConflictingConnectionParameterSource, nil, "credentials")
		}
		baseConfig.Credentials.User = credentials[0]
		baseConfig.Credentials.Password = credentials[1]
		effectiveDSN = parts[1]
	}

	// Step 3: parse query settings and apply them to the owned base configuration.
	// They are shared by every target; only DESCRIPTION and ADDRESS values wait
	// until the iterator selects a specific attempt.
	queryIndex := strings.LastIndex(effectiveDSN, "?")
	if queryIndex >= 0 {
		lastCloseParen := strings.LastIndex(effectiveDSN, ")")
		if lastCloseParen < queryIndex {
			connectionParts := strings.SplitN(effectiveDSN, "?", 2)
			effectiveDSN = connectionParts[0]
			if connectionParts[1] != "" {
				var err error
				querySettings, err = QueryStringToMap(connectionParts[1])
				if err != nil {
					return nil, common.NewOracleError(oracleErrors.NamingDSNInvalid, err, dsn)
				}
			}
		}
	}

	if err := baseConfig.AssignFromMap(querySettings); err != nil {
		return nil, common.NewOracleError(oracleErrors.NamingDSNInvalid, err, dsn)
	}
	queryConnectDataSet := connectDataFieldsFromQuerySettings(baseConfig, querySettings)
	// Step 4: parse the remaining target into one normalized plan, regardless
	// of whether the input used EZConnect or a TNS descriptor.
	var plan ConnectionDescriptionPlan
	var err error
	if _isConnectDescriptorFormat(effectiveDSN) {
		plan, err = handleConnectDescriptorFormatDsn(effectiveDSN)
	} else {
		plan, err = handleEasyConnectDescriptorFormatDsn(effectiveDSN)
	}
	if err != nil {
		return nil, err
	}
	// Step 5: validate the merged DSN-wide configuration. Keep descriptor
	// policies on the plan so DESCRIPTION_LIST settings do not become defaults
	// for the distinct DESCRIPTION or ADDRESS_LIST scopes.
	if err := baseConfig.Validate(); err != nil {
		return nil, common.NewOracleError(oracleErrors.NamingDSNInvalid, err, dsn)
	}

	// Retain the normalized DSN on the owned configuration for callers that
	// inspect the resolved plans.
	baseConfig.ConnectDescriptor = effectiveDSN

	// Step 6: return the merged global configuration and unresolved alternatives.
	return &ConnectionPlans{
		BaseConfig:          baseConfig,
		queryConnectDataSet: queryConnectDataSet,
		Plan:                plan,
	}, nil
}

// HandleRedirectDsn parses a listener redirect descriptor into connection
// plans. Redirect packets can carry CONNECT_DATA separately from the address
// descriptor, so this entry point permits DESCRIPTION nodes without
// CONNECT_DATA. The supplied configuration is cloned because redirect-specific
// settings must not mutate the active connection's configuration.
//
// Input:
//   - dsn: TNS DESCRIPTION or DESCRIPTION_LIST supplied by the listener.
//   - baseConfig: effective configuration of the active connection; nil uses
//     the default Oracle driver configuration.
//
// Output:
//   - *ConnectionPlans: normalized redirect targets using the cloned base
//     configuration.
//   - error: malformed redirect descriptor or invalid inherited configuration.
func HandleRedirectDsn(dsn string, baseConfig *OracleDriverConfig) (*ConnectionPlans, error) {
	config := NewOracleDriverConfig()
	if baseConfig != nil {
		config = baseConfig.Clone()
	}

	cleanDSN := driverCommon.StripSpacesOutsideQuotes(dsn)
	plan, err := handleConnectDescriptorFormatDsnWithConnectData(cleanDSN, false)
	if err != nil {
		return nil, err
	}
	if err := config.Validate(); err != nil {
		return nil, common.NewOracleError(oracleErrors.NamingDSNInvalid, err, dsn)
	}
	config.ConnectDescriptor = cleanDSN
	return &ConnectionPlans{BaseConfig: config, Plan: plan}, nil
}

// handleConnectDescriptorFormatDsn parses a TNS descriptor into the normalized
// connection-description plan.
//
// Input:
//   - dsn: TNS DESCRIPTION or DESCRIPTION_LIST text.
//
// Output:
//   - ConnectionDescriptionPlan: plan policy, address groups, and normalized
//     DESCRIPTION alternatives.
//   - error: invalid descriptor structure, policy value, endpoint, or setting.
func handleConnectDescriptorFormatDsn(dsn string) (ConnectionDescriptionPlan, error) {
	return handleConnectDescriptorFormatDsnWithConnectData(dsn, true)
}

// handleConnectDescriptorFormatDsnWithConnectData parses a TNS descriptor and
// optionally requires each DESCRIPTION to provide CONNECT_DATA.
func handleConnectDescriptorFormatDsnWithConnectData(dsn string, requireConnectData bool) (ConnectionDescriptionPlan, error) {
	root, err := ParseConnectorDescriptor(dsn)
	if err != nil {
		return ConnectionDescriptionPlan{}, err
	}

	plan := ConnectionDescriptionPlan{}
	switch root.Name {
	case "ADDRESS":
		accessPoint, parseErr := parseRemoteAccessPoint(root)
		if parseErr != nil {
			return ConnectionDescriptionPlan{}, parseErr
		}
		plan.Descriptions = append(plan.Descriptions, ConnectionDescription{
			AddressGroups: []ConnectionAddressGroup{{AccessPoints: []RemoteAccessPoint{accessPoint}}},
		})
	case "DESCRIPTION":
		description, err := parseConnectionDescription(root)
		if err != nil {
			return ConnectionDescriptionPlan{}, err
		}
		plan.Descriptions = append(plan.Descriptions, description)
	case "DESCRIPTION_LIST":
		for i := range root.Children {
			child := &root.Children[i]
			switch child.Name {
			case "FAILOVER":
				plan.Failover, err = parseBoolPointer(child)
			case "LOAD_BALANCE":
				plan.LoadBalance, err = parseBoolPointer(child)
			case "SOURCE_ROUTE":
				plan.SourceRoute, err = parseBoolPointer(child)
			case "ADDRESS_LIST":
				var group ConnectionAddressGroup
				group, err = parseConnectionAddressGroup(child)
				if err == nil {
					plan.AddressGroups = append(plan.AddressGroups, group)
				}
			case "DESCRIPTION":
				var description ConnectionDescription
				description, err = parseConnectionDescription(child)
				if err == nil {
					plan.Descriptions = append(plan.Descriptions, description)
				}
			default:
				if len(child.Children) != 0 {
					return ConnectionDescriptionPlan{}, common.NewOracleError(oracleErrors.NamingContextError, nil, child.Name, "DESCRIPTION_LIST")
				}
				if plan.AdditionalSettings == nil {
					plan.AdditionalSettings = make(map[string]string)
				}
				plan.AdditionalSettings[child.Name] = child.Value
			}
			if err != nil {
				return ConnectionDescriptionPlan{}, descriptorValueError(child, err)
			}
		}
	default:
		return ConnectionDescriptionPlan{}, common.NewOracleError(oracleErrors.NamingContextError, nil, root.Name, "DESCRIPTION or DESCRIPTION_LIST")
	}

	if len(plan.Descriptions) == 0 {
		return ConnectionDescriptionPlan{}, common.NewOracleError(oracleErrors.NamingContextError, nil, root.Name, "DESCRIPTION")
	}
	for _, description := range plan.Descriptions {
		if requireConnectData && description.ConnectData == nil {
			return ConnectionDescriptionPlan{}, common.NewOracleError(oracleErrors.NamingContextError, nil, "DESCRIPTION", "CONNECT_DATA")
		}
		if len(description.AddressGroups) == 0 && len(plan.AddressGroups) == 0 {
			return ConnectionDescriptionPlan{}, common.NewOracleError(oracleErrors.NamingContextError, nil, "DESCRIPTION", "ADDRESS or ADDRESS_LIST")
		}
	}
	return plan, nil
}

// parseConnectionDescription converts a DESCRIPTION subtree into typed fields.
//
// Input:
//   - node: DESCRIPTION node whose direct settings and nested sections are parsed.
//
// Output:
//   - ConnectionDescription: parsed policy, transport, security, connect-data,
//     endpoint, and preserved additional settings.
//   - error: invalid value or unsupported nested structure in the description.
func parseConnectionDescription(node *Node) (ConnectionDescription, error) {
	description := ConnectionDescription{}
	var directAddresses []RemoteAccessPoint
	flushDirectAddresses := func() {
		if len(directAddresses) == 0 {
			return
		}
		description.AddressGroups = append(description.AddressGroups, ConnectionAddressGroup{
			AccessPoints: directAddresses,
		})
		directAddresses = nil
	}

	for i := range node.Children {
		child := &node.Children[i]
		var err error
		switch child.Name {
		case "FAILOVER":
			description.Failover, err = parseBoolPointer(child)
		case "LOAD_BALANCE":
			description.LoadBalance, err = parseBoolPointer(child)
		case "SOURCE_ROUTE":
			description.SourceRoute, err = parseBoolPointer(child)
		case "USE_SNI":
			description.UseSNI, err = parseBoolPointer(child)
		case "CONNECT_TIMEOUT":
			description.ConnectTimeout, err = parseDurationPointer(child)
		case "RETRY_COUNT":
			description.RetryCount, err = parseIntPointer(child)
		case "RETRY_DELAY":
			description.RetryDelay, err = parseDurationPointer(child)
		case "TRANSPORT_CONNECT_TIMEOUT":
			description.TransportConnectTimeout, err = parseDurationPointer(child)
		case "EXPIRE_TIME":
			description.ExpireTime, err = parseIntPointer(child)
		case "RECV_TIMEOUT":
			description.RecvTimeout, err = parseDurationPointer(child)
		case "SEND_TIMEOUT":
			description.SendTimeout, err = parseDurationPointer(child)
		case "SDU":
			description.SDU, err = parseIntPointer(child)
		case "RECV_BUF_SIZE":
			description.RecvBufSize, err = parseIntPointer(child)
		case "SEND_BUF_SIZE":
			description.SendBufSize, err = parseIntPointer(child)
		case "ENABLE":
			description.Enable = stringPointer(child.Value)
		case "CONNECTION_ID_PREFIX":
			description.ConnectionIDPrefix = stringPointer(child.Value)
		case "COMPRESSION":
			description.Compression, err = parseBoolPointer(child)
		case "COMPRESSION_LEVELS":
			for _, level := range child.Children {
				if level.Name != "LEVEL" {
					return ConnectionDescription{}, common.NewOracleError(oracleErrors.NamingContextError, nil, level.Name, "COMPRESSION_LEVELS")
				}
				description.CompressionLevels = append(description.CompressionLevels, level.Value)
			}
		case "TYPE_OF_SERVICE":
			description.TypeOfService = stringPointer(child.Value)
		case "SECURITY":
			description.Security, description.SecuritySet, err = parseSecurity(child)
		case "CONNECT_DATA":
			description.ConnectData, description.ConnectDataSet, err = parseConnectData(child)
		case "ADDRESS":
			var address RemoteAccessPoint
			address, err = parseRemoteAccessPoint(child)
			if err == nil {
				directAddresses = append(directAddresses, address)
			}
		case "ADDRESS_LIST":
			flushDirectAddresses()
			var group ConnectionAddressGroup
			group, err = parseConnectionAddressGroup(child)
			if err == nil {
				description.AddressGroups = append(description.AddressGroups, group)
			}
		default:
			if len(child.Children) != 0 {
				return ConnectionDescription{}, common.NewOracleError(oracleErrors.NamingContextError, nil, child.Name, "DESCRIPTION")
			}
			if description.AdditionalSettings == nil {
				description.AdditionalSettings = make(map[string]string)
			}
			description.AdditionalSettings[child.Name] = child.Value
		}
		if err != nil {
			return ConnectionDescription{}, descriptorValueError(child, err)
		}
	}
	flushDirectAddresses()
	return description, nil
}

// parseConnectionAddressGroup parses an ADDRESS_LIST and its scoped settings.
//
// Input:
//   - node: ADDRESS_LIST node containing group policy and ADDRESS children.
//
// Output:
//   - ConnectionAddressGroup: parsed policy, endpoints, and additional settings.
//   - error: invalid group value, nested structure, or missing ADDRESS child.
func parseConnectionAddressGroup(node *Node) (ConnectionAddressGroup, error) {
	group := ConnectionAddressGroup{}
	for i := range node.Children {
		child := &node.Children[i]
		var err error
		switch child.Name {
		case "FAILOVER":
			group.Failover, err = parseBoolPointer(child)
		case "LOAD_BALANCE":
			group.LoadBalance, err = parseBoolPointer(child)
		case "SOURCE_ROUTE":
			group.SourceRoute, err = parseBoolPointer(child)
		case "ADDRESS":
			var address RemoteAccessPoint
			address, err = parseRemoteAccessPoint(child)
			if err == nil {
				group.AccessPoints = append(group.AccessPoints, address)
			}
		default:
			if len(child.Children) != 0 {
				return ConnectionAddressGroup{}, common.NewOracleError(oracleErrors.NamingContextError, nil, child.Name, "ADDRESS_LIST")
			}
			if group.AdditionalSettings == nil {
				group.AdditionalSettings = make(map[string]string)
			}
			group.AdditionalSettings[child.Name] = child.Value
		}
		if err != nil {
			return ConnectionAddressGroup{}, descriptorValueError(child, err)
		}
	}
	if len(group.AccessPoints) == 0 {
		return ConnectionAddressGroup{}, common.NewOracleError(oracleErrors.NamingContextError, nil, node.Name, "ADDRESS")
	}
	return group, nil
}

// parseRemoteAccessPoint parses a protocol endpoint from an ADDRESS node.
//
// Input:
//   - node: ADDRESS node containing protocol, host, port, and optional transport
//     settings.
//
// Output:
//   - RemoteAccessPoint: normalized protocol endpoint and address-scoped options.
//   - error: invalid protocol, host, port, or nested address setting.
func parseRemoteAccessPoint(node *Node) (RemoteAccessPoint, error) {
	address := RemoteAccessPoint{Protocol: defaultProtocol, Port: 1521}
	for i := range node.Children {
		child := &node.Children[i]
		var err error
		switch child.Name {
		case "PROTOCOL":
			address.Protocol, err = driverCommon.NormalizeProtocol(child.Value)
		case "HOST":
			address.Host = child.Value
		case "PORT":
			err = validatePort(child.Value)
			if err == nil {
				address.Port, err = strconv.Atoi(child.Value)
			}
		case "HTTPS_PROXY":
			address.HTTPSProxy = stringPointer(child.Value)
		case "HTTPS_PROXY_PORT":
			var port int
			port, err = strconv.Atoi(child.Value)
			if err == nil && (port < 0 || port > 65535) {
				err = common.NewOracleError(oracleErrors.InvalidConnectionParameter, nil, child.Value, "port", "0 to 65535")
			}
			if err == nil {
				address.HTTPSProxyPort = &port
			}
		case "RECV_BUF_SIZE":
			address.RecvBufSize, err = parseIntPointer(child)
		case "SEND_BUF_SIZE":
			address.SendBufSize, err = parseIntPointer(child)
		default:
			if len(child.Children) != 0 {
				return RemoteAccessPoint{}, common.NewOracleError(oracleErrors.NamingContextError, nil, child.Name, "ADDRESS")
			}
			if address.AdditionalSettings == nil {
				address.AdditionalSettings = make(map[string]string)
			}
			address.AdditionalSettings[child.Name] = child.Value
		}
		if err != nil {
			return RemoteAccessPoint{}, descriptorValueError(child, err)
		}
	}
	if address.Host == "" {
		return RemoteAccessPoint{}, common.NewOracleError(oracleErrors.NamingContextError, nil, node.Name, "HOST")
	}
	return address, nil
}

// parseConnectData converts a CONNECT_DATA subtree into typed connection values
// and records which settings were explicitly present.
//
// Input:
//   - node: CONNECT_DATA node containing service, instance, DRCP, and other values.
//
// Output:
//   - *ConnectData: parsed typed values plus preserved additional/structured data.
//   - map[string]bool: upper-case names of fields explicitly present in node.
//   - error: nested structure that cannot be represented or parsed.
func parseConnectData(node *Node) (*ConnectData, map[string]bool, error) {
	connectData := &ConnectData{}
	set := make(map[string]bool)
	for i := range node.Children {
		child := &node.Children[i]
		if child.Name != "FAILOVER_MODE" && len(child.Children) > 0 {
			if connectData.StructuredSettings == nil {
				connectData.StructuredSettings = make(map[string]map[string]string)
			}
			section := make(map[string]string, len(child.Children))
			for j := range child.Children {
				setting := &child.Children[j]
				if len(setting.Children) > 0 {
					return nil, nil, common.NewOracleError(oracleErrors.NamingContextError, nil, setting.Name, child.Name)
				}
				section[setting.Name] = setting.Value
			}
			connectData.StructuredSettings[child.Name] = section
			set[child.Name] = true
			continue
		}
		switch child.Name {
		case "SERVICE_NAME":
			connectData.ServiceName = child.Value
		case "SID":
			connectData.SID = child.Value
		case "SERVER":
			connectData.Server = child.Value
		case "POOL_NAME":
			connectData.PoolName = child.Value
		case "POOL_CONNECTION_CLASS":
			connectData.PoolConnectionClass = child.Value
		case "POOL_PURITY":
			connectData.PoolPurity = child.Value
		case "POOL_BOUNDARY":
			connectData.PoolBoundary = child.Value
		case "SERVICE_TAG":
			connectData.ServiceTag = child.Value
		case "INSTANCE_NAME":
			connectData.InstanceName = child.Value
		case "FAILOVER_MODE":
			if len(child.Children) == 0 {
				connectData.FailoverMode = child.Value
			} else {
				connectData.FailoverModeSettings = make(map[string]string, len(child.Children))
				for j := range child.Children {
					setting := &child.Children[j]
					if len(setting.Children) > 0 {
						return nil, nil, common.NewOracleError(oracleErrors.NamingContextError, nil, setting.Name, "FAILOVER_MODE")
					}
					connectData.FailoverModeSettings[setting.Name] = setting.Value
				}
			}
		case "HS":
			connectData.HS = child.Value
		case "RDB_DATABASE":
			connectData.RDBDatabase = child.Value
		case "GLOBAL_NAME":
			connectData.GlobalName = child.Value
		case "CONNECTION_ID_PREFIX":
			connectData.ConnectionIDPrefix = child.Value
		default:
			if len(child.Children) > 0 {
				return nil, nil, common.NewOracleError(oracleErrors.NamingContextError, nil, child.Name, "CONNECT_DATA")
			}
			if connectData.AdditionalSettings == nil {
				connectData.AdditionalSettings = make(map[string]string)
			}
			connectData.AdditionalSettings[child.Name] = child.Value
		}
		set[child.Name] = true
	}
	return connectData, set, nil
}

// parseSecurity converts a SECURITY subtree into typed TLS and wallet values
// while preserving other settings found in the descriptor.
//
// Input:
//   - node: SECURITY node containing TLS, wallet, and additional values.
//
// Output:
//   - *Security: parsed TLS/wallet settings and preserved nested settings.
//   - map[string]bool: upper-case names of settings explicitly present in node.
//   - error: invalid boolean setting or malformed supported structure.
func parseSecurity(node *Node) (*Security, map[string]bool, error) {
	security := &Security{}
	set := make(map[string]bool)
	for i := range node.Children {
		child := &node.Children[i]
		var err error
		if child.Name == "WALLET_LOCATION" && len(child.Children) > 0 {
			if security.StructuredSettings == nil {
				security.StructuredSettings = make(map[string]string)
			}
			security.StructuredSettings[child.Name] = child.ToString()
			if directory := findDescriptorValue(child, "DIRECTORY"); directory != nil {
				security.WalletLocation = directory.Value
			}
			set[child.Name] = true
			continue
		}
		switch child.Name {
		case "SSL_SERVER_CERT_DN":
			security.SSLServerCertDN = child.Value
		case "SSL_SERVER_DN_MATCH":
			security.SSLServerDNMatch, err = parseDescriptorBool(child.Value)
		case "SSL_ALLOW_WEAK_DN_MATCH":
			security.SSLAllowWeakDNMatch, err = parseDescriptorBool(child.Value)
		case "WALLET_LOCATION":
			security.WalletLocation = child.Value
		case "MY_WALLET_DIRECTORY":
			security.MyWalletDirectory = child.Value
		default:
			if len(child.Children) > 0 {
				if security.StructuredSettings == nil {
					security.StructuredSettings = make(map[string]string)
				}
				security.StructuredSettings[child.Name] = child.ToString()
			} else {
				if security.AdditionalSettings == nil {
					security.AdditionalSettings = make(map[string]string)
				}
				security.AdditionalSettings[child.Name] = child.Value
			}
		}
		if err != nil {
			return nil, nil, descriptorValueError(child, err)
		}
		set[child.Name] = true
	}
	return security, set, nil
}

// findDescriptorValue searches descendants depth-first for the first node with
// the requested name.
//
// Input:
//   - node: root of the subtree to search; the node itself is not tested.
//   - name: upper-case descriptor name to find.
//
// Output:
//   - *Node: pointer to the first matching descendant, or nil when none exists.
func findDescriptorValue(node *Node, name string) *Node {
	for i := range node.Children {
		child := &node.Children[i]
		if child.Name == name {
			return child
		}
		if found := findDescriptorValue(child, name); found != nil {
			return found
		}
	}
	return nil
}

// parseBoolPointer parses a descriptor node's boolean value while preserving
// the distinction between an omitted value and an explicit false value.
//
// Input:
//   - node: descriptor node whose scalar value contains the boolean text.
//
// Output:
//   - *bool: pointer to the parsed boolean value.
//   - error: invalid boolean text.
func parseBoolPointer(node *Node) (*bool, error) {
	value, err := parseDescriptorBool(node.Value)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

// parseDescriptorBool converts common TNS boolean spellings to a Go bool.
//
// Input:
//   - value: boolean text such as true/false, yes/no, or on/off.
//
// Output:
//   - bool: parsed value.
//   - error: value is not one of the supported boolean spellings.
func parseDescriptorBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "on":
		return true, nil
	case "false", "no", "off":
		return false, nil
	default:
		return false, common.NewOracleError(oracleErrors.InvalidConnectionParameter, nil, value, "boolean", "true/false, yes/no, or on/off")
	}
}

// parseIntPointer parses a non-negative integer while retaining whether the
// descriptor explicitly supplied it.
//
// Input:
//   - node: descriptor node containing the integer text.
//
// Output:
//   - *int: pointer to the parsed integer.
//   - error: text is not a non-negative decimal integer.
func parseIntPointer(node *Node) (*int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(node.Value))
	if err != nil || value < 0 {
		return nil, common.NewOracleError(oracleErrors.InvalidConnectionParameter, err, node.Value, "integer", "non-negative")
	}
	return &value, nil
}

// parseDurationPointer parses a descriptor duration while retaining whether it
// was explicitly supplied.
//
// Input:
//   - node: descriptor node containing the duration text.
//
// Output:
//   - *time.Duration: pointer to the parsed duration.
//   - error: invalid or negative duration.
func parseDurationPointer(node *Node) (*time.Duration, error) {
	value, err := parseDescriptorDuration(node.Value)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

// parseDescriptorDuration converts Oracle Net duration spellings into a Go duration.
// Bare integers are interpreted as seconds; msec, sec, and min suffixes are also
// accepted alongside Go duration units.
//
// Input:
//   - value: duration text from a descriptor or query setting.
//
// Output:
//   - time.Duration: parsed non-negative duration.
//   - error: invalid or negative duration.
func parseDescriptorDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	for _, suffix := range []struct{ oracle, goSuffix string }{
		{"msec", "ms"},
		{"sec", "s"},
		{"min", "m"},
	} {
		if strings.HasSuffix(lower, suffix.oracle) {
			value = strings.TrimSpace(value[:len(value)-len(suffix.oracle)]) + suffix.goSuffix
			break
		}
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0, common.NewOracleError(oracleErrors.InvalidConnectionParameter, nil, value, "duration", "non-negative")
		}
		return time.Duration(seconds) * time.Second, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < 0 {
		return 0, common.NewOracleError(oracleErrors.InvalidConnectionParameter, err, value, "duration", "non-negative")
	}
	return duration, nil
}

// stringPointer returns a pointer to value for optional descriptor fields.
//
// Input:
//   - value: string to retain as an explicitly supplied option.
//
// Output:
//   - *string: pointer to a copy of value.
func stringPointer(value string) *string { return &value }

// descriptorValueError adds descriptor field context to a parsing error.
//
// Input:
//   - node: descriptor node whose value failed to parse.
//   - err: underlying conversion or validation error.
//
// Output:
//   - error: Oracle naming error containing the node's value, name, and cause.
func descriptorValueError(node *Node, err error) error {
	return common.NewOracleError(oracleErrors.NamingContextError, err, node.Value, node.Name)
}

// handleEasyConnectDescriptorFormatDsn converts EZConnect into the normalized
// description-plan representation.
//
// Input:
//   - dsn: EZConnect target text with host groups and optional service settings.
//
// Output:
//   - ConnectionDescriptionPlan: one normalized description containing the
//     EZConnect targets and connect data.
//   - error: invalid EZConnect host, protocol, or port data.
func handleEasyConnectDescriptorFormatDsn(dsn string) (ConnectionDescriptionPlan, error) {
	accessPoints, serverType, err := parseEazyConnectDescriptor(dsn)
	if err != nil {
		return ConnectionDescriptionPlan{}, err
	}
	description := ConnectionDescription{}
	for _, group := range accessPoints.addressGroups {
		addressGroup := ConnectionAddressGroup{}
		for _, host := range group.hosts {
			port, err := strconv.Atoi(host.port)
			if err != nil {
				return ConnectionDescriptionPlan{}, err
			}
			addressGroup.AccessPoints = append(addressGroup.AccessPoints, RemoteAccessPoint{
				Protocol: accessPoints.protocol,
				Host:     host.host,
				Port:     port,
			})
		}
		description.AddressGroups = append(description.AddressGroups, addressGroup)
	}
	if len(accessPoints.addressGroups) == 1 && len(accessPoints.addressGroups[0].hosts) > 1 {
		value := true
		description.LoadBalance = &value
	}

	connectData := &ConnectData{}
	connectDataSet := make(map[string]bool)
	if accessPoints.serviceName != "" {
		connectData.ServiceName = accessPoints.serviceName
		connectDataSet["SERVICE_NAME"] = true
	}
	if accessPoints.sid != "" {
		connectData.SID = accessPoints.sid
		connectDataSet["SID"] = true
	}
	if accessPoints.instanceName != "" {
		connectData.InstanceName = accessPoints.instanceName
		connectDataSet["INSTANCE_NAME"] = true
	}
	if serverType != "" {
		connectData.Server = serverType
		connectDataSet["SERVER"] = true
	}
	if len(connectDataSet) > 0 {
		description.ConnectData = connectData
		description.ConnectDataSet = connectDataSet
	}
	return ConnectionDescriptionPlan{
		Descriptions: []ConnectionDescription{description},
	}, nil
}
