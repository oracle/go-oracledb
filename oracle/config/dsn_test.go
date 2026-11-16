/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package config

import (
	"strings"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

func assertConfigErrorCode(t *testing.T, err error, want oracleErrors.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %s, got nil", want)
	}
	sqle, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("expected SQLError, got %T (%v)", err, err)
	}
	if got := sqle.ErrorCode(); got != string(want) {
		t.Fatalf("error code = %s, want %s (%v)", got, want, err)
	}
}

func TestParseSimple(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if root.Name != "DESCRIPTION" {
		t.Errorf("Expected root name 'DESCRIPTION', got '%s'", root.Name)
	}
	if len(root.Children) != 1 {
		t.Errorf("Expected 1 child, got %d", len(root.Children))
	}
	connectData := root.Children[0]
	if connectData.Name != "CONNECT_DATA" {
		t.Errorf("Expected child name 'CONNECT_DATA', got '%s'", connectData.Name)
	}
	if len(connectData.Children) != 1 {
		t.Errorf("Expected 1 grandchild, got %d", len(connectData.Children))
	}
	serviceName := connectData.Children[0]
	if serviceName.Name != "SERVICE_NAME" {
		t.Errorf("Expected grandchild name 'SERVICE_NAME', got '%s'", serviceName.Name)
	}
	if serviceName.Value != "example" {
		t.Errorf("Expected value 'example', got '%s'", serviceName.Value)
	}
}

func TestParseComplex(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=localhost)(PORT=1521)))(CONNECT_DATA=(SERVICE_NAME=example)(SERVER=DEDICATED)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if root.Name != "DESCRIPTION" {
		t.Errorf("Expected root name 'DESCRIPTION', got '%s'", root.Name)
	}
	if len(root.Children) != 2 {
		t.Errorf("Expected 2 children, got %d", len(root.Children))
	}
}

func TestParseEmpty(t *testing.T) {
	t.Parallel()
	_, err := Parse("")
	assertConfigErrorCode(t, err, oracleErrors.NamingInputMissing)
}

func TestParseWhitespaceOnly(t *testing.T) {
	t.Parallel()
	_, err := Parse(" \t\n\r ")
	assertConfigErrorCode(t, err, oracleErrors.NamingInputMissing)
}

func TestParseInvalid(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example"
	_, err := Parse(input)
	assertConfigErrorCode(t, err, oracleErrors.NamingParseBounds)
}

func TestGetValue(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	value, err := root.GetValue("DESCRIPTION/CONNECT_DATA/SERVICE_NAME")
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if value != "example" {
		t.Errorf("Expected 'example', got '%s'", value)
	}
}

func TestGetValueNotFound(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	_, err = root.GetValue("DESCRIPTION/NOT_FOUND")
	assertConfigErrorCode(t, err, oracleErrors.NamingParsePathSegment)
}

func TestGetNode(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	node, err := root.GetNode("DESCRIPTION/CONNECT_DATA")
	if err != nil {
		t.Fatalf("GetNode failed: %v", err)
	}
	if node.Name != "CONNECT_DATA" {
		t.Errorf("Expected 'CONNECT_DATA', got '%s'", node.Name)
	}
}

func TestToString(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	output := root.ToString()
	if output != input {
		t.Errorf("ToString mismatch: expected '%s', got '%s'", input, output)
	}
}

func TestParse_PreservesQuotedValues(t *testing.T) {
	t.Parallel()
	input := `(DESCRIPTION=(SECURITY=(SSL_SERVER_CERT_DN="CN=My Server, OU=Org"))(CONNECT_DATA=(SERVICE_NAME="Sales Service")))`
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}
	v, err := root.GetValue("DESCRIPTION/SECURITY/SSL_SERVER_CERT_DN")
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if v != `CN=My Server, OU=Org` {
		t.Fatalf("expected wrapping quotes to be removed, got %q", v)
	}
	out := root.ToString()
	if out == input {
		t.Fatalf("expected output string without quotes\ninput:  %s\noutput: %s", input, out)
	}
	if strings.Contains(out, `\"CN=My Server, OU=Org\"`) {
		t.Fatalf("expected output to without quotes, got: %s", out)
	}
}

func TestChildCount(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)(SERVER=DEDICATED)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	connectData := root.Children[0]
	if connectData.ChildCount() != 2 {
		t.Errorf("Expected 2 children, got %d", connectData.ChildCount())
	}
}

func TestGetChild(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	connectData := root.Children[0]
	child, err := connectData.GetChild(0)
	if err != nil {
		t.Fatalf("GetChild failed: %v", err)
	}
	if child.Name != "SERVICE_NAME" {
		t.Errorf("Expected 'SERVICE_NAME', got '%s'", child.Name)
	}
}

func TestParseUnexpectedOpeningParenWithoutName(t *testing.T) {
	t.Parallel()
	_, err := Parse("(")
	if err == nil {
		t.Error("Expected error for '(' without name")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingParsePosition)
}

func TestParseUnexpectedClosingParen(t *testing.T) {
	t.Parallel()
	_, err := Parse(")")
	if err == nil {
		t.Error("Expected error for unexpected ')'")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingUnexpectedClosingParenthesis)
}

func TestParseUnexpectedToken(t *testing.T) {
	t.Parallel()
	_, err := Parse("foo")
	if err == nil {
		t.Error("Expected error for unexpected token")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingParseValue)
}

func TestGetValueNonLeafNode(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	_, err = root.GetValue("DESCRIPTION/CONNECT_DATA")
	if err == nil {
		t.Error("Expected error for non-leaf node")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingParsePath)
}

func TestGetNodeEmptyPath(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	_, err = root.GetNode("")
	if err == nil {
		t.Error("Expected error for empty path")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingInputMissing)
}

func TestGetNodeWrongRootName(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	_, err = root.GetNode("WRONG")
	if err == nil {
		t.Error("Expected error for wrong root name")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingParseValues)
}

func TestGetNodeEmptySegment(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	_, err = root.GetNode("DESCRIPTION//CONNECT_DATA")
	if err == nil {
		t.Error("Expected error for empty path segment")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingParsePathSegment)
}

func TestGetChildInvalidIndex(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	_, err = root.GetChild(-1)
	if err == nil {
		t.Error("Expected error for negative index")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingParseBounds)
	_, err = root.GetChild(100)
	if err == nil {
		t.Error("Expected error for out of bounds index")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingParseBounds)
}

func TestGetNodeRootPath(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	node, err := root.GetNode("DESCRIPTION")
	if err != nil {
		t.Errorf("GetNode failed: %v", err)
	}
	if node.Name != "DESCRIPTION" {
		t.Errorf("Expected 'DESCRIPTION', got '%s'", node.Name)
	}
}

func TestParseIterative_NoTokens(t *testing.T) {
	t.Parallel()
	_, err := parseIterative([]string{})
	assertConfigErrorCode(t, err, oracleErrors.NamingTokensMissing)
}

func TestParseEazyConnectDescriptorAllComponents(t *testing.T) {
	t.Parallel()

	parsed, serverMode, err := parseEazyConnectDescriptor("tcps://host1:2484,host2:2484/mydb:dedicated/inst1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.protocol != driverCommon.ProtocolTCPS {
		t.Errorf("expected TCPS protocol, got %v", parsed.protocol)
	}
	if parsed.serviceName != "mydb" {
		t.Errorf("expected mydb service, got %q", parsed.serviceName)
	}
	if serverMode != "DEDICATED" {
		t.Errorf("expected DEDICATED mode, got %q", serverMode)
	}
	if parsed.instanceName != "inst1" {
		t.Errorf("expected inst1 instance, got %q", parsed.instanceName)
	}
	if len(parsed.addressGroups) != 1 || len(parsed.addressGroups[0].hosts) != 2 {
		t.Fatalf("expected one address group with two hosts, got %+v", parsed.addressGroups)
	}
}

func TestParseEazyConnectDescriptorMinimalURL(t *testing.T) {
	t.Parallel()

	parsed, serverMode, err := parseEazyConnectDescriptor("host")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.protocol != driverCommon.ProtocolTCP {
		t.Errorf("expected default TCP protocol, got %v", parsed.protocol)
	}
	if parsed.serviceName != "" {
		t.Errorf("expected empty service name, got %q", parsed.serviceName)
	}
	if serverMode != "" {
		t.Errorf("expected empty server mode, got %q", serverMode)
	}
}

func TestParseEazyConnectDescriptorProtocols(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		protocol driverCommon.Protocol
	}{
		{"tcp://myhost:1521/mydb", driverCommon.ProtocolTCP},
		{"tcps://myhost:2484/securedb", driverCommon.ProtocolTCPS},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			parsed, _, err := parseEazyConnectDescriptor(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if parsed.protocol != tc.protocol {
				t.Errorf("protocol = %v, want %v", parsed.protocol, tc.protocol)
			}
		})
	}
}

func TestParseEazyConnectDescriptorInvalidProtocol(t *testing.T) {
	t.Parallel()

	parsed, _, err := parseEazyConnectDescriptor("http://myhost:1521/mydb")
	if err == nil {
		t.Fatal("expected error for unsupported protocol")
	}
	if !strings.Contains(err.Error(), "http") || !strings.Contains(err.Error(), "protocol") {
		t.Errorf("expected error mentioning protocol http, got: %v", err)
	}
	if parsed != nil {
		t.Error("expected nil parsed result on error")
	}
}

func TestParseEazyConnectDescriptorInvalidPorts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		value string
	}{
		{"host:abc/mydb", "abc"},
		{"host:0/mydb", "0"},
		{"host:-1/mydb", "-1"},
		{"host:65536/mydb", "65536"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			parsed, _, err := parseEazyConnectDescriptor(tc.input)
			if err == nil {
				t.Fatalf("expected error for %q", tc.input)
			}
			if !strings.Contains(err.Error(), tc.value) || !strings.Contains(err.Error(), "port") {
				t.Errorf("expected error mentioning value %q and field port, got: %v", tc.value, err)
			}
			if parsed != nil {
				t.Errorf("expected nil result on error for %q", tc.input)
			}
		})
	}
}

func TestParseEazyConnectDescriptorValidPortBoundaries(t *testing.T) {
	t.Parallel()

	for _, port := range []string{"1", "65535"} {
		t.Run(port, func(t *testing.T) {
			t.Parallel()
			parsed, _, err := parseEazyConnectDescriptor("host:" + port + "/mydb")
			if err != nil {
				t.Fatalf("unexpected error for port %s: %v", port, err)
			}
			if got := parsed.addressGroups[0].hosts[0].port; got != port {
				t.Errorf("port = %q, want %q", got, port)
			}
		})
	}
}

func TestParseEazyConnectDescriptorEmptyAddressGroups(t *testing.T) {
	t.Parallel()

	parsed, _, err := parseEazyConnectDescriptor(";;/mydb")
	if err == nil {
		t.Fatal("expected error for no valid address groups")
	}
	if !strings.Contains(err.Error(), "host") {
		t.Errorf("expected error mentioning host, got: %v", err)
	}
	if parsed != nil {
		t.Error("expected nil parsed result on error")
	}
}

func TestParseHostListTrailingComma(t *testing.T) {
	t.Parallel()

	_, err := parseHostList("host1:1521,")
	if err == nil {
		t.Fatal("expected error for empty host in list")
	}
	if !strings.Contains(err.Error(), "host") {
		t.Errorf("expected error mentioning host, got: %v", err)
	}
}

func TestParseHostListMultipleHosts(t *testing.T) {
	t.Parallel()

	hosts, err := parseHostList("host1:1521,host2:1522,host3:1523")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 3 {
		t.Fatalf("expected 3 hosts, got %d", len(hosts))
	}
}

func TestParseHostListEmptyHost(t *testing.T) {
	t.Parallel()

	if _, err := parseHostList(""); err == nil {
		t.Fatal("expected error for empty host string")
	}
}

func TestParseHostListDefaultPortInMiddle(t *testing.T) {
	t.Parallel()

	hosts, err := parseHostList("host1,host2:1522")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}
	if hosts[0].host != "host1" || hosts[0].port != defaultOraclePort {
		t.Errorf("expected host1 default port %s, got %+v", defaultOraclePort, hosts[0])
	}
	if hosts[1].host != "host2" || hosts[1].port != "1522" {
		t.Errorf("expected host2:1522, got %+v", hosts[1])
	}
}

func TestParseAddressGroupsMultipleGroups(t *testing.T) {
	t.Parallel()

	groups, err := parseAddressGroups("host1:1521;host2:1522;host3:1523")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 3 {
		t.Errorf("expected 3 groups, got %d", len(groups))
	}
}

func TestParseAddressGroupsEmptyString(t *testing.T) {
	t.Parallel()

	if _, err := parseAddressGroups(""); err == nil {
		t.Fatal("expected error for empty address groups")
	}
}
