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

	"github.com/oracle/go-driver/driver/common"
)

/* Test helpers */

func mustParse(t *testing.T, s string) *Node {
	t.Helper()
	n, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	return n
}

/* ExtractConnectionContext */

func TestExtractConnectionContext_Success(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		input         string
		wantDesc      bool
		wantDescList  bool
		wantAddrCount int
		verify        func(t *testing.T, ctx *ConnectionContext)
	}{
		{
			name:     "DESCRIPTION",
			input:    "(DESCRIPTION=(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=localhost)(PORT=1521)))(CONNECT_DATA=(SERVICE_NAME=example)))",
			wantDesc: true,
			verify: func(t *testing.T, ctx *ConnectionContext) {
				if ctx.Description == nil {
					t.Fatal("expected Description")
				}
				if len(ctx.Description.AddressLists) != 1 {
					t.Errorf("expected 1 AddressList, got %d", len(ctx.Description.AddressLists))
				}
				if ctx.Description.ConnectData.ServiceName != "example" {
					t.Errorf("expected ServiceName=example, got %q", ctx.Description.ConnectData.ServiceName)
				}
			},
		},
		{
			name:         "DESCRIPTION_LIST",
			input:        "(DESCRIPTION_LIST=(LOAD_BALANCE=on)(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example1)))(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example2))))",
			wantDescList: true,
			verify: func(t *testing.T, ctx *ConnectionContext) {
				if ctx.DescriptionList == nil {
					t.Fatal("expected DescriptionList")
				}
				if ctx.DescriptionList.LoadBalance != true {
					t.Errorf("expected LoadBalance=on, got %t", ctx.DescriptionList.LoadBalance)
				}
				if len(ctx.DescriptionList.Descriptions) != 2 {
					t.Errorf("expected 2 Descriptions, got %d", len(ctx.DescriptionList.Descriptions))
				}
				if ctx.DescriptionList.Descriptions[0].ConnectData.ServiceName != "example1" {
					t.Errorf("expected first ServiceName=example1, got %q", ctx.DescriptionList.Descriptions[0].ConnectData.ServiceName)
				}
			},
		},
		{
			name:          "ADDRESS_LIST",
			input:         "(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522)))",
			wantAddrCount: 2,
			verify: func(t *testing.T, ctx *ConnectionContext) {
				if len(ctx.Addresses) != 2 {
					t.Fatalf("expected 2 addresses, got %d", len(ctx.Addresses))
				}
				if ctx.Addresses[0].Host != "host1" || ctx.Addresses[1].Host != "host2" {
					t.Errorf("unexpected hosts: %+v", ctx.Addresses)
				}
			},
		},
		{
			name:          "ADDRESS",
			input:         "(ADDRESS=(PROTOCOL=TCP)(HOST=localhost)(PORT=1521))",
			wantAddrCount: 1,
			verify: func(t *testing.T, ctx *ConnectionContext) {
				if len(ctx.Addresses) != 1 {
					t.Fatalf("expected 1 address, got %d", len(ctx.Addresses))
				}
				a := ctx.Addresses[0]
				if a.Protocol != common.ProtocolTCP || a.Host != "localhost" || a.Port != 1521 {
					t.Errorf("unexpected address: %+v", a)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := mustParse(t, tc.input)
			ctx, err := ExtractConnectionContext(root)
			if err != nil {
				t.Fatalf("ExtractConnectionContext error: %v", err)
			}
			if tc.wantDesc && ctx.Description == nil {
				t.Fatal("expected Description non-nil")
			}
			if tc.wantDescList && ctx.DescriptionList == nil {
				t.Fatal("expected DescriptionList non-nil")
			}
			if tc.wantAddrCount > 0 && len(ctx.Addresses) != tc.wantAddrCount {
				t.Fatalf("expected %d addresses, got %d", tc.wantAddrCount, len(ctx.Addresses))
			}
			if tc.verify != nil {
				tc.verify(t, ctx)
			}
		})
	}
}

func TestExtractConnectionContext_Errors(t *testing.T) {
	t.Parallel()
	t.Run("nil root", func(t *testing.T) {
		if _, err := ExtractConnectionContext(nil); err == nil {
			t.Error("expected error for nil root")
		}
	})

	t.Run("unsupported root", func(t *testing.T) {
		root := mustParse(t, "(UNSUPPORTED=(VALUE=test))")
		if _, err := ExtractConnectionContext(root); err == nil {
			t.Error("expected error for unsupported root")
		}
	})

	errCases := []struct {
		name  string
		input string
	}{
		{"description invalid", "(DESCRIPTION=(INVALID=bad))"},
		{"description_list invalid", "(DESCRIPTION_LIST=(INVALID=bad))"},
		{"address_list invalid", "(ADDRESS_LIST=(INVALID=bad))"},
		{"address invalid", "(ADDRESS=(INVALID=bad))"},
	}
	for _, ec := range errCases {
		t.Run(ec.name, func(t *testing.T) {
			root := mustParse(t, ec.input)
			if _, err := ExtractConnectionContext(root); err == nil {
				t.Error("expected error")
			}
		})
	}
}

/* extractDescription + coverage for most branches */

func TestExtractDescription_Complete(t *testing.T) {
	t.Parallel()
	input := `(DESCRIPTION=
  (SOURCE_ROUTE=yes)
  (LOAD_BALANCE=off)
  (FAILOVER=on)
  (USE_SNI=no)
  (CONNECT_TIMEOUT=15)
  (RETRY_COUNT=2)
  (RETRY_DELAY=1)
  (TRANSPORT_CONNECT_TIMEOUT=5)
  (EXPIRE_TIME=10)
  (RECV_TIMEOUT=30)
  (SDU=8192)
  (RECV_BUF_SIZE=1024)
  (SEND_BUF_SIZE=2048)
  (ENABLE=broken)
  (CONNECTION_ID_PREFIX=prefix)
  (COMPRESSION=on)
  (COMPRESSION_LEVELS=(LEVEL=low)(LEVEL=high))
  (SECURITY=(SSL_SERVER_CERT_DN=cert)(SSL_SERVER_DN_MATCH=yes)(SSL_ALLOW_WEAK_DN_MATCH=no)(WALLET_LOCATION=/loc)(MY_WALLET_DIRECTORY=/dir))
  (ADDRESS_LIST=(LOAD_BALANCE=on)(FAILOVER=off)(SOURCE_ROUTE=yes)(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521)))
  (ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522))
  (CONNECT_DATA=(SERVICE_NAME=service)(SID=sid)(SERVER=dedicated)(INSTANCE_NAME=inst)(FAILOVER_MODE=select)(HS=ok)(RDB_DATABASE=db)(GLOBAL_NAME=global)(CONNECTION_ID_PREFIX=conn_prefix))
)`
	root := mustParse(t, input)
	desc, err := extractDescription(root)
	if err != nil {
		t.Fatalf("extractDescription failed: %v", err)
	}
	if desc.SourceRoute != true {
		t.Errorf("expected SourceRoute yes, got %t", desc.SourceRoute)
	}
	if desc.ConnectTimeout != 15000 {
		t.Errorf("expected ConnectTimeout=15000, got %d", desc.ConnectTimeout)
	}
	if len(desc.CompressionLevels) != 2 {
		t.Errorf("expected 2 compression levels, got %d", len(desc.CompressionLevels))
	}
	if desc.Security.SSLServerCertDN != "cert" {
		t.Errorf("expected SSLServerCertDN cert, got %q", desc.Security.SSLServerCertDN)
	}
	if len(desc.AddressLists) != 1 {
		t.Errorf("expected 1 AddressList, got %d", len(desc.AddressLists))
	}
	if len(desc.Addresses) != 1 {
		t.Errorf("expected 1 direct address, got %d", len(desc.Addresses))
	}
	if desc.ConnectData.ServiceName != "service" {
		t.Errorf("expected ServiceName service, got %q", desc.ConnectData.ServiceName)
	}
}

func TestExtractDescription_DefaultSecurity(t *testing.T) {
	t.Parallel()
	root := mustParse(t, "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=host)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=service)))")
	desc, err := extractDescription(root)
	if err != nil {
		t.Fatalf("extractDescription failed: %v", err)
	}
	if !desc.Security.SSLServerDNMatch {
		t.Fatal("expected SSLServerDNMatch to default to true when SECURITY node is absent")
	}
	if desc.Security.SSLAllowWeakDNMatch {
		t.Fatal("expected SSLAllowWeakDNMatch to default to false")
	}
}

func TestExtractDescription_CompressionLevels(t *testing.T) {
	t.Parallel()
	root := mustParse(t, "(DESCRIPTION=(COMPRESSION_LEVELS=(LEVEL=low)(UNKNOWN=bad)(LEVEL=high)))")
	desc, err := extractDescription(root)
	if err != nil {
		t.Fatalf("extractDescription failed: %v", err)
	}
	if len(desc.CompressionLevels) != 2 || desc.CompressionLevels[0] != "low" || desc.CompressionLevels[1] != "high" {
		t.Errorf("unexpected compression levels: %+v", desc.CompressionLevels)
	}
}

/* AddressList */

func TestExtractAddressList_FullEmptyAndErrors(t *testing.T) {
	t.Parallel()
	t.Run("full", func(t *testing.T) {
		root := mustParse(t, "(ADDRESS_LIST=(LOAD_BALANCE=on)(FAILOVER=off)(SOURCE_ROUTE=yes)(ADDRESS=(PROTOCOL=TCP)(HOST=host)(PORT=1521)))")
		al, err := extractAddressList(root)
		if err != nil {
			t.Fatalf("extractAddressList failed: %v", err)
		}
		if al.LoadBalance != true || al.Failover != false || al.SourceRoute != true {
			t.Errorf("unexpected flags: %+v", al)
		}
		if len(al.Addresses) != 1 {
			t.Errorf("expected 1 address, got %d", len(al.Addresses))
		}
	})

	t.Run("empty", func(t *testing.T) {
		root := mustParse(t, "(ADDRESS_LIST=)")
		al, err := extractAddressList(root)
		if err != nil {
			t.Fatalf("extractAddressList failed: %v", err)
		}
		if len(al.Addresses) != 0 {
			t.Errorf("expected 0 addresses, got %d", len(al.Addresses))
		}
	})

	t.Run("unknown child error", func(t *testing.T) {
		node := &Node{Name: "ADDRESS_LIST", Children: []Node{{Name: "UNKNOWN", Value: "bad"}}}
		if _, err := extractAddressList(node); err == nil {
			t.Error("expected error for unknown child")
		}
	})

	t.Run("address error propagate", func(t *testing.T) {
		root := mustParse(t, "(ADDRESS_LIST=(ADDRESS=(INVALID=bad)))")
		if _, err := extractAddressList(root); err == nil {
			t.Error("expected error from invalid ADDRESS")
		}
	})
}

/* DescriptionList */

func TestExtractDescriptionList_FullEmptyAndErrors(t *testing.T) {
	t.Parallel()
	t.Run("full", func(t *testing.T) {
		root := mustParse(t, "(DESCRIPTION_LIST=(LOAD_BALANCE=on)(FAILOVER=off)(SOURCE_ROUTE=yes)(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=s1))))")
		dl, err := extractDescriptionList(root)
		if err != nil {
			t.Fatalf("extractDescriptionList failed: %v", err)
		}
		if dl.LoadBalance != true || dl.Failover != false || dl.SourceRoute != true {
			t.Errorf("unexpected flags: %+v", dl)
		}
		if len(dl.Descriptions) != 1 {
			t.Errorf("expected 1 description, got %d", len(dl.Descriptions))
		}
	})

	t.Run("empty", func(t *testing.T) {
		root := mustParse(t, "(DESCRIPTION_LIST=)")
		dl, err := extractDescriptionList(root)
		if err != nil {
			t.Fatalf("extractDescriptionList failed: %v", err)
		}
		if len(dl.Descriptions) != 0 {
			t.Errorf("expected 0 Descriptions, got %d", len(dl.Descriptions))
		}
	})

	t.Run("unknown child error", func(t *testing.T) {
		node := &Node{Name: "DESCRIPTION_LIST", Children: []Node{{Name: "UNKNOWN", Value: "bad"}}}
		if _, err := extractDescriptionList(node); err == nil {
			t.Error("expected error for unknown child")
		}
	})

	t.Run("description child error", func(t *testing.T) {
		root := mustParse(t, "(DESCRIPTION_LIST=(DESCRIPTION=(INVALID=bad)))")
		if _, err := extractDescriptionList(root); err == nil {
			t.Error("expected error for invalid DESCRIPTION within DESCRIPTION_LIST")
		}
	})
}

/* Address */

func TestExtractAddress_SuccessAndError(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		root := mustParse(t, "(ADDRESS=(PROTOCOL=TCP)(HOST=localhost)(PORT=1521))")
		addr, err := extractAddress(root)
		if err != nil {
			t.Fatalf("extractAddress failed: %v", err)
		}
		if addr.Protocol != common.ProtocolTCP || addr.Host != "localhost" || addr.Port != 1521 {
			t.Errorf("unexpected address: %+v", addr)
		}
	})
	t.Run("error", func(t *testing.T) {
		node := &Node{Name: "ADDRESS", Children: []Node{{Name: "INVALID", Value: "bad"}}}
		if _, err := extractAddress(node); err == nil {
			t.Error("expected error for unknown param in ADDRESS")
		}
	})
}

/* ConnectData */

func TestExtractConnectData_SuccessEmptyAndError(t *testing.T) {
	t.Parallel()
	t.Run("success all fields", func(t *testing.T) {
		root := mustParse(t, "(CONNECT_DATA=(SERVICE_NAME=svc)(SID=sid1)(SERVER=DEDICATED)(INSTANCE_NAME=inst1)(FAILOVER_MODE=select)(HS=ok)(RDB_DATABASE=db1)(GLOBAL_NAME=global)(CONNECTION_ID_PREFIX=prefix))")
		cd, err := extractConnectData(root)
		if err != nil {
			t.Fatalf("extractConnectData failed: %v", err)
		}
		if cd.ServiceName != "svc" || cd.SID != "sid1" || cd.Server != "DEDICATED" || cd.InstanceName != "inst1" {
			t.Errorf("unexpected connect data: %+v", cd)
		}
	})
	t.Run("empty ok", func(t *testing.T) {
		node := &Node{Name: "CONNECT_DATA"}
		cd, err := extractConnectData(node)
		if err != nil {
			t.Fatalf("unexpected error on empty CONNECT_DATA: %v", err)
		}
		if cd == nil {
			t.Fatal("expected non-nil ConnectData")
		}
	})
	t.Run("unknown child error", func(t *testing.T) {
		node := &Node{Name: "CONNECT_DATA", Children: []Node{{Name: "UNKNOWN", Value: "bad"}}}
		if _, err := extractConnectData(node); err == nil {
			t.Error("expected error for unknown param in CONNECT_DATA")
		}
	})
}

/* Security */

func TestExtractSecurity_SuccessEmptyAndError(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		root := mustParse(t, "(SECURITY=(SSL_SERVER_CERT_DN=cert)(SSL_SERVER_DN_MATCH=yes)(SSL_ALLOW_WEAK_DN_MATCH=no)(WALLET_LOCATION=/loc)(MY_WALLET_DIRECTORY=/dir))")
		sec, err := extractSecurity(root)
		if err != nil {
			t.Fatalf("extractSecurity failed: %v", err)
		}
		if sec.SSLServerCertDN != "cert" || sec.SSLServerDNMatch != true || sec.SSLAllowWeakDNMatch != false || sec.WalletLocation != "/loc" || sec.MyWalletDirectory != "/dir" {
			t.Errorf("unexpected security: %+v", sec)
		}
	})
	t.Run("empty ok", func(t *testing.T) {
		node := &Node{Name: "SECURITY"}
		sec, err := extractSecurity(node)
		if err != nil {
			t.Fatalf("unexpected error on empty SECURITY: %v", err)
		}
		if sec == nil {
			t.Fatal("expected non-nil Security")
		}
	})
	t.Run("unknown child error", func(t *testing.T) {
		node := &Node{Name: "SECURITY", Children: []Node{{Name: "UNKNOWN", Value: "bad"}}}
		if _, err := extractSecurity(node); err == nil {
			t.Error("expected error for unknown param in SECURITY")
		}
	})
}

/* Helpers and boolean/int normalization + struct helper methods */

func TestHelpersAndMethods(t *testing.T) {
	t.Parallel()
	t.Run("normalizeBoolean", func(t *testing.T) {
		tru := []string{"ON", "Yes ", "TRUE"}
		fal := []string{"off", "no", "false", ""}
		for _, v := range tru {
			v, _ := normalizeBoolean(v)
			if !v {
				t.Errorf("expected true for %t", v)
			}
		}
		for _, v := range fal {
			v, _ := normalizeBoolean(v)
			if v {
				t.Errorf("expected false for %t", v)
			}
		}

		// Invalid boolean values should return a common.SQLError with a stable error code.
		_, err := normalizeBoolean("maybe")
		if err == nil {
			t.Fatal("expected error for invalid boolean")
		}
		sqle, ok := err.(common.SQLError)
		if !ok {
			t.Fatalf("expected common.SQLError, got %T (%v)", err, err)
		}
		if sqle.ErrorCode() != string(common.InvalidConnectionParameter) {
			t.Fatalf("expected error code %s, got %s", common.InvalidConnectionParameter, sqle.ErrorCode())
		}
		if !strings.Contains(sqle.Error(), "InvalidConnectionParameter") && !strings.Contains(sqle.Error(), string(common.InvalidConnectionParameter)) {
			// keep this loose; message text is localized/subject to change, but should contain the code.
			t.Fatalf("expected error message to contain error code, got %q", sqle.Error())
		}
	})

	t.Run("parseIntField", func(t *testing.T) {
		_, err := parseIntField("invalid")
		if err == nil {
			t.Error("expected error for invalid")
		}
		var v int
		v, err = parseIntField("-5")
		if v != 0 {
			t.Error("expected 0 for negative")
		}

		_, err = parseIntField("")
		if err == nil {
			t.Error("expected error for empty")
		}
	})

	t.Run("Description flags", func(t *testing.T) {
		d := NewDescription()
		d.SourceRoute = true
		d.UseSNI = true
		d.Compression = true
		if !d.IsSourceRouteEnabled() || !d.IsUseSNIEnabled() || !d.IsCompressionEnabled() {
			t.Error("expected true for on/yes")
		}
		d.Compression = false
		if d.IsCompressionEnabled() {
			t.Error("expected false for compression off")
		}
	})

	t.Run("AddressList flags", func(t *testing.T) {
		al := NewAddressList()
		al.LoadBalance = true

		if !al.IsLoadBalanceEnabled() {
			t.Error("expected load balance true")
		}
		if !al.IsFailoverEnabled() {
			t.Error("expected default failover true for empty")
		}
		al.Failover = false
		if al.IsFailoverEnabled() {
			t.Error("expected failover false")
		}
	})

	t.Run("DescriptionList flags", func(t *testing.T) {
		dl := NewDescriptionList()
		dl.LoadBalance = true

		if !dl.IsLoadBalanceEnabled() {
			t.Error("expected load balance true")
		}
		if !dl.IsFailoverEnabled() {
			t.Error("expected default failover true for empty")
		}
		dl.Failover = false
		if dl.IsFailoverEnabled() {
			t.Error("expected failover false")
		}
	})

	t.Run("Description address helpers", func(t *testing.T) {
		d := &Description{}
		if d.HasAddressLists() {
			t.Error("expected no address lists")
		}
		if d.GetPrimaryAddressList() != nil {
			t.Error("expected nil primary list")
		}
		d = &Description{
			AddressLists: []AddressList{
				{Addresses: []Address{{Host: "h1"}}, LoadBalance: true},
				{Addresses: []Address{{Host: "h2"}}},
			},
			Addresses: []Address{{Host: "h3"}, {Host: "h4"}},
		}
		if !d.HasAddressLists() {
			t.Error("expected address lists")
		}
		if p := d.GetPrimaryAddressList(); p == nil || p.LoadBalance != true {
			t.Errorf("unexpected primary: %+v", p)
		}
		all := d.GetAllAddresses()
		if len(all) != 4 {
			t.Errorf("expected 4 addresses, got %d", len(all))
		}
	})
}

/* Direct function-level error coverage that bubbles from extractDescription */

func TestExtractDescription_ErrorPropagation(t *testing.T) {
	t.Parallel()
	// invalid SECURITY child within DESCRIPTION
	root := mustParse(t, "(DESCRIPTION=(SECURITY=(INVALID=bad)))")
	if _, err := extractDescription(root); err == nil {
		t.Error("expected error for invalid SECURITY")
	}

	// invalid CONNECT_DATA child within DESCRIPTION
	root = mustParse(t, "(DESCRIPTION=(CONNECT_DATA=(INVALID=bad)))")
	if _, err := extractDescription(root); err == nil {
		t.Error("expected error for invalid CONNECT_DATA")
	}

	// invalid ADDRESS_LIST child within DESCRIPTION
	root = mustParse(t, "(DESCRIPTION=(ADDRESS_LIST=(INVALID=bad)))")
	if _, err := extractDescription(root); err == nil {
		t.Error("expected error for invalid ADDRESS_LIST")
	}

	// invalid ADDRESS child within DESCRIPTION
	root = mustParse(t, "(DESCRIPTION=(ADDRESS=(INVALID=bad)))")
	if _, err := extractDescription(root); err == nil {
		t.Error("expected error for invalid ADDRESS")
	}

	// unsupported parameter in DESCRIPTION
	node := &Node{
		Name: "DESCRIPTION",
		Children: []Node{
			{Name: "UNKNOWN", Value: "bad"},
		},
	}
	if _, err := extractDescription(node); err == nil {
		t.Error("expected error for unsupported parameter in DESCRIPTION")
	}
}

func TestCoverage_MissingFlags(t *testing.T) {
	t.Parallel()
	t.Run("AddressList source route", func(t *testing.T) {
		al := NewAddressList()
		al.SourceRoute = true
		if !al.IsSourceRouteEnabled() {
			t.Error("expected source route true")
		}
		al.SourceRoute = false
		if al.IsSourceRouteEnabled() {
			t.Error("expected source route false")
		}
	})

	t.Run("DescriptionList source route", func(t *testing.T) {
		dl := NewDescriptionList()
		dl.SourceRoute = true
		if !dl.IsSourceRouteEnabled() {
			t.Error("expected source route true")
		}
		dl.SourceRoute = false
		if dl.IsSourceRouteEnabled() {
			t.Error("expected source route false")
		}
	})

	t.Run("Security methods", func(t *testing.T) {
		s := NewSecurity()
		s.SSLServerDNMatch = true
		s.SSLAllowWeakDNMatch = true
		if !s.IsSSLServerDNMatchEnabled() {
			t.Error("expected SSLServerDNMatch true")
		}
		if !s.IsSSLAllowWeakDNMatchEnabled() {
			t.Error("expected SSLAllowWeakDNMatch true")
		}
		s.SSLServerDNMatch = false
		s.SSLAllowWeakDNMatch = false
		if s.IsSSLServerDNMatchEnabled() {
			t.Error("expected SSLServerDNMatch false")
		}
		if s.IsSSLAllowWeakDNMatchEnabled() {
			t.Error("expected SSLAllowWeakDNMatch false")
		}
	})
}
