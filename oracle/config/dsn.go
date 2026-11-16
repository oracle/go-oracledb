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
	Name     string
	Value    string
	Children []Node
}

// ParseConnectorDescriptor parses a TNS connection string into a name-value tree.
func ParseConnectorDescriptor(connectString string) (*Node, error) {

	tokens := tokenize(connectString)
	if len(tokens) == 0 {
		return nil, common.NewOracleError(oracleErrors.NamingTokensMissing, nil)
	}

	return parseIterative(tokens)
}

// tokenize splits a TNS string into parentheses and name-value tokens while
// retaining whitespace inside quoted values and unquoted values after '='.
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

// GetValue retrieves a leaf value by a slash-separated, case-insensitive path.
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

// GetNode retrieves a node by a slash-separated, case-insensitive path.
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

func (n *Node) ChildCount() int {
	return len(n.Children)
}

func (n *Node) GetChild(index int) (*Node, error) {
	if index < 0 || index >= len(n.Children) {
		return nil, common.NewOracleError(oracleErrors.NamingParseBounds, nil, index, len(n.Children))
	}
	return &n.Children[index], nil
}

// ToString serializes the node tree in TNS parenthetical form.
func (n *Node) ToString() string {
	var result strings.Builder
	result.WriteString("(")
	result.WriteString(n.Name)
	if n.Value != "" || len(n.Children) > 0 {
		result.WriteString("=")
	}
	if n.Value != "" {
		result.WriteString(n.Value)
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

// ConnectionPlans contains the base configuration, query-string settings, and
// the normalized description-list plan produced from a DSN.
type ConnectionPlans struct {
	BaseConfig    *OracleDriverConfig
	QuerySettings map[string]string
	Plan          ConnectionDescriptionPlan
}

// ConnectionDescriptionPlan contains the settings and alternatives found at
// the DESCRIPTION_LIST level. A single DESCRIPTION or EZConnect DSN is
// represented as a plan with one Description.
type ConnectionDescriptionPlan struct {
	Failover           *bool
	LoadBalance        *bool
	SourceRoute        *bool
	AddressGroups      []ConnectionAddressGroup
	Descriptions       []ConnectionDescription
	AdditionalSettings map[string]string
}

// ConnectionDescription contains the settings scoped to one DESCRIPTION.
// Pointer fields preserve the distinction between an omitted setting and an
// explicitly supplied zero value.
type ConnectionDescription struct {
	Failover                *bool
	LoadBalance             *bool
	SourceRoute             *bool
	UseSNI                  *bool
	ConnectTimeout          *time.Duration
	RetryCount              *int
	RetryDelay              *time.Duration
	TransportConnectTimeout *time.Duration
	ExpireTime              *int
	RecvTimeout             *time.Duration
	SendTimeout             *time.Duration
	SDU                     *int
	RecvBufSize             *int
	SendBufSize             *int
	Enable                  *string
	ConnectionIDPrefix      *string
	Compression             *bool
	CompressionLevels       []string
	TypeOfService           *string
	Security                *Security
	SecuritySet             map[string]bool
	ConnectData             *ConnectData
	ConnectDataSet          map[string]bool
	AddressGroups           []ConnectionAddressGroup
	AdditionalSettings      map[string]string
}

// ConnectionAddressGroup represents either an ADDRESS_LIST or the implicit
// group formed by direct ADDRESS children of a DESCRIPTION.
type ConnectionAddressGroup struct {
	Failover           *bool
	LoadBalance        *bool
	SourceRoute        *bool
	AccessPoints       []RemoteAccessPoint
	AdditionalSettings map[string]string
}

// RemoteAccessPoint is one protocol endpoint from an ADDRESS node.
type RemoteAccessPoint struct {
	Protocol           driverCommon.Protocol
	Host               string
	Port               int
	HTTPSProxy         *string
	HTTPSProxyPort     *int
	RecvBufSize        *int
	SendBufSize        *int
	AdditionalSettings map[string]string
}

// ConnectData contains database identification and connection parameters from
// the CONNECT_DATA section of a descriptor.
type ConnectData struct {
	ServiceName        string
	SID                string
	Server             string
	PoolName           string // DRCP pool selected with (POOL_NAME=...).
	InstanceName       string
	FailoverMode       string
	HS                 string
	RDBDatabase        string
	GlobalName         string
	ConnectionIDPrefix string
}

// Security contains TLS and wallet parameters from a SECURITY section.
type Security struct {
	SSLServerCertDN     string
	SSLServerDNMatch    bool
	SSLAllowWeakDNMatch bool
	WalletLocation      string
	MyWalletDirectory   string
}

var protocolPattern = regexp.MustCompile(`^([a-zA-Z0-9]+)://`)

// parseEazyConnectDescriptor parses the core EZConnect format.
// Format: [protocol://]host[:port][,host2[:port2]][;host3][/service][:server][/instance]
// returns:
//   - access points informations
//   - server type (DEDICATED/SHARED/POOLED) , may be empty
//   - error : oif parsing has failed
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

func _isConnectDescriptorFormat(dsn string) bool {
	return strings.HasPrefix(dsn, "(")
}

func HandleDsn(dsn string, drvConfig *OracleDriverConfig) (*ConnectionPlans, error) {
	cleanDSN := driverCommon.StripSpacesOutsideQuotes(dsn)
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
		if drvConfig.Credentials.User != "" || drvConfig.Credentials.Password != "" {
			return nil, common.NewOracleError(oracleErrors.ConflictingConnectionParameterSource, nil, "credentials")
		}
		drvConfig.Credentials.User = credentials[0]
		drvConfig.Credentials.Password = credentials[1]
		effectiveDSN = parts[1]
	}

	// Preserve query settings separately so they can be applied to the
	// configuration for the selected description later.
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
	return &ConnectionPlans{
		BaseConfig:    drvConfig,
		QuerySettings: querySettings,
		Plan:          plan,
	}, nil
}

// handleConnectDescriptorFormatDsn parses a TNS descriptor into the normalized
// connection-description plan.
func handleConnectDescriptorFormatDsn(dsn string) (ConnectionDescriptionPlan, error) {
	root, err := ParseConnectorDescriptor(dsn)
	if err != nil {
		return ConnectionDescriptionPlan{}, err
	}

	plan := ConnectionDescriptionPlan{}
	switch root.Name {
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
		if len(description.AddressGroups) == 0 && len(plan.AddressGroups) == 0 {
			return ConnectionDescriptionPlan{}, common.NewOracleError(oracleErrors.NamingContextError, nil, "DESCRIPTION", "ADDRESS or ADDRESS_LIST")
		}
	}
	return plan, nil
}

// parseConnectionDescription converts a DESCRIPTION subtree into typed fields.
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

func parseConnectData(node *Node) (*ConnectData, map[string]bool, error) {
	connectData := &ConnectData{}
	set := make(map[string]bool)
	for i := range node.Children {
		child := &node.Children[i]
		switch child.Name {
		case "SERVICE_NAME":
			connectData.ServiceName = child.Value
		case "SID":
			connectData.SID = child.Value
		case "SERVER":
			connectData.Server = child.Value
		case "POOL_NAME":
			connectData.PoolName = child.Value
		case "INSTANCE_NAME":
			connectData.InstanceName = child.Value
		case "FAILOVER_MODE":
			connectData.FailoverMode = child.Value
		case "HS":
			connectData.HS = child.Value
		case "RDB_DATABASE":
			connectData.RDBDatabase = child.Value
		case "GLOBAL_NAME":
			connectData.GlobalName = child.Value
		case "CONNECTION_ID_PREFIX":
			connectData.ConnectionIDPrefix = child.Value
		default:
			return nil, nil, common.NewOracleError(oracleErrors.NamingContextError, nil, child.Name, "CONNECT_DATA")
		}
		set[child.Name] = true
	}
	return connectData, set, nil
}

func parseSecurity(node *Node) (*Security, map[string]bool, error) {
	security := &Security{}
	set := make(map[string]bool)
	for i := range node.Children {
		child := &node.Children[i]
		var err error
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
			return nil, nil, common.NewOracleError(oracleErrors.NamingContextError, nil, child.Name, "SECURITY")
		}
		if err != nil {
			return nil, nil, descriptorValueError(child, err)
		}
		set[child.Name] = true
	}
	return security, set, nil
}

func parseBoolPointer(node *Node) (*bool, error) {
	value, err := parseDescriptorBool(node.Value)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

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

func parseIntPointer(node *Node) (*int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(node.Value))
	if err != nil || value < 0 {
		return nil, common.NewOracleError(oracleErrors.InvalidConnectionParameter, err, node.Value, "integer", "non-negative")
	}
	return &value, nil
}

func parseDurationPointer(node *Node) (*time.Duration, error) {
	value, err := parseDescriptorDuration(node.Value)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

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

func stringPointer(value string) *string { return &value }

func descriptorValueError(node *Node, err error) error {
	return common.NewOracleError(oracleErrors.NamingContextError, err, node.Value, node.Name)
}

// handleEasyConnectDescriptorFormatDsn converts EZConnect into the normalized
// description-plan representation.
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
