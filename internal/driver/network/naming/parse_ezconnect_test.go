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
	"strings"
	"testing"

	common "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestParseEzConnect_SimpleHostAndService tests basic host:port/service format
func TestParseEzConnect_SimpleHostAndService(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect("localhost:1521/mydb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convResult.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", convResult.unrecognizedParams)
	}

	expected := "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=localhost)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=mydb)))"
	if convResult.tns != expected {
		t.Errorf("expected %s, got %s", expected, convResult.tns)
	}
}

func TestParseEzConnect_DefaultPort(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect("myhost/orcl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convResult.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", convResult.unrecognizedParams)
	}

	expected := "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=myhost)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=orcl)))"
	if convResult.tns != expected {
		t.Errorf("expected %s, got %s", expected, convResult.tns)
	}
}

// TestParseEzConnect_MultipleHosts tests comma-separated hosts
func TestParseEzConnect_MultipleHosts(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect("host1:1521,host2:1522/mydb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convResult.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", convResult.unrecognizedParams)
	}
	result := convResult.tns

	if !strings.Contains(result, "(LOAD_BALANCE=ON)") {
		t.Errorf("expected LOAD_BALANCE=ON for multiple hosts, got %s", result)
	}
	if !strings.Contains(result, "(HOST=host1)") || !strings.Contains(result, "(HOST=host2)") {
		t.Errorf("expected both hosts in output, got %s", result)
	}
}

// TestParseEzConnect_MultipleAddressGroups tests semicolon-separated groups
func TestParseEzConnect_MultipleAddressGroups(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect("host1:1521;host2:1522/mydb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convResult.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", convResult.unrecognizedParams)
	}
	result := convResult.tns

	if !strings.Contains(result, "(ADDRESS_LIST=") {
		t.Errorf("expected ADDRESS_LIST for multiple groups, got %s", result)
	}
	if strings.Count(result, "(ADDRESS_LIST=") != 2 {
		t.Errorf("expected 2 ADDRESS_LIST entries, got %s", result)
	}
}

// server mode and instance name test
func TestParseEzConnect_ServerModeAndInstance(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect("host:1521/mydb:dedicated/inst1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convResult.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", convResult.unrecognizedParams)
	}
	result := convResult.tns

	if !strings.Contains(result, "(SERVER=DEDICATED)") {
		t.Errorf("expected SERVER=DEDICATED, got %s", result)
	}
	if !strings.Contains(result, "(INSTANCE_NAME=inst1)") {
		t.Errorf("expected INSTANCE_NAME=inst1, got %s", result)
	}
}

// TestParseEzConnect_ExtendedParams tests URL parameters
func TestParseEzConnect_ExtendedParams(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect("host:1521/mydb?connect_timeout=10&retry_count=3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convResult.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", convResult.unrecognizedParams)
	}
	result := convResult.tns

	if !strings.Contains(result, "(CONNECT_TIMEOUT=10)") {
		t.Errorf("expected CONNECT_TIMEOUT=10, got %s", result)
	}
	if !strings.Contains(result, "(RETRY_COUNT=3)") {
		t.Errorf("expected RETRY_COUNT=3, got %s", result)
	}
}

// TestParseEzConnect_QuotedParamValue tests quoted parameter values
func TestParseEzConnect_QuotedParamValue(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect(`tcps://host:1521/mydb?wallet_location="C:\my_wallet"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convResult.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", convResult.unrecognizedParams)
	}
	result := convResult.tns
	if !strings.Contains(result, `(WALLET_LOCATION=C:\my_wallet)`) {
		t.Errorf("expected wrapping quotes to be removed, got %s", result)
	}
}

// TestParseEzConnect_ParameterAliases tests parameter name normalization
func TestParseEzConnect_ParameterAliases(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect("tcps://host:1521/mydb?my_wallet_directory=/path/to/wallet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convResult.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", convResult.unrecognizedParams)
	}
	result := convResult.tns

	if !strings.Contains(result, "(WALLET_LOCATION=/path/to/wallet)") {
		t.Errorf("expected alias to be converted to WALLET_LOCATION, got %s", result)
	}
}

// TestParseEzConnect_HTTPSProxy tests proxy configuration
func TestParseEzConnect_HTTPSProxy(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect("host:1521/mydb?https_proxy=proxy.corp.com&https_proxy_port=8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convResult.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", convResult.unrecognizedParams)
	}
	result := convResult.tns

	if !strings.Contains(result, "(HTTPS_PROXY=proxy.corp.com)") {
		t.Errorf("expected HTTPS_PROXY in address, got %s", result)
	}
	if !strings.Contains(result, "(HTTPS_PROXY_PORT=8080)") {
		t.Errorf("expected HTTPS_PROXY_PORT in address, got %s", result)
	}
}

// TestParseEzConnect_IPv6Address tests IPv6 host parsing
func TestParseEzConnect_IPv6Address(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect("[::1]:1521/mydb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convResult.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", convResult.unrecognizedParams)
	}
	result := convResult.tns

	if !strings.Contains(result, "(HOST=::1)") {
		t.Errorf("expected IPv6 address without brackets in TNS, got %s", result)
	}
}

// TestParseEzConnect_IPv6WithMultipleHosts tests multiple IPv6 addresses
func TestParseEzConnect_IPv6WithMultipleHosts(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect("[::1]:1521,[2001:db8::1]:1522/mydb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convResult.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", convResult.unrecognizedParams)
	}
	result := convResult.tns

	if !strings.Contains(result, "(HOST=::1)") || !strings.Contains(result, "(HOST=2001:db8::1)") {
		t.Errorf("expected both IPv6 addresses, got %s", result)
	}
}

// empty service name tests
func TestParseEzConnect_EmptyServiceVariants(t *testing.T) {
	t.Parallel()
	inputs := []string{"host:1521/", "host:1521"}
	for _, in := range inputs {
		convResult, err := parseEzConnect(in)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", in, err)
		}
		if len(convResult.unrecognizedParams) != 0 {
			t.Errorf("unexpected unrecognized params for %q: %v", in, convResult.unrecognizedParams)
		}
		expected := "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=host)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=)))"
		if convResult.tns != expected {
			t.Errorf("expected %s for %q, got %s", expected, in, convResult.tns)
		}
	}
}

// TestParseEzConnect_WhitespaceRemoval tests whitespace handling
func TestParseEzConnect_WhitespaceRemoval(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect("host : 1521 / mydb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convResult.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", convResult.unrecognizedParams)
	}

	expected := "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=host)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=mydb)))"
	if convResult.tns != expected {
		t.Errorf("expected %s, got %s", expected, convResult.tns)
	}
}

func TestParseEzConnect_Whitespace_Removal_Preserve(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect(`host : 1521 / mydb?wallet_location="C:\my_wallet with space"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convResult.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", convResult.unrecognizedParams)
	}

	expected := `(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=host)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=mydb))(SECURITY=(WALLET_LOCATION=C:\my_wallet with space)))`
	if convResult.tns != expected {
		t.Errorf("expected %s, got %s", expected, convResult.tns)
	}
}

// TestParseEzConnect_EmptyURL tests error on empty URL
func TestParseEzConnect_EmptyURL(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect("")
	if err == nil {
		t.Error("expected error for empty URL")
	}
	if convResult != nil {
		t.Error("expected nil result on error")
	}
	assertErrorCode(t, err, oracleErrors.NamingEzConnectError)
}

// TestParseEzConnect_SecurityParams tests TCPS with security parameters
func TestParseEzConnect_SecurityParams(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect("tcps://host:2484/mydb?ssl_server_dn_match=yes&wallet_location=/wallet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convResult.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", convResult.unrecognizedParams)
	}
	result := convResult.tns

	if !strings.Contains(result, "(SECURITY=") {
		t.Errorf("expected SECURITY section for TCPS, got %s", result)
	}
	if !strings.Contains(result, "(SSL_SERVER_DN_MATCH=yes)") {
		t.Errorf("expected SSL_SERVER_DN_MATCH in SECURITY, got %s", result)
	}
}

// TestParseEzConnect_ConnectionPoolParams tests connection pool parameters
func TestParseEzConnect_ConnectionPoolParams(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect("host:1521/mydb?pool_connection_class=MYAPP&pool_purity=SELF")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convResult.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", convResult.unrecognizedParams)
	}
	result := convResult.tns

	if !strings.Contains(result, "(POOL_CONNECTION_CLASS=MYAPP)") {
		t.Errorf("expected POOL_CONNECTION_CLASS in CONNECT_DATA, got %s", result)
	}
	if !strings.Contains(result, "(POOL_PURITY=SELF)") {
		t.Errorf("expected POOL_PURITY in CONNECT_DATA, got %s", result)
	}
}

// TestParseEzConnect_ComplexMultiHostScenario tests real-world complex URL
func TestParseEzConnect_ComplexMultiHostScenario(t *testing.T) {
	t.Parallel()
	url := "tcps://host1:2484,host2:2484;host3:2484/proddb:dedicated?retry_count=3&connect_timeout=10"
	convResult, err := parseEzConnect(url)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convResult.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", convResult.unrecognizedParams)
	}
	result := convResult.tns

	// Verify key components
	if !strings.Contains(result, "(PROTOCOL=TCPS)") {
		t.Errorf("expected TCPS protocol")
	}
	if !strings.Contains(result, "(ADDRESS_LIST=") {
		t.Errorf("expected ADDRESS_LIST for semicolon groups")
	}
	if !strings.Contains(result, "(SERVICE_NAME=proddb)") {
		t.Errorf("expected service name proddb")
	}
	if !strings.Contains(result, "(SERVER=DEDICATED)") {
		t.Errorf("expected DEDICATED server mode")
	}
	if !strings.Contains(result, "(RETRY_COUNT=3)") {
		t.Errorf("expected RETRY_COUNT parameter")
	}
}

// TestParseExtendedParams_InvalidFormat tests parameter parsing error
func TestParseExtendedParams_InvalidFormat(t *testing.T) {
	t.Parallel()
	_, _, err := parseExtendedParams("invalidparam")
	if err == nil {
		t.Error("expected error for invalid parameter format")
	}
	assertErrorCode(t, err, oracleErrors.NamingEzConnectError)
}

// TestParseExtendedParams_EmptyString tests empty parameter string
func TestParseExtendedParams_EmptyString(t *testing.T) {
	t.Parallel()
	recognized, unrecognized, err := parseExtendedParams("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recognized) != 0 || len(unrecognized) != 0 {
		t.Errorf("expected empty maps for empty string")
	}
}

// TestParseExtendedParams_MultipleParams tests multiple parameters
func TestParseExtendedParams_MultipleParams(t *testing.T) {
	t.Parallel()
	recognized, unrecognized, err := parseExtendedParams("key1=val1&key2=val2&key3=val3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recognized) != 0 {
		t.Errorf("expected 0 recognized params, got %d", len(recognized))
	}
	if len(unrecognized) != 3 {
		t.Errorf("expected 3 unrecognized params, got %d", len(unrecognized))
	}
	if unrecognized["KEY1"] != "val1" || unrecognized["KEY2"] != "val2" || unrecognized["KEY3"] != "val3" {
		t.Errorf("unrecognized parameter values incorrect: %v", unrecognized)
	}
}

// TestParseEzConnect_InvalidExtendedParams tests invalid parameter format in full ParseEzConnect
func TestParseEzConnect_InvalidExtendedParams(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect("host:1521/mydb?invalidparam")
	if err == nil {
		t.Error("expected error for invalid extended parameter format")
	}
	if !strings.Contains(err.Error(), "invalidparam") || !strings.Contains(err.Error(), "extended-params") {
		t.Errorf("expected error mentioning value 'invalidparam' and field 'extended-params', got: %v", err)
	}
	if convResult != nil {
		t.Error("expected nil result on error")
	}
	assertErrorCode(t, err, oracleErrors.NamingEzConnectError)
}

// TestBuildAddress_TCPWithoutProxy tests basic address building
func TestBuildAddress_TCPWithoutProxy(t *testing.T) {
	t.Parallel()
	hp := hostPort{host: "myhost", port: "1521"}
	result := buildAddress(hp, common.ProtocolTCP, "")
	expected := "(ADDRESS=(PROTOCOL=TCP)(HOST=myhost)(PORT=1521))"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

// TestBuildAddress_TCPSWithProxy tests address with proxy
func TestBuildAddress_TCPSWithProxy(t *testing.T) {
	t.Parallel()
	hp := hostPort{host: "myhost", port: "2484"}
	proxyInfo := "(HTTPS_PROXY=proxy.com)(HTTPS_PROXY_PORT=8080)"
	result := buildAddress(hp, common.ProtocolTCPS, proxyInfo)
	expected := "(ADDRESS=(PROTOCOL=TCPS)(HOST=myhost)(PORT=2484)(HTTPS_PROXY=proxy.com)(HTTPS_PROXY_PORT=8080))"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

// TestBuildAddress_IPv6 tests IPv6 bracket removal
func TestBuildAddress_IPv6(t *testing.T) {
	t.Parallel()
	hp := hostPort{host: "[::1]", port: "1521"}
	result := buildAddress(hp, common.ProtocolTCP, "")
	expected := "(ADDRESS=(PROTOCOL=TCP)(HOST=::1)(PORT=1521))"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

// TestBuildConnectData_AllParams tests connect data with all parameters
func TestBuildConnectData_AllParams(t *testing.T) {
	t.Parallel()
	urlProps := map[string]string{
		"POOL_CONNECTION_CLASS": "MYCLASS",
		"POOL_PURITY":           "SELF",
		"SERVICE_TAG":           "TAG1",
		"CONNECTION_ID_PREFIX":  "PREFIX",
		"POOL_BOUNDARY":         "STATEMENT",
	}

	result := buildConnectData("mydb", "DEDICATED", "inst1", urlProps)

	if !strings.Contains(result, "(SERVICE_NAME=mydb)") {
		t.Errorf("expected SERVICE_NAME in %s", result)
	}
	if !strings.Contains(result, "(SERVER=DEDICATED)") {
		t.Errorf("expected SERVER in %s", result)
	}
	if !strings.Contains(result, "(INSTANCE_NAME=inst1)") {
		t.Errorf("expected INSTANCE_NAME in %s", result)
	}
	if !strings.Contains(result, "(POOL_CONNECTION_CLASS=MYCLASS)") {
		t.Errorf("expected POOL_CONNECTION_CLASS in %s", result)
	}
}

// TestBuildConnectData_EmptyService tests empty service name
func TestBuildConnectData_EmptyService(t *testing.T) {
	t.Parallel()
	result := buildConnectData("", "", "", make(map[string]string))
	expected := "(CONNECT_DATA=(SERVICE_NAME=))"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

// TestBuildSecurity_AllParams tests security section building
func TestBuildSecurity_AllParams(t *testing.T) {
	t.Parallel()
	urlProps := map[string]string{
		"SSL_SERVER_DN_MATCH": "yes",
		"SSL_SERVER_CERT_DN":  "CN=server",
		"WALLET_LOCATION":     "/wallet",
	}

	result := buildSecurity(urlProps)

	if !strings.Contains(result, "(SECURITY=") {
		t.Errorf("expected SECURITY section in %s", result)
	}
	if !strings.Contains(result, "(SSL_SERVER_DN_MATCH=yes)") {
		t.Errorf("expected SSL_SERVER_DN_MATCH in %s", result)
	}
}

// TestBuildSecurity_Empty tests empty security section
func TestBuildSecurity_Empty(t *testing.T) {
	t.Parallel()
	result := buildSecurity(make(map[string]string))
	if result != "" {
		t.Errorf("expected empty string for no security params, got %s", result)
	}
}

// TestBuildDescriptionParams_AutoLoadBalance tests automatic load balance
func TestBuildDescriptionParams_AutoLoadBalance(t *testing.T) {
	t.Parallel()
	parsed := &parsedURL{
		addressGroups: []addressGroup{
			{hosts: []hostPort{{host: "h1", port: "1521"}, {host: "h2", port: "1521"}}},
		},
	}

	result := buildDescriptionParams(parsed, make(map[string]string))

	if !strings.Contains(result, "(LOAD_BALANCE=ON)") {
		t.Errorf("expected auto LOAD_BALANCE for multiple hosts in single group, got %s", result)
	}
}

// TestBuildDescriptionParams_ExplicitLoadBalance tests explicit override
func TestBuildDescriptionParams_ExplicitLoadBalance(t *testing.T) {
	t.Parallel()
	parsed := &parsedURL{
		addressGroups: []addressGroup{
			{hosts: []hostPort{{host: "h1", port: "1521"}, {host: "h2", port: "1521"}}},
		},
	}
	urlProps := map[string]string{"LOAD_BALANCE": "OFF"}

	result := buildDescriptionParams(parsed, urlProps)

	// Should contain the explicit OFF, not auto ON
	if !strings.Contains(result, "(LOAD_BALANCE=OFF)") {
		t.Errorf("expected explicit LOAD_BALANCE=OFF, got %s", result)
	}
}

// TestBuildAddressList_SingleGroup tests single group address list
func TestBuildAddressList_SingleGroup(t *testing.T) {
	t.Parallel()
	groups := []addressGroup{
		{hosts: []hostPort{{host: "h1", port: "1521"}, {host: "h2", port: "1522"}}},
	}

	result := buildAddressList(groups, common.ProtocolTCP, make(map[string]string))
	expected := "(ADDRESS=(PROTOCOL=TCP)(HOST=h1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=h2)(PORT=1522))"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

// TestBuildAddressList_MultipleGroups tests multiple groups
func TestBuildAddressList_MultipleGroups(t *testing.T) {
	t.Parallel()
	groups := []addressGroup{
		{hosts: []hostPort{{host: "h1", port: "1521"}}},
		{hosts: []hostPort{{host: "h2", port: "1522"}}},
	}

	result := buildAddressList(groups, common.ProtocolTCP, make(map[string]string))

	if strings.Count(result, "(ADDRESS_LIST=") != 2 {
		t.Errorf("expected 2 ADDRESS_LIST entries, got %s", result)
	}
}

// TestSplitAndParseExtendedParams_WithParams tests URL with parameters
func TestSplitAndParseExtendedParams_WithParams(t *testing.T) {
	t.Parallel()
	cleanURL, recognized, unrecognized, err := splitAndParseExtendedParams("host:1521/db?key=val")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cleanURL != "host:1521/db" {
		t.Errorf("expected URL without params, got %s", cleanURL)
	}
	if len(recognized) != 0 {
		t.Errorf("expected 0 recognized, got %d", len(recognized))
	}
	if len(unrecognized) != 1 || unrecognized["KEY"] != "val" {
		t.Errorf("expected key=val in unrecognized, got %v", unrecognized)
	}
}

// TestParseEzConnect_HTTPSProxyWithoutPort covers HTTPS proxy without specifying HTTPS_PROXY_PORT
func TestParseEzConnect_HTTPSProxyWithoutPort(t *testing.T) {
	t.Parallel()
	convResult, err := parseEzConnect("host:1521/mydb?https_proxy=proxy.only")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(convResult.tns, "(HTTPS_PROXY=proxy.only)") {
		t.Errorf("expected HTTPS_PROXY without port, got %s", convResult.tns)
	}
	if strings.Contains(convResult.tns, "(HTTPS_PROXY_PORT=") {
		t.Errorf("did not expect HTTPS_PROXY_PORT, got %s", convResult.tns)
	}
}

// TestParseEzConnect_HTTPSProxyPortBounds verifies that proxy port zero is
// accepted as unspecified/default, while values above the TCP port limit fail
// during parsing.
func TestParseEzConnect_HTTPSProxyPortBounds(t *testing.T) {
	t.Parallel()

	for _, port := range []string{"0", "65535"} {
		convResult, err := parseEzConnect("host:1521/mydb?https_proxy=proxy.example.com&https_proxy_port=" + port)
		if err == nil {
			var root *Node
			root, err = Parse(convResult.tns)
			if err == nil {
				_, err = ExtractConnectionContext(root)
			}
		}
		if err != nil {
			t.Errorf("expected proxy port %s to be accepted, got %v", port, err)
		}
	}

	for _, port := range []string{"-1", "65536"} {
		convResult, err := parseEzConnect("host:1521/mydb?https_proxy=proxy.example.com&https_proxy_port=" + port)
		if err == nil {
			var root *Node
			root, err = Parse(convResult.tns)
			if err == nil {
				_, err = ExtractConnectionContext(root)
			}
		}
		if err == nil {
			t.Errorf("expected proxy port %s to be rejected", port)
		}
	}
}

// TestSplitAndParseExtendedParams_ParensBeforeParams ensures '?' inside parentheses is ignored
func TestSplitAndParseExtendedParams_ParensBeforeParams(t *testing.T) {
	t.Parallel()
	clean, recognized, unrecognized, err := splitAndParseExtendedParams("(FOO=BAR?BAZ)?a=b&https_proxy=proxy.corp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clean != "(FOO=BAR?BAZ)" {
		t.Errorf("expected clean URL '(FOO=BAR?BAZ)', got %s", clean)
	}
	if unrecognized["A"] != "b" {
		t.Errorf("expected unrecognized A=b, got %v", unrecognized)
	}
	if recognized["HTTPS_PROXY"] != "proxy.corp" {
		t.Errorf("expected recognized HTTPS_PROXY=proxy.corp, got %v", recognized)
	}
}
