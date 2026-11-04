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

package ttc

import (
	"bytes"
	"context"
	"strings"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
)

// TestTTICookieEligibility verifies that both the property and database UUID
// are required before looking up or populating negotiation data.
func TestTTICookieEligibility(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		enabled bool
		uuid    string
		want    bool
	}{
		{"enabled with UUID", true, "database-uuid", true},
		{"disabled", false, "database-uuid", false},
		{"missing UUID", true, "", false},
		{"blank UUID", true, " \t", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			props := oracleconfig.OracleDriverProperties{TTCCookieOptimizationEnabled: tc.enabled}
			if got := IsTTCCookieEligible(props, tc.uuid); got != tc.want {
				t.Fatalf("eligible = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestTTICookieCopiesRuntimeCapabilities verifies that the cookie's runtime
// capabilities do not share the source PRO's backing array.
func TestTTICookieCopiesRuntimeCapabilities(t *testing.T) {
	t.Parallel()

	pro := &tTIpro{
		serverCaps: &capability{
			runTimeCapabilities: []byte{1, 2, 3},
		},
	}

	cookie := newTTICookieFromTTIpro(pro)
	pro.serverCaps.runTimeCapabilities[0] = 255

	if got := cookie.databaseRuntimeCapabilities[0]; got != 1 {
		t.Errorf("cached runtime capability = %d, want 1", got)
	}
}

// TestNewTTICookieFromTTIpro verifies that the cookie preserves the server's
// negotiated protocol fields, capabilities, and platform description.
func TestNewTTICookieFromTTIpro(t *testing.T) {
	t.Parallel()
	runtime := []byte{1, 2, 3}
	compile := []byte{4, 5, 6, 7, 8, 9, 10, 123}
	desc := driverCommon.B1Array("test")
	p := &tTIpro{
		proSvrVer:          456,
		clientCaps:         &capability{runTimeCapabilities: []byte{90}, compileTimeCapabilities: []byte{91}},
		serverCaps:         &capability{runTimeCapabilities: runtime, compileTimeCapabilities: compile},
		svrCharSet:         871,
		nCharCharset:       775,
		svrFlags:           1,
		svrPortDescription: desc,
	}

	cookie := newTTICookieFromTTIpro(p)

	if cookie.GetMsgCode() != TTICOOKIE {
		t.Errorf("GetMsgCode = %v, want %v", cookie.GetMsgCode(), TTICOOKIE)
	}
	if cookie.connectionProtocolVersion != p.proSvrVer {
		t.Errorf("protocol version = %d, want %d", cookie.connectionProtocolVersion, p.proSvrVer)
	}
	if cookie.databaseCharSet != p.svrCharSet {
		t.Errorf("database character set = %d, want %d", cookie.databaseCharSet, p.svrCharSet)
	}
	if cookie.databaseNCharSet != p.nCharCharset {
		t.Errorf("database NCHAR character set = %d, want %d", cookie.databaseNCharSet, p.nCharCharset)
	}
	if cookie.databaseCharSetFlag != p.svrFlags {
		t.Errorf("character set flag = %d, want %d", cookie.databaseCharSetFlag, p.svrFlags)
	}
	if !bytes.Equal(cookie.databaseRuntimeCapabilities, runtime) {
		t.Errorf("runtime capabilities = %x, want %x", cookie.databaseRuntimeCapabilities, runtime)
	}
	if !bytes.Equal(cookie.databaseCompileTimeCapabilities, compile) {
		t.Errorf("compile-time capabilities = %x, want %x", cookie.databaseCompileTimeCapabilities, compile)
	}
	if !bytes.Equal(cookie.databasePortage, desc) {
		t.Errorf("platform description = %q, want %q", cookie.databasePortage, desc)
	}
}

// TestTTICookieMarshalTo_Fail verifies that a failure writing the cookie version
// is returned to the caller.
func TestTTICookieMarshalTo_Fail(t *testing.T) {
	t.Parallel()
	cookie := &ttiCookie{
		connectionProtocolVersion:       6,
		databaseCharSet:                 873,
		databaseCharSetFlag:             1,
		databaseNCharSet:                2000,
		databasePortage:                 []byte("X"),
		databaseCompileTimeCapabilities: []byte{0xaa, 0xbb},
		databaseRuntimeCapabilities:     []byte{0xcc},
	}
	ctx := context.Background()

	adb := NewArrayDataBuffer(128)

	buf := &FaultyArrayBasedDataBuffer{
		ArrayBasedDataBuffer: adb,
		FailOnWriteByteCall:  1,
	}

	marshal := NewNativeMarshalEngine(buf, driverCommon.BIG_ENDIAN)

	err := cookie.MarshalTo(ctx, marshal)
	if err == nil || !strings.Contains(err.Error(), "simulated write error (WriteByte)") {
		t.Fatalf("expected injected write error, got %v", err)
	}
}

// TestTTICookieMarshalWire verifies cookie field order, fixed-width charset
// byte order, portage termination, and capability lengths without a round trip.
func TestTTICookieMarshalWire(t *testing.T) {
	t.Parallel()
	cookie := &ttiCookie{
		connectionProtocolVersion:       6,
		databaseCharSet:                 873,
		databaseCharSetFlag:             1,
		databaseNCharSet:                2000,
		databasePortage:                 []byte("X"),
		databaseCompileTimeCapabilities: []byte{0xaa, 0xbb},
		databaseRuntimeCapabilities:     []byte{0xcc},
	}
	buffer := NewArrayDataBuffer(128)
	if err := cookie.MarshalTo(context.Background(), NewNativeMarshalEngine(buffer, driverCommon.BIG_ENDIAN)); err != nil {
		t.Fatalf("MarshalTo: %v", err)
	}
	want := []byte{1, 6, 0x69, 0x03, 1, 0xd0, 0x07, 2, 'X', 0, 2, 0xaa, 0xbb, 1, 0xcc}
	if got := buffer.bytes[:buffer.currentWritePosition]; !bytes.Equal(got, want) {
		t.Fatalf("cookie bytes = %x, want %x", got, want)
	}
	if cookie.GetMsgCode() != TTICOOKIE {
		t.Fatal("unexpected cookie message code")
	}
}
