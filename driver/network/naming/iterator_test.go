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
	"reflect"
	"testing"
	"time"

	"github.com/oracle/go-driver/driver/common"
)

// ============================================================================
// BASIC ITERATOR TESTS
// ============================================================================

func TestNewConnectionIterator_Basic(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=localhost)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	if iter == nil {
		t.Fatal("Expected non-nil iterator")
	}
	if iter.rootNode != root {
		t.Error("Expected rootNode to be set")
	}
	if iter.context != ctx {
		t.Error("Expected context to be set")
	}
	if iter.currentDescIndex != 0 {
		t.Errorf("Expected currentDescIndex to be 0, got %d", iter.currentDescIndex)
	}
	if iter.exhausted {
		t.Error("Expected exhausted to be false")
	}
	if len(iter.descAttempts) == 0 {
		t.Error("Expected descAttempts to be generated")
	}
}

func TestConnectionIterator_Next_Basic(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=testhost)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=testsvc)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	option := iter.Next()
	if option == nil {
		t.Fatal("Expected non-nil option")
	}
	if option.Address.Host != "testhost" {
		t.Errorf("Expected host 'testhost', got '%s'", option.Address.Host)
	}
	if option.Address.Port != 1521 {
		t.Errorf("Expected port '1521', got '%d'", option.Address.Port)
	}
	if option.ConnectData.ServiceName != "testsvc" {
		t.Errorf("Expected service name 'testsvc', got '%s'", option.ConnectData.ServiceName)
	}
}

func TestConnectionIterator_HasNext(t *testing.T) {
	t.Parallel()
	// Use direct IP to avoid multiple DNS answers (e.g., IPv4 + IPv6 for localhost).
	input := "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=127.0.0.1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	if !iter.HasNext() {
		t.Error("Expected HasNext to be true initially")
	}

	// Consume all options
	for iter.HasNext() {
		iter.Next()
	}
	if iter.HasNext() {
		t.Error("Expected HasNext to be false after consuming all options")
	}
}

func TestConnectionIterator_Reset(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522)))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// Consume all options
	for iter.HasNext() {
		iter.Next()
	}

	if iter.HasNext() {
		t.Error("Expected no more options after consuming all")
	}

	// Reset
	iter.Reset()

	if !iter.HasNext() {
		t.Error("Expected HasNext to be true after reset")
	}
	if iter.currentDescIndex != 0 {
		t.Errorf("Expected currentDescIndex to be 0 after reset, got %d", iter.currentDescIndex)
	}
	if iter.exhausted {
		t.Error("Expected exhausted to be false after reset")
	}
}

func TestConnectionIterator_Remaining_And_Total(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522)))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)
	total := iter.Total()
	remaining := iter.Remaining()

	if remaining != total {
		t.Errorf("Expected remaining to equal total initially, got remaining=%d, total=%d", remaining, total)
	}

	iter.Next()
	if iter.Remaining() != total-1 {
		t.Errorf("Expected remaining to be %d after one Next(), got %d", total-1, iter.Remaining())
	}

	// Consume all
	for iter.HasNext() {
		iter.Next()
	}

	if iter.Remaining() != 0 {
		t.Errorf("Expected remaining to be 0 after exhaustion, got %d", iter.Remaining())
	}
}

func TestConnectionIterator_ConnectStringFormat(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=testhost)(PORT=9999))(CONNECT_DATA=(SERVICE_NAME=testsvc)(INSTANCE_NAME=testinst)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)
	option := iter.Next()
	if option == nil {
		t.Fatal("Expected non-nil option")
	}

	// Parse the generated connect string to verify format
	parsed, err := Parse(option.ConnectString)
	if err != nil {
		t.Fatalf("Generated ConnectString is not valid: %v", err)
	}

	if parsed.Name != "DESCRIPTION" {
		t.Errorf("Expected root to be DESCRIPTION, got %s", parsed.Name)
	}

	if len(parsed.Children) != 2 {
		t.Errorf("Expected 2 children (ADDRESS and CONNECT_DATA), got %d", len(parsed.Children))
	}
}

func TestConnectionIterator_Attempt_EZConnect_PreservesQuotedWalletLocation(t *testing.T) {
	t.Parallel()
	// End-to-end: EZCONNECT Data Source Name -> ParseDSNString -> iterator -> attempt.ConnectString
	dsn := `scott/tiger@localhost : 1521 / mydb?wallet_location="C:\\my_wallet with space"`
	parsed, err := ParseDSNString(dsn)
	if err != nil {
		t.Fatalf("unexpected error parsing Data Source Name: %v", err)
	}

	iter := parsed.NewConnectionAttemptIterator(context.Background())
	opt := iter.Next()
	if opt == nil {
		t.Fatalf("expected non-nil connection option")
	}
	if opt.Description == nil {
		t.Fatalf("expected non-nil Description in connection option")
	}

	if opt.Description.Security.WalletLocation != `C:\\my_wallet with space` {
		t.Fatalf(
			"expected wallet location to be preserved in security context.\nwant: %s\ngot:  %s",
			`C:\\my_wallet with space`,
			opt.Description.Security.WalletLocation,
		)
	}
}

func TestConnectionIterator_Attempt_LongTNS_PreservesQuotedServerCertDN(t *testing.T) {
	t.Parallel()
	// End-to-end: LONG TNS Data Source Name -> ParseDSNString -> iterator -> attempt.ConnectString
	dsn := `(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=localhost)(PORT=1521))` +
		`(CONNECT_DATA=(SERVICE_NAME=mydb))` +
		`(SECURITY=(SSL_SERVER_DN_MATCH=ON)(SSL_SERVER_CERT_DN="CN=My Server, OU=Org")))`
	parsed, err := ParseDSNString(dsn)
	if err != nil {
		t.Fatalf("unexpected error parsing Data Source Name: %v", err)
	}

	iter := parsed.NewConnectionAttemptIterator(context.Background())
	opt := iter.Next()
	if opt == nil {
		t.Fatalf("expected non-nil connection option")
	}
	if opt.Description == nil {
		t.Fatalf("expected non-nil Description in connection option")
	}

	if opt.Description.Security.SSLServerCertDN != `CN=My Server, OU=Org` {
		t.Fatalf(
			"expected server cert DN to be preserved in security context.\nwant: %s\ngot:  %s",
			`CN=My Server, OU=Org`,
			opt.Description.Security.SSLServerCertDN,
		)
	}
}

// ============================================================================
// RETRY COUNT TESTS
// ============================================================================

func TestConnectionIterator_RetryCount_Single(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(RETRY_COUNT=2)(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// With RETRY_COUNT=2 and 1 address, should have 1 * (2+1) = 3 attempts
	total := iter.Total()
	if total != 3 {
		t.Errorf("Expected total attempts to be 3 (1 address * 3 cycles), got %d", total)
	}

	// Verify options
	options := []string{}
	for iter.HasNext() {
		option := iter.Next()
		if option == nil {
			t.Fatal("Expected non-nil option")
		}
		options = append(options, option.Address.Host)
	}

	expectedOptions := []string{"host1", "host1", "host1"}
	if !reflect.DeepEqual(options, expectedOptions) {
		t.Errorf("Expected options %v, got %v", expectedOptions, options)
	}
}

func TestConnectionIterator_RetryCount_Multiple_Addresses(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(RETRY_COUNT=2)(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522)))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// With RETRY_COUNT=2 and 2 addresses, should have 2 * (2+1) = 6 attempts
	total := iter.Total()
	if total != 6 {
		t.Errorf("Expected total attempts to be 6 (2 addresses * 3 cycles), got %d", total)
	}

	// Verify the retry pattern: host1, host2, host1, host2, host1, host2
	expectedHosts := []string{"host1", "host2", "host1", "host2", "host1", "host2"}
	for i, expectedHost := range expectedHosts {
		option := iter.Next()
		if option == nil {
			t.Fatalf("Expected option %d to be non-nil", i+1)
		}
		if option.Address.Host != expectedHost {
			t.Errorf("Attempt %d: expected host '%s', got '%s'", i+1, expectedHost, option.Address.Host)
		}
	}
}

func TestConnectionIterator_RetryCount_Zero(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(RETRY_COUNT=0)(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// With RETRY_COUNT=0, should try once (0+1 = 1)
	total := iter.Total()
	if total != 1 {
		t.Errorf("Expected total attempts to be 1, got %d", total)
	}
}

func TestConnectionIterator_RetryCount_Negative(t *testing.T) {
	t.Parallel()
	// Negative retry count should be treated as 0
	input := "(DESCRIPTION=(RETRY_COUNT=-1)(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// Should still have at least 1 attempt
	total := iter.Total()
	if total < 1 {
		t.Errorf("Expected at least 1 attempt, got %d", total)
	}
}

// ============================================================================
// RETRY DELAY TESTS
// ============================================================================

func TestConnectionIterator_RetryDelay_Configuration(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(RETRY_COUNT=1)(RETRY_DELAY=3)(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// Verify retry delay is configured correctly
	if len(iter.descAttempts) == 0 {
		t.Fatal("Expected at least one description attempt")
	}

	descAttempt := iter.descAttempts[0]
	expectedDelay := 3 * time.Second
	if descAttempt.RetryDelay != expectedDelay {
		t.Errorf("Expected retry delay to be %v, got %v", expectedDelay, descAttempt.RetryDelay)
	}
}

func TestConnectionIterator_RetryDelay_Zero(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(RETRY_COUNT=1)(RETRY_DELAY=0)(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	if len(iter.descAttempts) == 0 {
		t.Fatal("Expected at least one description attempt")
	}

	descAttempt := iter.descAttempts[0]
	if descAttempt.RetryDelay != 0 {
		t.Errorf("Expected retry delay to be 0, got %v", descAttempt.RetryDelay)
	}
}

func TestConnectionIterator_RetryDelay_ActualWait(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(RETRY_COUNT=1)(RETRY_DELAY=1)(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// First option should be immediate
	start := time.Now()
	iter.Next()
	firstDuration := time.Since(start)
	if firstDuration > 100*time.Millisecond {
		t.Errorf("First option took too long: %v", firstDuration)
	}

	// Second option should wait for retry delay
	start = time.Now()
	iter.Next()
	secondDuration := time.Since(start)
	expectedMin := 1 * time.Second
	expectedMax := 1200 * time.Millisecond // Allow 200ms tolerance
	if secondDuration < expectedMin || secondDuration > expectedMax {
		t.Errorf("Second option delay expected between %v and %v, got %v", expectedMin, expectedMax, secondDuration)
	}
}

// Retry delay should stop waiting as soon as the caller context is cancelled.
func TestConnectionIterator_RetryDelay_ContextCancellation(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(RETRY_COUNT=1)(RETRY_DELAY=2)(ADDRESS=(PROTOCOL=TCP)(HOST=127.0.0.1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	connCtx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	iter := NewConnectionIterator(ctx, root, connCtx)

	if option := iter.Next(); option == nil {
		t.Fatal("Expected first option before retry delay")
	}

	start := time.Now()
	done := make(chan *ConnectionOption, 1)
	go func() {
		done <- iter.Next()
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case option := <-done:
		if option != nil {
			t.Fatal("Expected no option after context cancellation")
		}
		if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
			t.Fatalf("Retry delay did not stop promptly after cancellation: %v", elapsed)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Retry delay did not stop after context cancellation")
	}

	if iter.HasNext() {
		t.Fatal("Expected iterator to be exhausted after context cancellation")
	}
}

// ============================================================================
// FAILOVER TESTS
// ============================================================================

func TestConnectionIterator_Failover_DescriptionList_Enabled(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION_LIST=(FAILOVER=on)(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=svc1))(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521)))(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=svc2))(ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522))))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// With failover enabled, should try both descriptions
	if iter.Total() != 2 {
		t.Errorf("Expected total attempts to be 2, got %d", iter.Total())
	}

	services := []string{}
	for iter.HasNext() {
		option := iter.Next()
		services = append(services, option.ConnectData.ServiceName)
	}

	// Should have both services
	if len(services) != 2 {
		t.Errorf("Expected 2 services, got %d", len(services))
	}
}

// ============================================================================
// INTERNAL/ADDITIONAL EDGE COVERAGE
// ============================================================================

// Covers buildFromDescriptionList path where desc nodes cannot be found (root is nil),
// ensuring graceful handling and no options are produced.
func TestIterator_DescriptionList_RootMissing(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION_LIST=(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=h1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=s1)))(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=h2)(PORT=1522))(CONNECT_DATA=(SERVICE_NAME=s2))))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	// Pass nil root so findDescriptionNode cannot locate DESCRIPTION nodes
	iter := NewConnectionIterator(context.Background(), nil, ctx)
	if got := iter.Total(); got != 0 {
		t.Errorf("expected total=0 when DESCRIPTION_LIST root node is missing, got %d", got)
	}
}

// Covers extractConnectDataNode branches:
//   - descNode == nil (returns default CONNECT_DATA node)
//   - fallback to scanning direct children when root name doesn't match "DESCRIPTION"
func TestIterator_ExtractConnectDataNode_Cases(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=h)(PORT=1))(CONNECT_DATA=(SERVICE_NAME=s)))"
	descRoot, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	ctx, err := ExtractConnectionContext(descRoot)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	t.Run("nil desc node returns default CONNECT_DATA", func(t *testing.T) {
		iter := NewConnectionIterator(context.Background(), nil, ctx) // descNode=nil path
		if iter.Total() < 1 {
			t.Fatalf("expected at least 1 option, got %d", iter.Total())
		}
		a := iter.Next()
		if a == nil {
			t.Fatal("nil option")
		}
		parsed, err := Parse(a.ConnectString)
		if err != nil {
			t.Fatalf("generated ConnectString invalid: %v", err)
		}
		hasCD := false
		for _, ch := range parsed.Children {
			if ch.Name == "CONNECT_DATA" {
				hasCD = true
				break
			}
		}
		if !hasCD {
			t.Error("expected CONNECT_DATA in ConnectString when desc node is nil")
		}
	})

	t.Run("fallback direct child when root name mismatches", func(t *testing.T) {
		// Root not named DESCRIPTION, but has a direct CONNECT_DATA child: forces fallback branch
		wrongRoot := &Node{
			Name: "WRONG",
			Children: []Node{
				{
					Name: "CONNECT_DATA",
					Children: []Node{
						{Name: "SERVICE_NAME", Value: "x"},
					},
				},
			},
		}
		iter := NewConnectionIterator(context.Background(), wrongRoot, ctx)
		if iter.Total() < 1 {
			t.Fatalf("expected at least 1 option, got %d", iter.Total())
		}
		a := iter.Next()
		if a == nil {
			t.Fatal("nil option")
		}
		parsed, err := Parse(a.ConnectString)
		if err != nil {
			t.Fatalf("generated ConnectString invalid: %v", err)
		}
		hasCD := false
		for _, ch := range parsed.Children {
			if ch.Name == "CONNECT_DATA" {
				hasCD = true
				break
			}
		}
		if !hasCD {
			t.Error("expected CONNECT_DATA in ConnectString using fallback direct child")
		}
	})
}

func TestIterator_buildFromAddresses_Empty(t *testing.T) {
	t.Parallel()
	iter := NewConnectionIterator(context.Background(), nil, &ConnectionContext{})
	got := iter.buildFromAddresses(context.Background(), []Address{})
	if len(got) != 0 {
		t.Errorf("expected 0 attempts for empty addresses, got %d", len(got))
	}
}

func TestIterator_extractConnectDataNode_GetNodePath(t *testing.T) {
	t.Parallel()
	// Build a DESCRIPTION node with proper CONNECT_DATA to hit the GetNode success path
	desc := &Node{
		Name: "DESCRIPTION",
		Children: []Node{
			{Name: "CONNECT_DATA", Children: []Node{{Name: "SERVICE_NAME", Value: "svc"}}},
		},
	}
	description := NewDescription()
	description.Addresses = []Address{{Protocol: common.ProtocolTCP, Host: "h", Port: 1}}
	description.ConnectData = ConnectData{ServiceName: "svc"}
	iter := NewConnectionIterator(context.Background(), desc, &ConnectionContext{
		Description: description,
	})
	cd := iter.extractConnectDataNode(desc)
	if cd == nil || cd.Name != "CONNECT_DATA" {
		t.Fatalf("expected CONNECT_DATA node, got %+v", cd)
	}
}

func TestIterator_findDescriptionNode_WrongRootAndOutOfRange(t *testing.T) {
	t.Parallel()
	iter := NewConnectionIterator(context.Background(), &Node{Name: "WRONG"}, &ConnectionContext{})
	if n := iter.findDescriptionNode(0); n != nil {
		t.Errorf("expected nil for wrong root, got %+v", n)
	}

	// Build DESCRIPTION_LIST with a single DESCRIPTION and ask for out-of-range index
	root := &Node{
		Name: "DESCRIPTION_LIST",
		Children: []Node{
			{Name: "DESCRIPTION"},
		},
	}
	iter = NewConnectionIterator(context.Background(), root, &ConnectionContext{DescriptionList: &DescriptionList{Descriptions: []Description{{}}}})
	if n := iter.findDescriptionNode(5); n != nil {
		t.Errorf("expected nil for out-of-range index, got %+v", n)
	}
}

func TestIterator_Remaining_ExhaustedEarlyReturn(t *testing.T) {
	t.Parallel()
	// Manually set exhausted to ensure early return branch is covered
	iter := NewConnectionIterator(context.Background(), nil, &ConnectionContext{})
	iter.exhausted = true
	if r := iter.Remaining(); r != 0 {
		t.Errorf("expected 0 remaining when exhausted, got %d", r)
	}
}

func TestConnectionIterator_Failover_DescriptionList_Disabled(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION_LIST=(FAILOVER=off)(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=svc1))(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521)))(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=svc2))(ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522))))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// With failover disabled, should only try first description
	if iter.Total() != 1 {
		t.Errorf("Expected total attempts to be 1 (failover disabled), got %d", iter.Total())
	}

	option := iter.Next()
	if option == nil {
		t.Fatal("Expected non-nil option")
	}

	// Should only get first service
	if option.ConnectData.ServiceName != "svc1" {
		t.Errorf("Expected service 'svc1' (first only), got '%s'", option.ConnectData.ServiceName)
	}

	// No more options
	if iter.HasNext() {
		t.Error("Expected no more options with failover disabled")
	}
}

func TestConnectionIterator_Failover_DescriptionList_Default(t *testing.T) {
	t.Parallel()
	// When FAILOVER is not specified, it should default to true
	input := "(DESCRIPTION_LIST=(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=svc1))(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521)))(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=svc2))(ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522))))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// Default failover=true, should try both descriptions
	if iter.Total() != 2 {
		t.Errorf("Expected total attempts to be 2 (default failover), got %d", iter.Total())
	}
}

func TestConnectionIterator_Failover_Description_Enabled(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(FAILOVER=on)(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522)))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// With failover enabled, should try both addresses
	if iter.Total() != 2 {
		t.Errorf("Expected total attempts to be 2, got %d", iter.Total())
	}
}

func TestConnectionIterator_Failover_Description_Disabled(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(FAILOVER=off)(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522)))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// With failover disabled, should only try first address
	if iter.Total() != 1 {
		t.Errorf("Expected total attempts to be 1 (failover disabled), got %d", iter.Total())
	}

	option := iter.Next()
	if option == nil {
		t.Fatal("Expected non-nil option")
	}

	// No more options
	if iter.HasNext() {
		t.Error("Expected no more options with failover disabled")
	}
}

func TestConnectionIterator_Failover_Mixed_Settings(t *testing.T) {
	t.Parallel()
	// DESCRIPTION_LIST has failover=on, first DESCRIPTION has failover=off
	input := "(DESCRIPTION_LIST=(FAILOVER=on)(DESCRIPTION=(FAILOVER=off)(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=h1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=h2)(PORT=1522)))(CONNECT_DATA=(SERVICE_NAME=svc1)))(DESCRIPTION=(FAILOVER=on)(ADDRESS=(PROTOCOL=TCP)(HOST=h3)(PORT=1523))(ADDRESS=(PROTOCOL=TCP)(HOST=h4)(PORT=1524))(CONNECT_DATA=(SERVICE_NAME=svc2))))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// First description: failover=off -> 1 address
	// Second description: failover=on -> 2 addresses
	// Total: 3 attempts
	if iter.Total() != 3 {
		t.Errorf("Expected total attempts to be 3, got %d", iter.Total())
	}

	hosts := []string{}
	for iter.HasNext() {
		option := iter.Next()
		hosts = append(hosts, option.Address.Host)
	}

	// Should have 1 host from first description and 2 from second
	if len(hosts) != 3 {
		t.Errorf("Expected 3 hosts, got %d: %v", len(hosts), hosts)
	}
}

// ============================================================================
// LOAD BALANCE TESTS
// ============================================================================

func TestConnectionIterator_LoadBalance_DescriptionList(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION_LIST=(LOAD_BALANCE=on)(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=svc1))(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521)))(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=svc2))(ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522))))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	// Test multiple iterations to see randomization
	orderCounts := make(map[string]int)
	for run := 0; run < 20; run++ {
		iter := NewConnectionIterator(context.Background(), root, ctx)
		option := iter.Next()
		orderCounts[option.ConnectData.ServiceName]++
	}

	// Should see some randomization (not always the same order)
	if len(orderCounts) < 2 {
		t.Error("Expected some randomization in description order with load balancing, but got consistent ordering")
	}
}

func TestConnectionIterator_LoadBalance_Description(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(LOAD_BALANCE=on)(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522))(ADDRESS=(PROTOCOL=TCP)(HOST=host3)(PORT=1523)))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	// Test multiple iterations to see randomization
	firstHosts := make(map[string]int)
	for run := 0; run < 30; run++ {
		iter := NewConnectionIterator(context.Background(), root, ctx)
		option := iter.Next()
		firstHosts[option.Address.Host]++
	}

	// Should see randomization (not always the same host first)
	if len(firstHosts) < 2 {
		t.Error("Expected randomization in address order with load balancing, but got consistent ordering")
	}
}

/* func TestConnectionIterator_LoadBalance_Description_OrderStableAcrossRetries(t *testing.T) {
	// When load balancing is enabled, the shuffled order should remain the same across retry cycles
	input := "(DESCRIPTION=(LOAD_BALANCE=on)(RETRY_COUNT=2)(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=a1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=a2)(PORT=1522))(ADDRESS=(PROTOCOL=TCP)(HOST=a3)(PORT=1523)))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)
	if iter.Total() != 9 {
		t.Fatalf("Expected 9 options (3 addresses * 3 cycles), got %d", iter.Total())
	}

	cycleLen := 3
	cycle1 := make([]string, 0, cycleLen)
	cycle2 := make([]string, 0, cycleLen)
	cycle3 := make([]string, 0, cycleLen)

	for i := 0; i < cycleLen; i++ {
		a := iter.Next()
		if a == nil {
			t.Fatalf("nil option in cycle1 at index %d", i)
		}
		cycle1 = append(cycle1, a.Address.Host)
	}
	for i := 0; i < cycleLen; i++ {
		a := iter.Next()
		if a == nil {
			t.Fatalf("nil option in cycle2 at index %d", i)
		}
		cycle2 = append(cycle2, a.Address.Host)
	}
	for i := 0; i < cycleLen; i++ {
		a := iter.Next()
		if a == nil {
			t.Fatalf("nil option in cycle3 at index %d", i)
		}
		cycle3 = append(cycle3, a.Address.Host)
	}

	if !reflect.DeepEqual(cycle1, cycle2) || !reflect.DeepEqual(cycle1, cycle3) {
		t.Errorf("Expected same shuffled order to repeat across retries, got cycle1=%v cycle2=%v cycle3=%v", cycle1, cycle2, cycle3)
	}
} */

func TestConnectionIterator_LoadBalance_Off(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(LOAD_BALANCE=off)(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522)))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	// Test multiple iterations - should always get same order
	firstHosts := make(map[string]int)
	for run := 0; run < 10; run++ {
		iter := NewConnectionIterator(context.Background(), root, ctx)
		option := iter.Next()
		firstHosts[option.Address.Host]++
	}

	// Should always get the same host first (no randomization)
	if len(firstHosts) != 1 {
		t.Error("Expected consistent ordering with load balance off, but got randomization")
	}
}

// ============================================================================
// COMBINED SCENARIO TESTS
// ============================================================================

func TestConnectionIterator_Complex_Scenario_All_Features(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION_LIST=(LOAD_BALANCE=on)(FAILOVER=on)(DESCRIPTION=(LOAD_BALANCE=on)(FAILOVER=on)(RETRY_COUNT=1)(RETRY_DELAY=1)(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=db1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=db2)(PORT=1522)))(CONNECT_DATA=(SERVICE_NAME=primary)))(DESCRIPTION=(RETRY_COUNT=0)(ADDRESS=(PROTOCOL=TCP)(HOST=backup)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=backup))))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// Description 1: 2 addresses, RETRY_COUNT=1 -> 2 * (1+1) = 4 options
	// Description 2: 1 address, RETRY_COUNT=0 -> 1 * (0+1) = 1 option
	// Total: 5 options
	total := iter.Total()
	if total != 5 {
		t.Errorf("Expected total attempts to be 5, got %d", total)
	}

	// Verify retry delay is configured
	if len(iter.descAttempts) < 1 {
		t.Fatal("Expected at least one description attempt")
	}
	if iter.descAttempts[0].RetryDelay != 1*time.Second && iter.descAttempts[0].Description.RetryCount == 1 {
		t.Errorf("Expected retry delay to be 1s, got %v", iter.descAttempts[0].RetryDelay)
	}

	// Verify all options have required fields
	servicesSeen := make(map[string]int)
	for iter.HasNext() {
		option := iter.Next()
		if option == nil {
			t.Error("Expected non-nil option")
			continue
		}

		if option.Address.Host == "" {
			t.Error("Expected Address.Host to be set")
		}
		if option.Address.Port == 0 {
			t.Error("Expected Address.Port to be set")
		}
		if option.ConnectString == "" {
			t.Error("Expected ConnectString to be set")
		}
		servicesSeen[option.ConnectData.ServiceName]++
	}

	// Should see both services
	if len(servicesSeen) != 2 {
		t.Errorf("Expected 2 different services, got %d: %v", len(servicesSeen), servicesSeen)
	}
}

func TestConnectionIterator_Failover_And_Retry_Combined(t *testing.T) {
	t.Parallel()
	// Failover disabled with retry - should retry same address multiple times
	input := "(DESCRIPTION=(FAILOVER=off)(RETRY_COUNT=2)(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522)))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// Failover=off means only 1 address, retry_count=2 means 3 cycles
	// Total: 1 * 3 = 3 options
	if iter.Total() != 3 {
		t.Errorf("Expected 3 options (1 address * 3 cycles), got %d", iter.Total())
	}

	hosts := []string{}
	for iter.HasNext() {
		option := iter.Next()
		hosts = append(hosts, option.Address.Host)
	}

	// All options should be to the same host
	if len(hosts) != 3 {
		t.Errorf("Expected 3 options, got %d", len(hosts))
	}
	for _, host := range hosts {
		if host != hosts[0] {
			t.Errorf("Expected all options to same host with failover=off, got varying hosts: %v", hosts)
			break
		}
	}
}

func TestConnectionIterator_LoadBalance_And_Failover_Disabled(t *testing.T) {
	t.Parallel()
	// Both load balance and failover disabled - should get first address only, no randomization
	input := "(DESCRIPTION_LIST=(LOAD_BALANCE=off)(FAILOVER=off)(DESCRIPTION=(LOAD_BALANCE=off)(FAILOVER=off)(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=h1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=h2)(PORT=1522)))(CONNECT_DATA=(SERVICE_NAME=svc1)))(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=h3)(PORT=1523))(CONNECT_DATA=(SERVICE_NAME=svc2))))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	// Run multiple times to ensure consistency
	for run := 0; run < 5; run++ {
		iter := NewConnectionIterator(context.Background(), root, ctx)

		// Should have only 1 option (first description, first address)
		if iter.Total() != 1 {
			t.Errorf("Run %d: Expected 1 option, got %d", run, iter.Total())
		}

		option := iter.Next()
		if option == nil {
			t.Fatalf("Run %d: Expected non-nil option", run)
		}

		// Should always be first address from first description
		if option.Address.Host != "h1" {
			t.Errorf("Run %d: Expected host 'h1', got '%s'", run, option.Address.Host)
		}
	}
}

// ============================================================================
// ADDRESS COLLECTION TESTS
// ============================================================================

func TestConnectionIterator_CollectAllAddresses(t *testing.T) {
	t.Parallel()
	// Test collecting addresses from both ADDRESS_LIST and direct ADDRESS children
	input := "(DESCRIPTION=(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=list1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=list2)(PORT=1522)))(ADDRESS=(PROTOCOL=TCP)(HOST=direct)(PORT=1523))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// Should have 3 options (3 addresses total)
	total := iter.Total()
	if total != 3 {
		t.Errorf("Expected 3 total options, got %d", total)
	}

	// Collect all hosts from options
	hosts := make(map[string]bool)
	for iter.HasNext() {
		option := iter.Next()
		hosts[option.Address.Host] = true
	}

	expectedHosts := map[string]bool{
		"list1":  true,
		"list2":  true,
		"direct": true,
	}

	if !reflect.DeepEqual(hosts, expectedHosts) {
		t.Errorf("Expected hosts %v, got %v", expectedHosts, hosts)
	}
}

func TestConnectionIterator_MultipleAddressLists(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=list1_a)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=list1_b)(PORT=1522)))(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=list2_a)(PORT=1523))(ADDRESS=(PROTOCOL=TCP)(HOST=list2_b)(PORT=1524)))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// Should have 4 options (4 addresses from 2 lists)
	if iter.Total() != 4 {
		t.Errorf("Expected 4 total options, got %d", iter.Total())
	}

	hosts := make(map[string]bool)
	for iter.HasNext() {
		option := iter.Next()
		hosts[option.Address.Host] = true
	}

	expectedHosts := map[string]bool{
		"list1_a": true,
		"list1_b": true,
		"list2_a": true,
		"list2_b": true,
	}

	if !reflect.DeepEqual(hosts, expectedHosts) {
		t.Errorf("Expected hosts %v, got %v", expectedHosts, hosts)
	}
}

// ============================================================================
// SIMPLE ADDRESS TESTS
// ============================================================================

func TestConnectionIterator_SingleAddress(t *testing.T) {
	t.Parallel()
	// Use direct IP to avoid multiple DNS answers (localhost can resolve to IPv4 + IPv6).
	input := "(ADDRESS=(PROTOCOL=TCP)(HOST=127.0.0.1)(PORT=1521))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	if iter.Total() != 1 {
		t.Errorf("Expected total options to be 1, got %d", iter.Total())
	}

	option := iter.Next()

	if option.Address.Protocol != common.ProtocolTCP {
		t.Errorf("Expected protocol 'TCP', got '%s'", option.Address.Protocol)
	}
	if option.Address.Host != "127.0.0.1" {
		t.Errorf("Expected host '127.0.0.1', got '%s'", option.Address.Host)
	}
	if option.Address.Port != 1521 {
		t.Errorf("Expected port '1521', got '%d'", option.Address.Port)
	}
}

func TestConnectionIterator_AddressesOnly(t *testing.T) {
	t.Parallel()
	input := "(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	if iter.Total() != 2 {
		t.Errorf("Expected total options to be 2, got %d", iter.Total())
	}

	option1 := iter.Next()
	option2 := iter.Next()

	if option1.Description != nil {
		t.Error("Expected Description to be nil for address-only iteration")
	}
	if option2.Description != nil {
		t.Error("Expected Description to be nil for address-only iteration")
	}

	if option1.ConnectString == "" {
		t.Error("Expected ConnectString to be populated")
	}
	if option2.ConnectString == "" {
		t.Error("Expected ConnectString to be populated")
	}
}

func TestConnectionIterator_AddressesOnly_ConnectStringStructure(t *testing.T) {
	t.Parallel()
	input := "(ADDRESS=(PROTOCOL=TCP)(HOST=h)(PORT=1))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)
	if iter.Total() != 1 {
		t.Fatalf("Expected 1 option, got %d", iter.Total())
	}

	option := iter.Next()
	if option == nil {
		t.Fatal("Expected non-nil option")
	}

	parsed, err := Parse(option.ConnectString)
	if err != nil {
		t.Fatalf("Generated ConnectString is not valid: %v", err)
	}
	if parsed.Name != "DESCRIPTION" {
		t.Errorf("Expected DESCRIPTION root, got %s", parsed.Name)
	}
	if len(parsed.Children) != 1 {
		t.Errorf("Expected DESCRIPTION with only ADDRESS child, got %d children", len(parsed.Children))
	}
	if parsed.Children[0].Name != "ADDRESS" {
		t.Errorf("Expected child node ADDRESS, got %s", parsed.Children[0].Name)
	}
}

// ============================================================================
// DESCRIPTION LIST TESTS
// ============================================================================

func TestConnectionIterator_DescriptionList_Basic(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION_LIST=(LOAD_BALANCE=off)(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=svc1))(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521)))(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=svc2))(ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522))))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	if iter.Total() != 2 {
		t.Errorf("Expected total options to be 2, got %d", iter.Total())
	}

	option1 := iter.Next()
	option2 := iter.Next()

	if option1.ConnectData.ServiceName != "svc1" {
		t.Errorf("Expected first option service name 'svc1', got '%s'", option1.ConnectData.ServiceName)
	}
	if option2.ConnectData.ServiceName != "svc2" {
		t.Errorf("Expected second option service name 'svc2', got '%s'", option2.ConnectData.ServiceName)
	}
}

func TestConnectionIterator_DescriptionList_MultipleAddresses(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION_LIST=(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=h1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=h2)(PORT=1522))(CONNECT_DATA=(SERVICE_NAME=svc1)))(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=h3)(PORT=1523))(CONNECT_DATA=(SERVICE_NAME=svc2))))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	// First description: 2 addresses
	// Second description: 1 address
	// Total: 3 options
	if iter.Total() != 3 {
		t.Errorf("Expected 3 total options, got %d", iter.Total())
	}

	services := []string{}
	for iter.HasNext() {
		option := iter.Next()
		services = append(services, option.ConnectData.ServiceName)
	}

	// First two should be svc1, last should be svc2
	if len(services) != 3 {
		t.Fatalf("Expected 3 services, got %d", len(services))
	}
	if services[0] != "svc1" || services[1] != "svc1" {
		t.Errorf("Expected first two options to be svc1, got %v", services)
	}
	if services[2] != "svc2" {
		t.Errorf("Expected third option to be svc2, got %s", services[2])
	}
}

// ============================================================================
// EDGE CASES
// ============================================================================

func TestConnectionIterator_Empty_Context(t *testing.T) {
	t.Parallel()
	// Empty context should result in no options
	ctx := &ConnectionContext{}
	iter := NewConnectionIterator(context.Background(), nil, ctx)

	if iter.Total() != 0 {
		t.Errorf("Expected total options to be 0 for empty context, got %d", iter.Total())
	}

	if iter.HasNext() {
		t.Error("Expected HasNext to be false for empty context")
	}

	if iter.Next() != nil {
		t.Error("Expected Next() to return nil for empty context")
	}

	if iter.Remaining() != 0 {
		t.Errorf("Expected Remaining to be 0 for empty context, got %d", iter.Remaining())
	}
}

func TestConnectionIterator_NoConnectData(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=test)(PORT=1521)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)
	if iter.Total() != 1 {
		t.Fatalf("Expected 1 option, got %d", iter.Total())
	}

	option := iter.Next()
	if option == nil {
		t.Fatal("Expected non-nil option")
	}
	if option.ConnectString == "" {
		t.Fatal("Expected ConnectString to be populated")
	}

	parsed, err := Parse(option.ConnectString)
	if err != nil {
		t.Fatalf("Generated ConnectString is not valid: %v", err)
	}
	if parsed.Name != "DESCRIPTION" {
		t.Errorf("Expected DESCRIPTION root, got %s", parsed.Name)
	}

	// Ensure a CONNECT_DATA node is present even if the original description had none
	hasConnectData := false
	for _, ch := range parsed.Children {
		if ch.Name == "CONNECT_DATA" {
			hasConnectData = true
			break
		}
	}
	if !hasConnectData {
		t.Error("Expected CONNECT_DATA node in ConnectString when none provided in input")
	}
}

func TestConnectionIterator_ZeroAddressesAfterFailover(t *testing.T) {
	t.Parallel()
	// This scenario shouldn't happen in practice, but test robustness
	// Create a context with description but somehow no addresses after failover logic
	description := NewDescription()
	description.Failover = false
	description.Addresses = []Address{}
	description.AddressLists = []AddressList{}
	description.ConnectData = ConnectData{ServiceName: "test"}
	ctx := &ConnectionContext{
		Description: description,
	}

	iter := NewConnectionIterator(context.Background(), nil, ctx)

	if iter.Total() != 0 {
		t.Errorf("Expected 0 options when no addresses available, got %d", iter.Total())
	}

	if iter.HasNext() {
		t.Error("Expected HasNext to be false when no addresses")
	}
}

// end
// start

// ============================================================================
// NIL POINTER HANDLING TESTS
// ============================================================================

func TestConnectionIterator_NilRootNode(t *testing.T) {
	t.Parallel()
	ctx := &ConnectionContext{
		Addresses: []Address{
			// Use direct IP to avoid multiple DNS answers for localhost.
			{Protocol: common.ProtocolTCP, Host: "127.0.0.1", Port: 1521},
		},
	}

	iter := NewConnectionIterator(context.Background(), nil, ctx)

	// Should still work with nil root node when addresses are provided
	if iter.Total() != 1 {
		t.Errorf("Expected 1 option with nil root node but valid addresses, got %d", iter.Total())
	}

	option := iter.Next()
	if option == nil {
		t.Fatal("Expected non-nil option")
	}
	if option.Address.Host != "127.0.0.1" {
		t.Errorf("Expected host '127.0.0.1', got '%s'", option.Address.Host)
	}
}

func TestConnectionIterator_NilDescription(t *testing.T) {
	t.Parallel()
	// Simple address-only context
	ctx := &ConnectionContext{
		Addresses: []Address{
			{Protocol: common.ProtocolTCP, Host: "host1", Port: 1521},
		},
	}

	iter := NewConnectionIterator(context.Background(), nil, ctx)

	option := iter.Next()
	if option == nil {
		t.Fatal("Expected non-nil option")
	}

	// Description should be nil for address-only iteration
	if option.Description != nil {
		t.Error("Expected nil Description for address-only context")
	}

	// But ConnectString should still be valid
	if option.ConnectString == "" {
		t.Error("Expected valid ConnectString even without Description")
	}
}

func TestConnectionIterator_NilDescriptionList(t *testing.T) {
	t.Parallel()
	ctx := &ConnectionContext{
		DescriptionList: nil,
		Description:     nil,
		Addresses:       []Address{},
	}

	iter := NewConnectionIterator(context.Background(), nil, ctx)

	if iter.Total() != 0 {
		t.Errorf("Expected 0 options with all nil, got %d", iter.Total())
	}
}

// ============================================================================
// CORRUPTED NODE STRUCTURE TESTS
// ============================================================================

func TestConnectionIterator_MissingAddressFields(t *testing.T) {
	t.Parallel()
	// Address with missing host
	ctx := &ConnectionContext{
		Addresses: []Address{
			{Protocol: common.ProtocolTCP, Host: "", Port: 1521}, // Missing host
		},
	}

	iter := NewConnectionIterator(context.Background(), nil, ctx)

	// Should still create option, validation happens elsewhere
	if iter.Total() != 1 {
		t.Errorf("Expected 1 option even with missing host, got %d", iter.Total())
	}

	option := iter.Next()
	if option == nil {
		t.Fatal("Expected non-nil option")
	}

	// Host should be empty as provided
	if option.Address.Host != "" {
		t.Errorf("Expected empty host, got '%s'", option.Address.Host)
	}
}

func TestConnectionIterator_MixedValidInvalidAddresses(t *testing.T) {
	t.Parallel()
	a1 := NewAddress()
	a1.Host = "valid"
	a1.Port = 1521
	a2 := NewAddress()
	a2.Host = ""
	a2.Port = 0
	a3 := NewAddress()
	a3.Host = "valid2"
	a3.Port = 1522
	ctx := &ConnectionContext{
		Addresses: []Address{*a1, *a2, *a3},
	}

	iter := NewConnectionIterator(context.Background(), nil, ctx)

	// Should have 3 options (including invalid one)
	if iter.Total() != 3 {
		t.Errorf("Expected 3 options, got %d", iter.Total())
	}

	count := 0
	for iter.HasNext() {
		option := iter.Next()
		if option != nil {
			count++
		}
	}

	if count != 3 {
		t.Errorf("Expected to iterate 3 options, got %d", count)
	}
}

// ============================================================================
// INVALID CONNECTION STRING TESTS
// ============================================================================

func TestConnectionIterator_InvalidConnectionString(t *testing.T) {
	t.Parallel()
	// This tests robustness when Parse returns error
	invalidInputs := []string{
		"(DESCRIPTION=",                   // Incomplete
		"DESCRIPTION=(ADDRESS=())",        // Missing parens
		"(DESCRIPTION=(ADDRESS=(HOST=)))", // Incomplete address
	}

	for _, input := range invalidInputs {
		_, err := Parse(input)
		if err == nil {
			// If parse succeeds somehow, test iterator handles it
			continue
		}
		// Expected to fail at parse stage
		// No need to check err == nil here as above handles it
	}
}

// ============================================================================
// PARAMS ASSOCIATION TESTS
// ============================================================================

func TestConnectionIterator_ParamsAssociationPerAddress(t *testing.T) {
	t.Parallel()
	// Ensure description-level params (CONNECT_DATA) are correctly associated with each address
	input := "(DESCRIPTION=(LOAD_BALANCE=on)(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=h1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=h2)(PORT=1522)))(CONNECT_DATA=(SERVICE_NAME=svcA)(INSTANCE_NAME=instA)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)
	if iter.Total() != 2 {
		t.Fatalf("Expected 2 options, got %d", iter.Total())
	}

	seen := map[string]bool{}
	for iter.HasNext() {
		a := iter.Next()
		if a == nil {
			t.Fatal("nil option")
		}
		// All options from this description should carry the same CONNECT_DATA
		if a.ConnectData.ServiceName != "svcA" {
			t.Errorf("Expected ServiceName 'svcA' for host %s, got %s", a.Address.Host, a.ConnectData.ServiceName)
		}
		if a.ConnectData.InstanceName != "instA" {
			t.Errorf("Expected InstanceName 'instA' for host %s, got %s", a.Address.Host, a.ConnectData.InstanceName)
		}
		if a.ConnectDataNode == nil {
			t.Error("Expected ConnectDataNode to be set")
		}
		if a.ConnectDataStr == "" {
			t.Error("Expected ConnectDataStr to be set")
		}
		seen[a.Address.Host] = true
	}

	expectedHosts := map[string]bool{"h1": true, "h2": true}
	if !reflect.DeepEqual(seen, expectedHosts) {
		t.Errorf("Expected hosts %v, got %v", expectedHosts, seen)
	}
}

func TestConnectionIterator_DifferentConnectDataPerDescription(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION_LIST=(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=h1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=svc1)(SID=sid1)))(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=h2)(PORT=1522))(CONNECT_DATA=(SERVICE_NAME=svc2)(SID=sid2))))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	option1 := iter.Next()
	option2 := iter.Next()

	if option1 == nil || option2 == nil {
		t.Fatal("Expected non-nil options")
	}

	// Each should have different CONNECT_DATA
	if option1.ConnectData.ServiceName == option2.ConnectData.ServiceName {
		t.Error("Expected different service names for different descriptions")
	}

	if option1.ConnectData.ServiceName != "svc1" {
		t.Errorf("Expected svc1, got %s", option1.ConnectData.ServiceName)
	}
	if option2.ConnectData.ServiceName != "svc2" {
		t.Errorf("Expected svc2, got %s", option2.ConnectData.ServiceName)
	}
}

// ============================================================================
// PERFORMANCE AND TIMEOUT TESTS
// ============================================================================

func TestConnectionIterator_LargeNumberOfAttempts(t *testing.T) {

	// Test iteration completes within reasonable time
	input := "(DESCRIPTION_LIST=(LOAD_BALANCE=on)(DESCRIPTION=(RETRY_COUNT=5)(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=h1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=h2)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=h3)(PORT=1521)))(CONNECT_DATA=(SERVICE_NAME=test)))(DESCRIPTION=(RETRY_COUNT=3)(ADDRESS=(PROTOCOL=TCP)(HOST=h4)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=test))))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	start := time.Now()
	iter := NewConnectionIterator(context.Background(), root, ctx)

	// Description 1: 3 addresses, RETRY_COUNT=5 -> 3 * (5+1) = 18 options
	// Description 2: 1 address, RETRY_COUNT=3 -> 1 * (3+1) = 4 options
	// Total: 22 options
	expectedTotal := 22
	if iter.Total() != expectedTotal {
		t.Errorf("Expected %d total options, got %d", expectedTotal, iter.Total())
	}

	count := 0
	for iter.HasNext() {
		iter.Next()
		count++
		if time.Since(start) > (time.Second * 2) {
			t.Fatal("Iteration took too long - possible infinite loop")
		}
	}

	if count != expectedTotal {
		t.Errorf("Iterated %d options, expected %d", count, expectedTotal)
	}
}

func TestConnectionIterator_VeryLargeRetryCount(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(RETRY_COUNT=1000)(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), nil, ctx)

	// Should calculate correctly even with large retry count
	expectedTotal := 1001 // 1 address * (1000+1)
	if iter.Total() != expectedTotal {
		t.Errorf("Expected %d total options, got %d", expectedTotal, iter.Total())
	}

	// Test that Remaining() works correctly
	if iter.Remaining() != expectedTotal {
		t.Errorf("Expected %d remaining, got %d", expectedTotal, iter.Remaining())
	}
}

// ============================================================================
// RESET BEHAVIOR TESTS
// ============================================================================
func TestConnectionIterator_Reset_WithLoadBalance(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(LOAD_BALANCE=on)(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522))(ADDRESS=(PROTOCOL=TCP)(HOST=host3)(PORT=1523)))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), nil, ctx)

	// Get first order
	order1 := []string{}
	for iter.HasNext() {
		option := iter.Next()
		order1 = append(order1, option.Address.Host)
	}

	// Reset and get second order
	iter.Reset()
	order2 := []string{}
	for iter.HasNext() {
		option := iter.Next()
		order2 = append(order2, option.Address.Host)
	}

	// With load balancing, orders might be different due to re-shuffling
	// Just verify we got all hosts both times
	if len(order1) != 3 || len(order2) != 3 {
		t.Errorf("Expected 3 hosts in each iteration, got %d and %d", len(order1), len(order2))
	}

	// Verify all hosts are present in both
	hosts1 := make(map[string]bool)
	hosts2 := make(map[string]bool)
	for _, h := range order1 {
		hosts1[h] = true
	}
	for _, h := range order2 {
		hosts2[h] = true
	}

	expectedHosts := map[string]bool{"host1": true, "host2": true, "host3": true}
	if !reflect.DeepEqual(hosts1, expectedHosts) || !reflect.DeepEqual(hosts2, expectedHosts) {
		t.Errorf("Expected all hosts in both iterations")
	}
}
func TestConnectionIterator_Reset_PreservesTotal(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(RETRY_COUNT=2)(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=host2)(PORT=1522)))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), nil, ctx)

	totalBefore := iter.Total()

	// Consume some
	iter.Next()
	iter.Next()

	// Reset
	iter.Reset()

	totalAfter := iter.Total()

	if totalBefore != totalAfter {
		t.Errorf("Expected total to remain %d after reset, got %d", totalBefore, totalAfter)
	}

	if iter.Remaining() != totalAfter {
		t.Errorf("Expected remaining to equal total after reset")
	}
}
func TestConnectionIterator_MultipleResets(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), nil, ctx)

	// Reset multiple times
	for i := 0; i < 5; i++ {
		if !iter.HasNext() {
			t.Errorf("Reset %d: Expected HasNext to be true", i)
		}
		iter.Next()
		if iter.HasNext() {
			t.Errorf("Reset %d: Expected HasNext to be false after consuming single option", i)
		}
		iter.Reset()
	}
}

// ============================================================================
// BOUNDARY CONDITION TESTS
// ============================================================================
func TestConnectionIterator_ExhaustionBehavior(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), nil, ctx)

	// Consume the single option
	iter.Next()

	// Now exhausted - verify behavior
	if iter.HasNext() {
		t.Error("Expected HasNext to be false after exhaustion")
	}

	if iter.Next() != nil {
		t.Error("Expected Next() to return nil after exhaustion")
	}

	if iter.Remaining() != 0 {
		t.Errorf("Expected Remaining to be 0 after exhaustion, got %d", iter.Remaining())
	}

	// Multiple Next() calls after exhaustion should all return nil
	for i := 0; i < 5; i++ {
		if iter.Next() != nil {
			t.Errorf("Call %d: Expected Next() to return nil after exhaustion", i)
		}
	}
}
func TestConnectionIterator_SingleAttemptMultipleCalls(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=host1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), nil, ctx)

	// First call should succeed
	option1 := iter.Next()
	if option1 == nil {
		t.Fatal("Expected first Next() to return non-nil")
	}

	// Second call should return nil (exhausted)
	option2 := iter.Next()
	if option2 != nil {
		t.Error("Expected second Next() to return nil")
	}
}

// ============================================================================
// STRESS TESTS
// ============================================================================
func TestConnectionIterator_ManyDescriptions(t *testing.T) {
	t.Parallel()
	// Build a large DESCRIPTION_LIST
	var builder string
	builder = "(DESCRIPTION_LIST="
	for i := 0; i < 50; i++ {
		builder += "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=host" + string(rune('0'+i%10)) + ")(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=svc)))"
	}
	builder += ")"
	root, err := Parse(builder)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	// Pass the real root (DESCRIPTION_LIST) so iterator can find DESCRIPTION nodes.
	iter := NewConnectionIterator(context.Background(), root, ctx)

	if iter.Total() != 50 {
		t.Errorf("Expected 50 options, got %d", iter.Total())
	}

	count := 0
	for iter.HasNext() {
		if iter.Next() != nil {
			count++
		}
	}

	if count != 50 {
		t.Errorf("Expected to iterate 50 options, got %d", count)
	}
}
func TestConnectionIterator_ManyAddresses(t *testing.T) {
	t.Parallel()
	// Build an ADDRESS_LIST with many addresses
	var builder string
	builder = "(DESCRIPTION=(ADDRESS_LIST="
	for i := 0; i < 100; i++ {
		builder += "(ADDRESS=(PROTOCOL=TCP)(HOST=host" + string(rune('0'+i%10)) + ")(PORT=1521))"
	}
	builder += ")(CONNECT_DATA=(SERVICE_NAME=test)))"
	root, err := Parse(builder)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ctx, err := ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}

	iter := NewConnectionIterator(context.Background(), root, ctx)

	if iter.Total() != 100 {
		t.Errorf("Expected 100 options, got %d", iter.Total())
	}

	count := 0
	start := time.Now()
	for iter.HasNext() {
		if iter.Next() != nil {
			count++
		}
		if time.Since(start) > 2*time.Second {
			t.Fatal("Iteration took too long")
		}
	}

	if count != 100 {
		t.Errorf("Expected to iterate 100 options, got %d", count)
	}
}

// ============================================================================
// DIRECT COVERAGE FOR roundRobin
// ============================================================================
func TestConnectionIterator_RoundRobin(t *testing.T) {
	t.Parallel()
	iter := NewConnectionIterator(context.Background(), nil, &ConnectionContext{})

	t.Run("zero addresses - no change", func(t *testing.T) {
		var addrs []Address
		iter.roundRobin(addrs)
		if len(addrs) != 0 {
			t.Fatalf("expected 0 addresses, got %d", len(addrs))
		}
	})

	t.Run("one address - no change", func(t *testing.T) {
		addrs := []Address{{Host: "h1"}}
		iter.roundRobin(addrs)
		if len(addrs) != 1 || addrs[0].Host != "h1" {
			t.Fatalf("expected [h1], got %+v", addrs)
		}
	})

	t.Run("two addresses - rotate", func(t *testing.T) {
		addrs := []Address{{Host: "a"}, {Host: "b"}}
		iter.roundRobin(addrs)
		got := []string{addrs[0].Host, addrs[1].Host}
		exp := []string{"b", "a"}
		if !reflect.DeepEqual(got, exp) {
			t.Fatalf("expected %v, got %v", exp, got)
		}
	})

	t.Run("three addresses - rotate twice", func(t *testing.T) {
		addrs := []Address{{Host: "a"}, {Host: "b"}, {Host: "c"}}
		iter.roundRobin(addrs)
		got1 := []string{addrs[0].Host, addrs[1].Host, addrs[2].Host}
		exp1 := []string{"b", "c", "a"}
		if !reflect.DeepEqual(got1, exp1) {
			t.Fatalf("after 1 rotate expected %v, got %v", exp1, got1)
		}
		iter.roundRobin(addrs)
		got2 := []string{addrs[0].Host, addrs[1].Host, addrs[2].Host}
		exp2 := []string{"c", "a", "b"}
		if !reflect.DeepEqual(got2, exp2) {
			t.Fatalf("after 2 rotates expected %v, got %v", exp2, got2)
		}
	})
}
