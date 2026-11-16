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

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

func assertErrorCode(t *testing.T, err error, want oracleErrors.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %s, got nil", want)
	}
	sqle, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("expected common.SQLError, got %T (%v)", err, err)
	}
	if sqle.ErrorCode() != string(want) {
		t.Fatalf("expected error code %s, got %s (%v)", want, sqle.ErrorCode(), err)
	}
}

func TestParseSimpleWithConnectionProperties(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))?oracle.go.Locale.Language=fr&oracle.go.DriverProperties.DefaultLobPrefetchSize=12345&oracle.go.DriverProperties.StrictNullValueHandling=false"
	parsedConfig, err := ParseDSNString(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if parsedConfig == nil {
		t.Fatalf("Parse failed: parsed config was nil")
	}

	if parsedConfig.rootNode.Name != "DESCRIPTION" {
		t.Errorf("Expected root name 'DESCRIPTION', got '%s'", parsedConfig.rootNode.Name)
	}
	if len(parsedConfig.rootNode.Children) != 1 {
		t.Errorf("Expected 1 child, got %d", len(parsedConfig.rootNode.Children))
	}
	connectData := parsedConfig.rootNode.Children[0]
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

// Tests for resolveConnectStringURL

func TestResolveConnectStringURL_EZConnect(t *testing.T) {
	t.Parallel()
	input := "localhost:1521/mydb"
	result, err := resolveConnectStringURL(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=localhost)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=mydb)))"
	if result.tns != expected {
		t.Errorf("expected %s, got %s", expected, result.tns)
	}
	if len(result.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", result.unrecognizedParams)
	}
}

func TestResolveConnectStringURL_TNS(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=localhost)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=mydb)))"
	result, err := resolveConnectStringURL(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.tns != input {
		t.Errorf("expected unchanged TNS, got %s", result.tns)
	}
	if len(result.unrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", result.unrecognizedParams)
	}
}

// TestResolveConnectStringURL_Invalid tests handling of non-matching format
// TODO enable this test after adding support for alias from tnsnames
// invalid should become an alias
/* func TestResolveConnectStringURL_Invalid(t *testing.T) {
	input := "invalid"
	result, err := resolveConnectStringURL(input)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.TNS != input {
		t.Errorf("expected TNS to be unchanged input, got %s", result.TNS)
	}
	if len(result.UnrecognizedParams) != 0 {
		t.Errorf("unexpected unrecognized params: %v", result.UnrecognizedParams)
	}
} */

// Tests for ParseDSNString
func TestParseDSNString_InvalidInputs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in           string
		code         oracleErrors.ErrorCode
		wantContains string
		notContains  []string
	}{
		{in: "", code: oracleErrors.NamingEzConnectError},
	}
	for _, c := range cases {
		_, err := ParseDSNString(c.in)
		assertErrorCode(t, err, c.code)
		if c.wantContains != "" && !strings.Contains(err.Error(), c.wantContains) {
			t.Fatalf("expected error %q to contain %q", err.Error(), c.wantContains)
		}
		for _, value := range c.notContains {
			if strings.Contains(err.Error(), value) {
				t.Fatalf("expected error %q to redact %q", err.Error(), value)
			}
		}
	}
}

func TestParseDSNString_EZConnect_Quotes_Spaces(t *testing.T) {
	t.Parallel()
	input := `localhost : 1521 / mydb?wallet_location="C:\my_wallet with space"`
	parsed, err := ParseDSNString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Ensure whitespace outside quotes is stripped (host/port/service should still parse)
	if !strings.Contains(parsed.ConnectString, "(HOST=localhost)") {
		t.Errorf("expected HOST=localhost in connect string, got %s", parsed.ConnectString)
	}
	if !strings.Contains(parsed.ConnectString, "(PORT=1521)") {
		t.Errorf("expected PORT=1521 in connect string, got %s", parsed.ConnectString)
	}
	if !strings.Contains(parsed.ConnectString, "(SERVICE_NAME=mydb)") {
		t.Errorf("expected service name in connect string, got %s", parsed.ConnectString)
	}

	// wallet_location is a recognized EZConnect parameter and should end up under SECURITY.
	if strings.Contains(parsed.ConnectString, "?oracle.wallet_location") {
		t.Errorf("expected cleaned connect string to not contain query params, got %s", parsed.ConnectString)
	}
	if !strings.Contains(parsed.ConnectString, `(WALLET_LOCATION=C:\my_wallet with space)`) {
		t.Errorf("expected quotes to be removed and spaces preserved, got %s", parsed.ConnectString)
	}
	if strings.Contains(parsed.ConnectString, "C:\\my_walletwithspace") {
		t.Errorf("expected spaces inside quotes to be preserved, got %s", parsed.ConnectString)
	}

}

func TestParseDSNString_LongTNS_WithProperties_CleanConnectString(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))?oracle.go.Locale.language=fr"
	parsed, err := ParseDSNString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.ConnectString != "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))" {
		t.Errorf("expected cleaned connect string, got %s", parsed.ConnectString)
	}
}

func TestParseDSNString_ParseError(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example))"
	_, err := ParseDSNString(input)
	if err == nil {
		t.Fatal("expected error for malformed TNS")
	}
	assertErrorCode(t, err, oracleErrors.NamingParseFailed)
}

func TestParseDSNString_ExtractContextError(t *testing.T) {
	t.Parallel()
	input := "(FOO=BAR)"
	_, err := ParseDSNString(input)
	if err == nil {
		t.Fatal("expected error for unsupported root")
	}
	assertErrorCode(t, err, oracleErrors.NamingContextError)
}

func TestParseDSNString_ConversionError(t *testing.T) {
	t.Parallel()
	input := "host:abc/mydb"
	_, err := ParseDSNString(input)
	if err == nil {
		t.Fatal("expected error for invalid port in EZConnect")
	}
	assertErrorCode(t, err, oracleErrors.NamingEzConnectError)
}

func TestParseDSNString_EmptyPassword(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	parsed, err := ParseDSNString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed.ConnectString != "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))" {
		t.Errorf("unexpected connect string: %s", parsed.ConnectString)
	}
}

func TestParseDSNString_CommaBetweenNodes_FailsFast(t *testing.T) {
	t.Parallel()
	// Regression test: ensure we fail fast (and do not hang) when an unexpected comma is present.
	input := "(description=(recv_timeout=5s),(transport_connect_timeout=10s)(connect_timeout=2s)(address=(protocol=tcp)(host=1.1.1.1)(port=4441))(connect_data=(service_name=freepdb1)))"
	_, err := ParseDSNString(input)
	if err == nil {
		t.Fatalf("expected error due to invalid comma token, got nil")
	}
	assertErrorCode(t, err, oracleErrors.NamingParseFailed)
}
