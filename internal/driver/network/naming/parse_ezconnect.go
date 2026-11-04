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
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	defaultOraclePort = "1521"
	defaultProtocol   = driverCommon.ProtocolTCP
)

// HostPort represents a database host and port combination
type hostPort struct {
	host string
	port string
}

// AddressGroup represents a group of addresses (for ADDRESS_LIST)
type addressGroup struct {
	hosts []hostPort
}

// ParsedURL contains the parsed components of an EZConnect URL
type parsedURL struct {
	protocol      driverCommon.Protocol
	addressGroups []addressGroup
	serviceName   string
	serverMode    string
	instanceName  string
}

// ConversionResult contains the TNS string and any unrecognized parameters
type conversionResult struct {
	tns                string
	unrecognizedParams map[string]string
}

// Parameter aliases map user-friendly names to TNS parameter names
// This support the ability to have different names through this map.
var _parameterAliases = map[string]string{
	"my_wallet_directory": "WALLET_LOCATION",
}

// Parameter destinations map TNS parameter names to their target section
var parameterDestinations = map[string]string{
	// DESCRIPTION-level parameters
	"ENABLE":                    "DESCRIPTION",
	"FAILOVER":                  "DESCRIPTION",
	"LOAD_BALANCE":              "DESCRIPTION",
	"RECV_BUF_SIZE":             "DESCRIPTION",
	"SEND_BUF_SIZE":             "DESCRIPTION",
	"SDU":                       "DESCRIPTION",
	"SOURCE_ROUTE":              "DESCRIPTION",
	"RETRY_COUNT":               "DESCRIPTION",
	"RETRY_DELAY":               "DESCRIPTION",
	"CONNECT_TIMEOUT":           "DESCRIPTION",
	"TRANSPORT_CONNECT_TIMEOUT": "DESCRIPTION",
	"RECV_TIMEOUT":              "DESCRIPTION",
	"USE_SNI":                   "DESCRIPTION",
	"COMPRESSION":               "DESCRIPTION",
	"EXPIRE_TIME":               "DESCRIPTION",

	// ADDRESS-level parameters
	"HTTPS_PROXY":      "ADDRESS",
	"HTTPS_PROXY_PORT": "ADDRESS",

	// CONNECT_DATA parameters
	"POOL_CONNECTION_CLASS": "CONNECT_DATA",
	"POOL_PURITY":           "CONNECT_DATA",
	"SERVICE_TAG":           "CONNECT_DATA",
	"CONNECTION_ID_PREFIX":  "CONNECT_DATA",
	"POOL_BOUNDARY":         "CONNECT_DATA",

	// SECURITY parameters
	"SSL_SERVER_CERT_DN":  "SECURITY",
	"SSL_SERVER_DN_MATCH": "SECURITY",
	"WALLET_LOCATION":     "SECURITY",
}

// Pre-compiled regex for protocol extraction
var protocolPattern = regexp.MustCompile(`^([a-zA-Z0-9]+)://`)

// ParseEzConnect converts an EZConnect URL to long TNS format
// Example: "host:1521/mydb" -> "(DESCRIPTION=(ADDRESS=...)...)"
func parseEzConnect(url string) (*conversionResult, error) {
	// Remove all whitespace
	url = driverCommon.StripSpacesOutsideQuotes(url)

	// Step 1: Split and parse extended parameters (?key=val&key2=val2)
	cleanURL, recognizedParams, unrecognizedParams, err := splitAndParseExtendedParams(url)
	if err != nil {
		return nil, err
	}

	// Step 2: Parse the main EZConnect URL
	parsed, err := parseMainURL(cleanURL)
	if err != nil {
		return nil, err
	}

	// Step 3: Build the TNS format string
	tns := buildTNS(parsed, recognizedParams)

	return &conversionResult{
		tns:                tns,
		unrecognizedParams: unrecognizedParams,
	}, nil
}

// splitAndParseExtendedParams separates the URL from extended parameters
// and categorizes them into recognized and unrecognized parameters
func splitAndParseExtendedParams(url string) (string, map[string]string, map[string]string, error) {
	// Find the '?' that's not inside parentheses
	parenDepth := 0
	paramStart := -1

	for i, ch := range url {
		if ch == '(' {
			parenDepth++
		} else if ch == ')' {
			parenDepth--
		} else if ch == '?' && parenDepth == 0 {
			paramStart = i
			break
		}
	}

	// No extended parameters found
	if paramStart == -1 {
		return url, make(map[string]string), make(map[string]string), nil
	}

	// Parse the parameters
	paramStr := url[paramStart+1:]
	recognizedParams, unrecognizedParams, err := parseExtendedParams(paramStr)
	if err != nil {
		return "", nil, nil, err
	}

	return url[:paramStart], recognizedParams, unrecognizedParams, nil
}

// parseExtendedParams parses the query-string-like parameters
// Format: key1=value1&key2=value2&key3="quoted value"
func parseExtendedParams(paramStr string) (map[string]string, map[string]string, error) {
	recognizedParams := make(map[string]string)
	unrecognizedParams := make(map[string]string)

	if paramStr == "" {
		return recognizedParams, unrecognizedParams, nil
	}

	// Split by '&'
	params := strings.Split(paramStr, "&")

	for _, param := range params {
		param = strings.TrimSpace(param)
		if param == "" {
			continue
		}

		// Split by '=' (key,val)
		parts := strings.SplitN(param, "=", 2)
		if len(parts) != 2 {
			return nil, nil, common.NewOracleError(oracleErrors.NamingEzConnectError, nil, param, "extended-params")
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		// Strip a single pair of wrapping double quotes, but preserve whitespace
		// inside them.
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}

		// Normalize key: check aliases first, then uppercase
		normalizedKey := strings.ToLower(key)
		if aliasedKey, exists := _parameterAliases[normalizedKey]; exists {
			key = aliasedKey
		} else {
			key = strings.ToUpper(key)
		}

		// Categorize into recognized or unrecognized
		if _, isRecognized := parameterDestinations[key]; isRecognized {
			recognizedParams[key] = value
		} else {
			unrecognizedParams[key] = value
		}
	}

	return recognizedParams, unrecognizedParams, nil
}

// parseMainURL parses the core EZConnect format
// Format: [protocol://]host[:port][,host2[:port2]][;host3][/service][:server][/instance]
func parseMainURL(url string) (*parsedURL, error) {
	result := &parsedURL{}

	var err error
	// Extract protocol if present (tcp:// or tcps://)
	if matches := protocolPattern.FindStringSubmatch(url); matches != nil {
		result.protocol, err = driverCommon.NormalizeProtocol(matches[1])
		if err != nil {
			return nil, common.NewOracleError(oracleErrors.NamingEzConnectError, nil, matches[1], "protocol")
		}
		url = protocolPattern.ReplaceAllString(url, "")

	} else {
		result.protocol = defaultProtocol
	}

	// Split by '/' to separate hosts from service/instance
	parts := strings.Split(url, "/")
	hostPart := parts[0]

	// Parse host groups (semicolon separates groups for ADDRESS_LIST)
	addressGroups, err := parseAddressGroups(hostPart)
	if err != nil {
		return nil, err
	}
	result.addressGroups = addressGroups

	// Parse service name, server mode, and instance
	if len(parts) > 1 && parts[1] != "" {
		// Service name is before any ':'
		serviceAndMode := parts[1]
		colonIdx := strings.Index(serviceAndMode, ":")

		if colonIdx != -1 {
			result.serviceName = serviceAndMode[:colonIdx]
			result.serverMode = strings.ToUpper(serviceAndMode[colonIdx+1:])
		} else {
			result.serviceName = serviceAndMode
		}
	}

	// Parse instance name (if present in parts[2])
	if len(parts) > 2 && parts[2] != "" {
		result.instanceName = parts[2]
	}

	return result, nil
}

// parseAddressGroups parses semicolon-separated address groups
// Each group contains comma-separated hosts
func parseAddressGroups(hostStr string) ([]addressGroup, error) {
	if hostStr == "" {
		return nil, common.NewOracleError(oracleErrors.NamingEzConnectError, nil, "", "host")
	}

	// Split by semicolon to get address groups
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

// parseHostList parses comma-separated host:port combinations
// Supports IPv6 addresses in brackets: [::1]:1521
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

			// Validate port number
			portNum, err := strconv.Atoi(port)
			if err != nil {
				return nil, common.NewOracleError(oracleErrors.NamingEzConnectError, err, port, "port")
			}
			if portNum < 1 || portNum > 65535 {
				return nil, common.NewOracleError(oracleErrors.NamingEzConnectError, nil, port, "port")
			}

			hosts = append(hosts, hostPort{host: host, port: port})

			currentHost.Reset()
			currentPort.Reset()
			parsingPort = false
		} else {
			if parsingPort {
				currentPort.WriteByte(ch)
			} else {
				currentHost.WriteByte(ch)
			}
		}
	}

	// Add the last host
	host := strings.TrimSpace(currentHost.String())
	port := strings.TrimSpace(currentPort.String())

	if host == "" {
		return nil, common.NewOracleError(oracleErrors.NamingEzConnectError, nil, "", "host")
	}

	if port == "" {
		port = defaultOraclePort
	}

	// Validate port number
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return nil, common.NewOracleError(oracleErrors.NamingEzConnectError, err, port, "port")
	}
	if portNum < 1 || portNum > 65535 {
		return nil, common.NewOracleError(oracleErrors.NamingEzConnectError, nil, port, "port")
	}

	hosts = append(hosts, hostPort{host: host, port: port})

	return hosts, nil
}

// buildTNS constructs the final TNS format string
func buildTNS(parsed *parsedURL, params map[string]string) string {
	var tns strings.Builder

	tns.WriteString("(DESCRIPTION=")

	// Add DESCRIPTION-level parameters
	tns.WriteString(buildDescriptionParams(parsed, params))

	// Add ADDRESS or ADDRESS_LIST nodes
	tns.WriteString(buildAddressList(parsed.addressGroups, parsed.protocol, params))

	// Add CONNECT_DATA
	tns.WriteString(buildConnectData(parsed.serviceName, parsed.serverMode, parsed.instanceName, params))

	// Add SECURITY (for TCPS)
	security := buildSecurity(params)
	if security != "" {
		tns.WriteString(security)
	}

	tns.WriteString(")")

	return tns.String()
}

// buildDescriptionParams adds DESCRIPTION-level parameters
func buildDescriptionParams(parsed *parsedURL, params map[string]string) string {
	var result strings.Builder

	// Auto-enable LOAD_BALANCE if multiple hosts in a single group and not explicitly set
	if _, explicitlySet := params["LOAD_BALANCE"]; !explicitlySet {
		if len(parsed.addressGroups) == 1 && len(parsed.addressGroups[0].hosts) > 1 {
			result.WriteString("(LOAD_BALANCE=ON)")
		}
	}

	// Add all DESCRIPTION-level parameters from params
	for key, value := range params {
		if parameterDestinations[key] == "DESCRIPTION" {
			result.WriteString(fmt.Sprintf("(%s=%s)", key, value))
		}
	}

	return result.String()
}

// buildAddressList creates ADDRESS or ADDRESS_LIST sections
func buildAddressList(groups []addressGroup, protocol driverCommon.Protocol, params map[string]string) string {
	var result strings.Builder

	// Get proxy info if provided
	proxyHost := params["HTTPS_PROXY"]
	proxyPort := params["HTTPS_PROXY_PORT"]
	proxyInfo := ""
	if proxyHost != "" {
		if proxyPort != "" {
			proxyInfo = fmt.Sprintf("(HTTPS_PROXY=%s)(HTTPS_PROXY_PORT=%s)", proxyHost, proxyPort)
		} else {
			proxyInfo = fmt.Sprintf("(HTTPS_PROXY=%s)", proxyHost)
		}
	}

	// If multiple groups (separated by semicolon), create ADDRESS_LIST for each
	if len(groups) > 1 {
		for _, group := range groups {
			var addressList strings.Builder
			addressList.WriteString("(ADDRESS_LIST=")

			// Add load balance for this group if it has multiple hosts
			if len(group.hosts) > 1 {
				addressList.WriteString("(LOAD_BALANCE=ON)")
			}

			// Add all addresses in this group
			for _, hp := range group.hosts {
				addressList.WriteString(buildAddress(hp, protocol, proxyInfo))
			}

			addressList.WriteString(")")
			result.WriteString(addressList.String())
		}
	} else {
		// Single group - just add ADDRESS nodes (load balance handled at DESCRIPTION level)
		for _, hp := range groups[0].hosts {
			result.WriteString(buildAddress(hp, protocol, proxyInfo))
		}
	}

	return result.String()
}

// buildAddress creates a single ADDRESS node
func buildAddress(hp hostPort, protocol driverCommon.Protocol, proxyInfo string) string {
	host := hp.host

	// Remove brackets from IPv6 addresses for TNS format
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}

	return fmt.Sprintf("(ADDRESS=(PROTOCOL=%s)(HOST=%s)(PORT=%s)%s)",
		protocol, host, hp.port, proxyInfo)
}

// buildConnectData creates the CONNECT_DATA section
func buildConnectData(serviceName, serverMode, instanceName string, params map[string]string) string {
	var connectData strings.Builder

	connectData.WriteString("(CONNECT_DATA=")

	// Service name (always included, even if empty)
	if serviceName != "" {
		connectData.WriteString(fmt.Sprintf("(SERVICE_NAME=%s)", serviceName))
	} else {
		connectData.WriteString("(SERVICE_NAME=)")
	}

	// Server mode (DEDICATED, SHARED, or POOLED)
	if serverMode != "" {
		connectData.WriteString(fmt.Sprintf("(SERVER=%s)", serverMode))
	}

	// Instance name
	if instanceName != "" {
		connectData.WriteString(fmt.Sprintf("(INSTANCE_NAME=%s)", instanceName))
	}

	// Add all CONNECT_DATA-level parameters from params
	for key, value := range params {
		if parameterDestinations[key] == "CONNECT_DATA" {
			connectData.WriteString(fmt.Sprintf("(%s=%s)", key, value))
		}
	}

	connectData.WriteString(")")

	return connectData.String()
}

// buildSecurity creates the SECURITY section (for TCPS connections)
func buildSecurity(params map[string]string) string {
	var security strings.Builder

	// Add all SECURITY-level parameters from params
	for key, value := range params {
		if parameterDestinations[key] == "SECURITY" {
			security.WriteString(fmt.Sprintf("(%s=%s)", key, value))
		}
	}

	if security.Len() == 0 {
		return ""
	}

	return fmt.Sprintf("(SECURITY=%s)", security.String())
}
