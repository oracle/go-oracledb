/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package config

import (
	"context"
	"strings"
	"testing"
	"time"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestHandleDsnBuildsConnectionPlans verifies that EZConnect and TNS inputs
// become normalized plans with DSN-wide settings merged into an owned base
// configuration and target-scoped settings resolved by the iterator.
func TestHandleDsnBuildsConnectionPlans(t *testing.T) {
	t.Parallel()

	t.Run("EZConnect with base config and query settings", func(t *testing.T) {
		t.Parallel()
		baseConfig := NewOracleDriverConfig()
		baseConfig.ConnectionProperties.SendBufSize = 1024
		baseConfig.ConnectionProperties.ConnectTimeout = 2
		baseConfig.ConnectionProperties.SSLServerDNMatch = true

		plans, err := HandleDsn("tcps://db-ez.example.com:2484/orders.example.com?connect_timeout=9&send_buf_size=4096&ssl_server_dn_match=false", baseConfig)
		if err != nil {
			t.Fatalf("HandleDsn failed: %v", err)
		}
		if plans.BaseConfig != baseConfig {
			t.Fatal("HandleDsn should use the caller-owned working configuration")
		}
		if baseConfig.ConnectionProperties.SendBufSize != 4096 || baseConfig.ConnectionProperties.ConnectTimeout != 9 || baseConfig.ConnectionProperties.SSLServerDNMatch {
			t.Fatalf("DSN query settings were not merged into the working configuration: %+v", baseConfig.ConnectionProperties)
		}
		if len(plans.Plan.Descriptions) != 1 {
			t.Fatalf("description count = %d, want 1", len(plans.Plan.Descriptions))
		}
		description := plans.Plan.Descriptions[0]
		if len(description.AddressGroups) != 1 || len(description.AddressGroups[0].AccessPoints) != 1 {
			t.Fatalf("expected one normalized address group and access point, got %+v", description.AddressGroups)
		}
		accessPoint := description.AddressGroups[0].AccessPoints[0]
		if accessPoint.Protocol != driverCommon.ProtocolTCPS || accessPoint.Host != "db-ez.example.com" || accessPoint.Port != 2484 {
			t.Fatalf("unexpected EZConnect access point: %+v", accessPoint)
		}
		if description.ConnectData == nil || description.ConnectData.ServiceName != "orders.example.com" {
			t.Fatalf("unexpected EZConnect connect data: %+v", description.ConnectData)
		}

		iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "ez-test-id")
		if err != nil {
			t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
		}
		attempt, err := iterator.Next()
		if err != nil {
			t.Fatalf("iterator.Next failed: %v", err)
		}
		if attempt.GetHost() != "db-ez.example.com" || attempt.GetPort() != 2484 || attempt.GetProtocol() != driverCommon.ProtocolTCPS {
			t.Fatalf("unexpected EZConnect attempt target: %s://%s:%d", attempt.GetProtocol(), attempt.GetHost(), attempt.GetPort())
		}
		if attempt.GetConnectTimeout() != 9*time.Second {
			t.Fatalf("connect timeout = %v, want 9s", attempt.GetConnectTimeout())
		}
		if attempt.GetSendBufferSize() != 4096 {
			t.Fatalf("send buffer size = %d, want 4096", attempt.GetSendBufferSize())
		}
		if attempt.GetSecurity().SSLServerDNMatch {
			t.Fatal("query setting ssl_server_dn_match=false should override the base configuration")
		}
		if attempt.GetConnectionID() != "ez-test-id" {
			t.Fatalf("connection ID = %q, want ez-test-id", attempt.GetConnectionID())
		}
		if got := attempt.GetEffectiveConfig().ConnectionProperties.SendBufSize; got != 4096 {
			t.Fatalf("effective config send buffer size = %d, want 4096", got)
		}
	})

	t.Run("TNS DESCRIPTION_LIST with base config and query settings", func(t *testing.T) {
		t.Parallel()
		baseConfig := NewOracleDriverConfig()
		baseConfig.ConnectionProperties.SendBufSize = 1024
		baseConfig.ConnectionProperties.ConnectTimeout = 2
		baseConfig.ConnectionProperties.SSLServerDNMatch = true
		baseConfig.ConnectionProperties.Failover = false
		baseConfig.ConnectionProperties.LoadBalance = true

		dsn := `(DESCRIPTION_LIST=(FAILOVER=on)(LOAD_BALANCE=off)` +
			`(DESCRIPTION=(CONNECT_TIMEOUT=5)(RETRY_COUNT=2)(SEND_BUF_SIZE=8192)` +
			`(ADDRESS=(PROTOCOL=tcps)(HOST=db-primary.example.com)(PORT=2484))` +
			`(CONNECT_DATA=(SERVICE_NAME=orders.example.com))` +
			`(SECURITY=(SSL_SERVER_DN_MATCH=no)(SSL_SERVER_CERT_DN="CN=orders-primary, OU=DB")))` +
			`(DESCRIPTION=(CONNECT_TIMEOUT=30)(RETRY_COUNT=0)` +
			`(ADDRESS=(PROTOCOL=tcps)(HOST=db-dr.example.com)(PORT=2484))` +
			`(CONNECT_DATA=(SERVICE_NAME=orders.example.com))` +
			`(SECURITY=(SSL_SERVER_CERT_DN="CN=orders-dr, OU=DB"))))` +
			`?send_buf_size=4096&recv_timeout=500ms`
		plans, err := HandleDsn(dsn, baseConfig)
		if err != nil {
			t.Fatalf("HandleDsn failed: %v", err)
		}
		if plans.BaseConfig != baseConfig {
			t.Fatal("HandleDsn should use the caller-owned working configuration")
		}
		if baseConfig.ConnectionProperties.SendBufSize != 4096 || baseConfig.ConnectionProperties.ConnectTimeout != 2 || !baseConfig.ConnectionProperties.SSLServerDNMatch || baseConfig.ConnectionProperties.Failover || !baseConfig.ConnectionProperties.LoadBalance || baseConfig.ConnectionProperties.ReceiveTimeout != "500ms" {
			t.Fatalf("DSN-wide settings were not merged without changing descriptor-scoped policy: %+v", baseConfig.ConnectionProperties)
		}
		if plans.Plan.Failover == nil || !*plans.Plan.Failover {
			t.Fatalf("plan failover = %v, want true", plans.Plan.Failover)
		}
		if plans.Plan.LoadBalance == nil || *plans.Plan.LoadBalance {
			t.Fatalf("plan load balance = %v, want false", plans.Plan.LoadBalance)
		}
		if len(plans.Plan.Descriptions) != 2 {
			t.Fatalf("description count = %d, want 2", len(plans.Plan.Descriptions))
		}
		if plans.BaseConfig.ConnectionProperties.SendBufSize != 4096 || plans.BaseConfig.ConnectionProperties.ReceiveTimeout != "500ms" {
			t.Fatalf("DSN query settings were not merged into base config: %+v", plans.BaseConfig.ConnectionProperties)
		}
		iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "tns-test-id")
		if err != nil {
			t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
		}
		primary, err := iterator.Next()
		if err != nil {
			t.Fatalf("iterator.Next for primary failed: %v", err)
		}
		if primary.GetHost() != "db-primary.example.com" || primary.GetConnectTimeout() != 5*time.Second || primary.GetRetryCount() != 2 {
			t.Fatalf("unexpected primary attempt: host=%q timeout=%v retryCount=%d", primary.GetHost(), primary.GetConnectTimeout(), primary.GetRetryCount())
		}
		if primary.GetSendBufferSize() != 8192 {
			t.Fatalf("primary send buffer size = %d, want DESCRIPTION override 8192", primary.GetSendBufferSize())
		}
		if primary.GetReceiveTimeout() != 500*time.Millisecond {
			t.Fatalf("primary receive timeout = %v, want 500ms", primary.GetReceiveTimeout())
		}
		if security := primary.GetSecurity(); security.SSLServerDNMatch || security.SSLServerCertDN != "CN=orders-primary, OU=DB" {
			t.Fatalf("unexpected primary security settings: %+v", security)
		}
		if data := primary.GetConnectData(); data.ServiceName != "orders.example.com" {
			t.Fatalf("primary service name = %q, want orders.example.com", data.ServiceName)
		}
		if primary.GetConnectionID() != "tns-test-id" {
			t.Fatalf("primary connection ID = %q, want tns-test-id", primary.GetConnectionID())
		}

		// RETRY_COUNT=2 yields two more attempts against the primary target
		// before failover advances the iterator to the second DESCRIPTION.
		for retry := 0; retry < 2; retry++ {
			retryAttempt, err := iterator.Next()
			if err != nil {
				t.Fatalf("iterator.Next for primary retry %d failed: %v", retry+1, err)
			}
			if retryAttempt.GetHost() != "db-primary.example.com" {
				t.Fatalf("primary retry %d host = %q, want db-primary.example.com", retry+1, retryAttempt.GetHost())
			}
		}

		disasterRecovery, err := iterator.Next()
		if err != nil {
			t.Fatalf("iterator.Next for disaster-recovery target failed: %v", err)
		}
		if disasterRecovery.GetHost() != "db-dr.example.com" || disasterRecovery.GetConnectTimeout() != 30*time.Second || disasterRecovery.GetRetryCount() != 0 {
			t.Fatalf("unexpected disaster-recovery attempt: host=%q timeout=%v retryCount=%d", disasterRecovery.GetHost(), disasterRecovery.GetConnectTimeout(), disasterRecovery.GetRetryCount())
		}
		if disasterRecovery.GetSendBufferSize() != 4096 {
			t.Fatalf("disaster-recovery send buffer size = %d, want query setting 4096", disasterRecovery.GetSendBufferSize())
		}
		if security := disasterRecovery.GetSecurity(); !security.SSLServerDNMatch || security.SSLServerCertDN != "CN=orders-dr, OU=DB" {
			t.Fatalf("unexpected disaster-recovery security settings: %+v", security)
		}
	})
}

// TestConnectionAttemptBuildsConnectDescriptorWithClientInfo verifies that
// session-specific CONNECT_DATA is serialized from the resolved attempt and
// replaces any caller-supplied CID or CONNECTION_ID fields.
func TestConnectionAttemptBuildsConnectDescriptorWithClientInfo(t *testing.T) {
	t.Parallel()

	dsn := `(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=db.example.com)(PORT=1521))` +
		`(CONNECT_DATA=(SERVICE_NAME=orders)(CID=(PROGRAM=caller)(HOST=caller)(USER=caller))` +
		`(CONNECTION_ID=caller-id)))`
	baseConfig := NewOracleDriverConfig()
	baseConfig.ConnectionProperties.ConnectionIDPrefix = "prefix-"
	plans, err := HandleDsn(dsn, baseConfig)
	if err != nil {
		t.Fatalf("HandleDsn failed: %v", err)
	}
	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "generated-id")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	attempt, err := iterator.Next()
	if err != nil {
		t.Fatalf("iterator.Next failed: %v", err)
	}

	const want = `(CID=(PROGRAM=driver)(HOST=client)(USER=alice))(CONNECTION_ID=prefix-generated-id)`
	if attempt.GetConnectionID() != "prefix-generated-id" {
		t.Fatalf("connection ID = %q, want prefix-generated-id", attempt.GetConnectionID())
	}
	descriptor := attempt.GetConnectDescriptorWithClientInfo("driver", "client", "alice")
	if !strings.Contains(descriptor, "(HOST=db.example.com)") {
		t.Fatalf("descriptor does not contain resolved target host: %s", descriptor)
	}
	if !strings.Contains(descriptor, want) {
		t.Fatalf("descriptor does not contain generated session values %s: %s", want, descriptor)
	}
	if strings.Contains(descriptor, "caller") || strings.Contains(descriptor, "caller-id") {
		t.Fatalf("descriptor retained caller-supplied session values: %s", descriptor)
	}
}

// TestHandleDsnPreservesExplicitEmptyConnectDataQuery verifies that config
// assignment and the query-field presence set together retain an explicit empty
// CONNECT_DATA value through attempt resolution.
func TestHandleDsnPreservesExplicitEmptyConnectDataQuery(t *testing.T) {
	t.Parallel()

	plans, err := HandleDsn("(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=db.example.com)(PORT=1521)))?service_name=", NewOracleDriverConfig())
	if err != nil {
		t.Fatalf("HandleDsn failed: %v", err)
	}
	if _, ok := plans.queryConnectDataSet["SERVICE_NAME"]; !ok {
		t.Fatal("query field set does not record explicitly supplied SERVICE_NAME")
	}

	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "empty-service-id")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	attempt, err := iterator.Next()
	if err != nil {
		t.Fatalf("iterator.Next failed: %v", err)
	}
	if _, ok := attempt.GetConnectDataSet()["SERVICE_NAME"]; !ok {
		t.Fatal("attempt lost the explicit SERVICE_NAME presence marker")
	}
	if descriptor := attempt.GetConnectDataDescriptor(); !strings.Contains(descriptor, "(SERVICE_NAME=)") {
		t.Fatalf("descriptor did not preserve the explicit empty SERVICE_NAME: %s", descriptor)
	}
}

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
	root, err := ParseConnectorDescriptor(input)
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
	root, err := ParseConnectorDescriptor(input)
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
	_, err := ParseConnectorDescriptor("")
	assertConfigErrorCode(t, err, oracleErrors.NamingTokensMissing)
}

func TestParseWhitespaceOnly(t *testing.T) {
	t.Parallel()
	_, err := ParseConnectorDescriptor(" \t\n\r ")
	assertConfigErrorCode(t, err, oracleErrors.NamingTokensMissing)
}

func TestParseInvalid(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example"
	_, err := ParseConnectorDescriptor(input)
	assertConfigErrorCode(t, err, oracleErrors.NamingParseBounds)
}

func TestGetValue(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := ParseConnectorDescriptor(input)
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
	root, err := ParseConnectorDescriptor(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	_, err = root.GetValue("DESCRIPTION/NOT_FOUND")
	assertConfigErrorCode(t, err, oracleErrors.NamingParsePathSegment)
}

func TestGetNode(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := ParseConnectorDescriptor(input)
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
	root, err := ParseConnectorDescriptor(input)
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
	root, err := ParseConnectorDescriptor(input)
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
	if out != input {
		t.Fatalf("descriptor round trip changed quoted values\ninput:  %s\noutput: %s", input, out)
	}
}

func TestChildCount(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)(SERVER=DEDICATED)))"
	root, err := ParseConnectorDescriptor(input)
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
	root, err := ParseConnectorDescriptor(input)
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
	_, err := ParseConnectorDescriptor("(")
	if err == nil {
		t.Error("Expected error for '(' without name")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingParsePosition)
}

func TestParseUnexpectedClosingParen(t *testing.T) {
	t.Parallel()
	_, err := ParseConnectorDescriptor(")")
	if err == nil {
		t.Error("Expected error for unexpected ')'")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingUnexpectedClosingParenthesis)
}

func TestParseUnexpectedToken(t *testing.T) {
	t.Parallel()
	_, err := ParseConnectorDescriptor("foo")
	if err == nil {
		t.Error("Expected error for unexpected token")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingParseValue)
}

func TestGetValueNonLeafNode(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := ParseConnectorDescriptor(input)
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
	root, err := ParseConnectorDescriptor(input)
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
	root, err := ParseConnectorDescriptor(input)
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
	root, err := ParseConnectorDescriptor(input)
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
	root, err := ParseConnectorDescriptor(input)
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
	root, err := ParseConnectorDescriptor(input)
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
