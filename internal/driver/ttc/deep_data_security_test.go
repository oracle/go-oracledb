/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively, the "Software"), free of charge and under any and all copyright
** rights and any and all patent rights owned or freely
** licensable by each licensor hereunder covering either (i) the unmodified
** Software as contributed to or provided by such licensor, or (ii) the Larger
** Works (as defined below),
** to deal in both
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
** a minimum a reference to the UPL must be included in all copies or substantial
** portions of the Software.
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
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/oracle/datatype/json"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"github.com/oracle/go-oracledb/v26/oracle/providers"
	osql "github.com/oracle/go-oracledb/v26/oracle/sql"
)

// securityTestProvider records requests and supplies a configured context.
type securityTestProvider struct {
	selected osql.EndUserSecurityContext
	err      error
	calls    int
	seen     context.Context
}

// EndUserSecurityContext records the request and returns the configured result.
func (p *securityTestProvider) EndUserSecurityContext(ctx context.Context) (osql.EndUserSecurityContext, error) {
	p.calls++
	p.seen = ctx
	return p.selected, p.err
}

// securityTestContext supplies application-owned context values for driver tests.
type securityTestContext struct {
	dbToken     []byte
	endUsername string
	attrs       map[string]json.JSONObject
}

// DatabaseAccessToken returns the configured database token.
func (c *securityTestContext) DatabaseAccessToken() []byte { return c.dbToken }

// DatabaseAccessTokenExpiration returns a future expiration for test credentials.
func (c *securityTestContext) DatabaseAccessTokenExpiration() time.Time {
	return time.Now().Add(time.Hour)
}

// EndUserToken reports that this fixture has no separate end-user token.
func (c *securityTestContext) EndUserToken() ([]byte, bool) { return nil, false }

// EndUserTokenExpiration returns zero because no end-user token is present.
func (c *securityTestContext) EndUserTokenExpiration() time.Time { return time.Time{} }

// EndUserName returns the configured optional principal.
func (c *securityTestContext) EndUserName() (string, bool) {
	return c.endUsername, c.endUsername != ""
}

// DataRoles returns no additional roles.
func (c *securityTestContext) DataRoles() []string { return nil }

// Attributes returns the configured context attributes.
func (c *securityTestContext) Attributes() map[string]json.JSONObject { return c.attrs }

// LookUpKey reports that this fixture has no lookup key.
func (c *securityTestContext) LookUpKey() ([]byte, bool) { return nil, false }

// securityTestExpiringContext exposes a controllable expiration for a context.
type securityTestExpiringContext struct {
	osql.EndUserSecurityContext
	expires time.Time
}

// DatabaseAccessTokenExpiration returns the configured expiration time.
func (c *securityTestExpiringContext) DatabaseAccessTokenExpiration() time.Time {
	return c.expires
}

// TestDeepDataSecurityContextSelection verifies precedence and provider results.
func TestDeepDataSecurityContextSelection(t *testing.T) {
	t.Parallel()
	shelf := newShelf[driverCommon.MessageType]()
	conn := &connection{shelf: shelf, _isValid: true}
	state := &deepDataSecurity{connection: conn}
	shelf._deepDataSecurity = state
	shelf.RegisterCapabilities(map[string]driverCommon.Capability{kpccapCtbBackportFeature1Xsctx: {IsSet: true}})
	shelf.RegisterMessageFactory(&mockFactory{returnMsg: newOXSCTX()})
	if selected, err := state.selectContext(context.Background()); err != nil || selected != nil {
		t.Fatalf("select without a provider: context = %v, error = %v", selected, err)
	}
	explicit := &securityTestContext{dbToken: []byte("explicit")}
	provided := &securityTestContext{dbToken: []byte("provided")}
	provider := &securityTestProvider{selected: provided}
	registry := common.NewSafeRegistry[providers.Provider]()
	registry.Register(provider)
	shelf.registerProviderRegistry(registry)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := conn.SetEndUserSecurityContext(explicit); err != nil {
		t.Fatal(err)
	}
	if _, err := state.prepare(ctx); err != nil || provider.calls != 0 {
		t.Fatalf("prepare with explicit context: calls = %d, error = %v", provider.calls, err)
	}
	conn.ClearEndUserSecurityContext()
	for i := 0; i < 2; i++ {
		if msg, err := state.prepare(ctx); err != nil || msg == nil {
			t.Fatalf("prepare with provider: message = %v, error = %v", msg, err)
		}
	}
	if provider.calls != 2 || provider.seen != ctx {
		t.Fatalf("provider calls = %d, context matches = %v", provider.calls, provider.seen == ctx)
	}
	provider.selected = nil
	if msg, err := state.prepare(ctx); err != nil || msg != nil {
		t.Fatalf("prepare with no context: message = %v, error = %v", msg, err)
	}
	provider.err = common.NewOracleError(oracleErrors.InternalError, nil)
	if msg, err := state.prepare(ctx); msg != nil || !errors.Is(err, provider.err) {
		t.Fatalf("prepare with provider failure: message = %v, error = %v", msg, err)
	}
}

// TestDeepDataSecurityValidation verifies selection and send-time validation.
func TestDeepDataSecurityValidation(t *testing.T) {
	t.Parallel()
	shelf := newShelf[driverCommon.MessageType]()
	conn := &connection{shelf: shelf, _isValid: true}
	state := &deepDataSecurity{connection: conn}
	shelf._deepDataSecurity = state
	shelf.RegisterCapabilities(map[string]driverCommon.Capability{kpccapCtbBackportFeature1Xsctx: {IsSet: true}})
	shelf.RegisterMessageFactory(&mockFactory{returnMsg: newOXSCTX()})
	base := &securityTestContext{dbToken: []byte("token")}
	selected := &securityTestExpiringContext{EndUserSecurityContext: base, expires: time.Now().Add(time.Hour)}
	if err := conn.SetEndUserSecurityContext(selected); err != nil {
		t.Fatal(err)
	}
	invalid := &securityTestContext{dbToken: []byte{0xff}}
	if err := conn.SetEndUserSecurityContext(invalid); err == nil {
		t.Fatal("selecting a context with an invalid token succeeded")
	}
	if current, err := state.selectContext(context.Background()); err != nil || current != selected {
		t.Fatalf("context changed after invalid selection: context = %v, error = %v", current, err)
	}
	selected.expires = time.Now().Add(-time.Hour)
	_, err := state.prepare(context.Background())
	var sqlErr oracleErrors.SQLError
	if !errors.As(err, &sqlErr) || sqlErr.ErrorCode() != string(oracleErrors.ExpiredToken) {
		t.Fatalf("prepare with expired token: error = %v", err)
	}
	if err := conn.SetEndUserSecurityContext(selected); err == nil {
		t.Fatal("selecting an expired context succeeded")
	}

	// payload size is checked when sending rather than when selecting
	oversized := &securityTestContext{dbToken: []byte(strings.Repeat("x", maxXsctxValueLength))}
	if err := conn.SetEndUserSecurityContext(oversized); err != nil {
		t.Fatalf("selecting oversized context: %v", err)
	}
	if _, err := state.prepare(context.Background()); err == nil {
		t.Fatal("preparing an oversized payload succeeded")
	}
}

// TestEncodeEndUserSecurityContextAttributes verifies JSONObject attributes keep
// the existing database payload shape and reject uninitialized object values.
func TestEncodeEndUserSecurityContextAttributes(t *testing.T) {
	t.Parallel()

	attributes := make(map[string]json.JSONObject)
	for name, value := range map[string]string{"Z.CONTEXT": "z", "A.CONTEXT": "a"} {
		attributeJSON, err := json.NewJSON(map[string]any{"value": value})
		if err != nil {
			t.Fatal(err)
		}
		attribute, err := attributeJSON.AsJSONObject()
		if err != nil {
			t.Fatal(err)
		}
		attributes[name] = attribute
	}
	context := &securityTestContext{
		dbToken:     []byte("database-token"),
		endUsername: "user",
		attrs:       attributes,
	}

	encoded, err := encodeEndUserSecurityContext(context)
	if err != nil {
		t.Fatal(err)
	}
	var document json.JSON
	if err := document.Scan(encoded); err != nil {
		t.Fatal(err)
	}
	payload, err := document.GetValue()
	if err != nil {
		t.Fatal(err)
	}
	got := payload.(map[string]any)["attributes"]
	want := []any{
		map[string]any{"name": "A.CONTEXT", "values": map[string]any{"value": "a"}},
		map[string]any{"name": "Z.CONTEXT", "values": map[string]any{"value": "z"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("encoded attributes = %#v, want %#v", got, want)
	}

	context = &securityTestContext{
		dbToken:     []byte("database-token"),
		endUsername: "user",
		attrs:       map[string]json.JSONObject{"A.CONTEXT": {}},
	}
	if _, err := encodeEndUserSecurityContext(context); err == nil {
		t.Fatal("encodeEndUserSecurityContext() error = nil, want uninitialized attribute error")
	}
}

// TestDeepDataSecurityExecution verifies request ordering, repeated provider use,
// and failures before the statement is queued in both execution paths.
func TestDeepDataSecurityExecution(t *testing.T) {
	t.Parallel()
	for _, query := range []bool{true, false} {
		name := "exec"
		if query {
			name = "query"
		}
		t.Run(name, func(t *testing.T) {
			for _, scenario := range []string{"context", "no context", "provider error", "unsupported", "expired", "factory error", "push error"} {
				t.Run(scenario, func(t *testing.T) {
					shelf := newShelf[driverCommon.MessageType]()
					conn := &connection{shelf: shelf, _isValid: true}
					shelf._deepDataSecurity = &deepDataSecurity{connection: conn}
					shelf.RegisterCapabilities(map[string]driverCommon.Capability{kpccapCtbBackportFeature1Xsctx: {IsSet: scenario != "unsupported"}})
					selected := &securityTestContext{dbToken: []byte("token")}
					provider := &securityTestProvider{selected: selected}
					registry := common.NewSafeRegistry[providers.Provider]()
					registry.Register(provider)
					shelf.registerProviderRegistry(registry)
					piggyback := newOXSCTX()
					factory := &mockFactory{returnMsg: piggyback}
					shelf.RegisterMessageFactory(factory)
					streamer := &mockStreamer{pullMsg: &mockOer{}}
					shelf.RegisterMessageStreamer(streamer)
					var err error
					var wantErr error
					var wantCode oracleErrors.ErrorCode
					switch scenario {
					case "no context":
						provider.selected = nil
					case "provider error":
						provider.err = common.NewOracleError(oracleErrors.InternalError, nil)
						wantErr = provider.err
					case "unsupported":
						wantCode = oracleErrors.DeepDataSecurityNotSupported
					case "expired":
						provider.selected = &securityTestExpiringContext{EndUserSecurityContext: selected, expires: time.Now().Add(-time.Hour)}
						wantCode = oracleErrors.ExpiredToken
					case "factory error":
						factory.returnErr = common.NewOracleError(oracleErrors.InternalError, nil)
						wantErr = factory.returnErr
					case "push error":
						streamer.pushErr = common.NewOracleError(oracleErrors.InternalError, nil)
						wantErr = streamer.pushErr
					}
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					statement := &dummyMsg{}
					selectExecutor := newStatementExecutorSelect()
					selectExecutor.SetShelf(shelf)
					execExecutor := newStatementExecutorDML()
					execExecutor.SetShelf(shelf)
					for attempt := 1; attempt <= 2; attempt++ {
						streamer.pushedMsg.Init()
						if query {
							// End-of-fetch allows the query to complete without column metadata.
							streamer.pullMsg = &mockOer{err: common.NewOracleError(oracleErrors.NoDataFound, nil)}
							_, _, err = selectExecutor.runQuery(ctx, statement)
							if isNoDataFoundError(err) {
								err = nil
							}
						} else {
							_, err = execExecutor.runExec(ctx, statement)
						}
						if wantErr != nil && !errors.Is(err, wantErr) {
							t.Fatalf("execution error = %v, want %v", err, wantErr)
						}
						if wantCode != "" {
							var sqlErr oracleErrors.SQLError
							if !errors.As(err, &sqlErr) || sqlErr.ErrorCode() != string(wantCode) {
								t.Fatalf("execution error = %v, want code %s", err, wantCode)
							}
						}
						if wantErr == nil && wantCode == "" && err != nil {
							t.Fatal(err)
						}
						if provider.calls != attempt || provider.seen != ctx {
							t.Fatalf("provider calls = %d, want %d; request context matches = %v", provider.calls, attempt, provider.seen == ctx)
						}
						var want []driverCommon.Message[driverCommon.MessageType]
						switch scenario {
						case "context":
							want = append(want, piggyback, statement)
						case "no context":
							want = append(want, statement)
						case "push error":
							want = append(want, piggyback)
						}
						var got []driverCommon.Message[driverCommon.MessageType]
						for entry := streamer.pushedMsg.Front(); entry != nil; entry = entry.Next() {
							got = append(got, *entry.Value.(*driverCommon.Message[driverCommon.MessageType]))
						}
						if !reflect.DeepEqual(got, want) {
							t.Fatalf("queued messages = %v, want %v", got, want)
						}
						if (wantErr != nil || wantCode != "") && streamer.pullCalled {
							t.Fatal("execution read a response after a preparation or push failure")
						}
					}
				})
			}
		})
	}
}

// TestDeepDataSecurityFlush verifies flushing queued or empty output never
// consults the security provider, including when the provider would fail.
func TestDeepDataSecurityFlush(t *testing.T) {
	t.Parallel()
	shelf, streamer, _ := newExecTestShelf(1024)
	conn := &connection{shelf: shelf, _isValid: true}
	shelf._deepDataSecurity = &deepDataSecurity{connection: conn}
	provider := &securityTestProvider{err: common.NewOracleError(oracleErrors.InternalError, nil)}
	registry := common.NewSafeRegistry[providers.Provider]()
	registry.Register(provider)
	shelf.registerProviderRegistry(registry)
	ctx := context.Background()
	if err := streamer.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := streamer.Push(ctx, &mockMessage{msgType: TTIPRO}); err != nil {
		t.Fatal(err)
	}
	if err := streamer.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 0 || streamer.outgoingMessages.Len() != 0 {
		t.Fatalf("flush called provider %d times; %d messages remain", provider.calls, streamer.outgoingMessages.Len())
	}
}
