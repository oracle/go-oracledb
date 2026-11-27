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
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/oracle/datatype/json"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"github.com/oracle/go-oracledb/v26/oracle/providers"
	osql "github.com/oracle/go-oracledb/v26/oracle/sql"
)

// ddsTestProvider supplies a replaceable context and records provider calls.
type ddsTestProvider struct {
	selected osql.EndUserSecurityContext
	err      error
	calls    int
	ctx      context.Context
}

// EndUserSecurityContext returns the configured context for the current request.
func (p *ddsTestProvider) EndUserSecurityContext(ctx context.Context) (osql.EndUserSecurityContext, error) {
	p.calls++
	p.ctx = ctx
	return p.selected, p.err
}

// ddsTestConnection builds a connection with a configurable transport and server capability.
func ddsTestConnection(tls, supported bool) (*connection, *deepDataSecurity) {
	shelf := newShelf[driverCommon.MessageType]()
	conn := &connection{shelf: shelf, ns: &mockNetworkSession{tls: tls}, _isValid: true}
	dds := &deepDataSecurity{connection: conn}
	shelf._deepDataSecurity = dds
	shelf.RegisterCapabilities(map[string]driverCommon.Capability{kpccapCtbBackportFeature1Xsctx: {IsSet: supported}})
	shelf.RegisterMessageFactory(&mockFactory{returnMsg: newOXSCTX()})
	return conn, dds
}

// ddsAssertError checks the public driver error code.
func ddsAssertError(t *testing.T, err error, code oracleErrors.ErrorCode) {
	t.Helper()
	var sqlErr oracleErrors.SQLError
	if !errors.As(err, &sqlErr) || sqlErr.ErrorCode() != string(code) {
		t.Fatalf("error = %v, want %s", err, code)
	}
}

// ddsAssertPayload decodes a context and checks the fields seen by the database.
func ddsAssertPayload(t *testing.T, msg *ttiOXSCTX, want map[string]any) {
	t.Helper()
	if msg == nil || len(msg.pairs) != 1 || string(msg.pairs[0].key.value) != "ORCL_XS_AUTHZ_CONTEXT" {
		t.Fatalf("unexpected context message: %#v", msg)
	}
	var document json.JSON
	if err := document.Scan([]byte(msg.pairs[0].binaryValue.value)); err != nil {
		t.Fatal(err)
	}
	got, err := document.GetValue()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("payload = %#v, error = %v, want %#v", got, err, want)
	}
}

// TestDDSSecurityContextValidation checks identity rules and token expiration.
func TestDDSSecurityContextValidation(t *testing.T) {
	t.Parallel()
	jwt := func(claims string) string { return "a." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".b" }
	for _, tc := range []struct {
		name  string
		value osql.EndUserSecurityContext
		valid bool
	}{
		{name: "missing database token"},
		{name: "application identity", value: osql.EndUserSecurityContext{DatabaseAccessToken: "opaque"}, valid: true},
		{name: "named identity", value: osql.EndUserSecurityContext{DatabaseAccessToken: "db", NamedUser: osql.NamedEndUser{Username: "user", LookupKey: "key"}}, valid: true},
		{name: "conflicting identities", value: osql.EndUserSecurityContext{DatabaseAccessToken: "db", EndUserToken: "user", NamedUser: osql.NamedEndUser{Username: "user"}}},
		{name: "key without name", value: osql.EndUserSecurityContext{DatabaseAccessToken: "db", NamedUser: osql.NamedEndUser{LookupKey: "key"}}},
		{name: "expired database token", value: osql.EndUserSecurityContext{DatabaseAccessToken: jwt(`{"exp":1}`)}},
		{name: "expired user token", value: osql.EndUserSecurityContext{DatabaseAccessToken: "db", EndUserToken: jwt(`{"exp":1}`)}},
		{name: "unknown expiration", value: osql.EndUserSecurityContext{DatabaseAccessToken: jwt(`{"sub":"user"}`), EndUserToken: "opaque"}, valid: true},
		{name: "future expiration", value: osql.EndUserSecurityContext{DatabaseAccessToken: jwt(`{"exp":9999999999}`)}, valid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// a rejected replacement must not change the active identity
			conn, state := ddsTestConnection(true, true)
			previous := osql.EndUserSecurityContext{DatabaseAccessToken: "previous"}
			if err := conn.SetEndUserSecurityContext(previous); err != nil {
				t.Fatal(err)
			}
			err := conn.SetEndUserSecurityContext(tc.value)
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			ddsAssertError(t, err, oracleErrors.InvalidEndUserSecurityContext)
			if !reflect.DeepEqual(state.context, previous) {
				t.Fatal("rejected context replaced the active context")
			}
		})
	}
}

// TestDDSProviderAndReset checks context precedence, refresh, and pool reuse.
func TestDDSProviderAndReset(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), struct{}{}, "request")
	conn, state := ddsTestConnection(true, true)
	conn.sessCtx = driverCommon.NewSessionContext()
	conn.shelf.RegisterMessageStreamer(&mockStreamer{})
	provider := &ddsTestProvider{selected: osql.EndUserSecurityContext{DatabaseAccessToken: "provider-one"}}
	registry := common.NewSafeRegistry[providers.Provider]()
	registry.Register(provider)
	conn.shelf.registerProviderRegistry(registry)

	// an explicit context wins without calling the provider
	if err := conn.SetEndUserSecurityContext(osql.EndUserSecurityContext{DatabaseAccessToken: "explicit"}); err != nil {
		t.Fatal(err)
	}
	msg, _, err := state.preparePiggyback(ctx)
	if err != nil || provider.calls != 0 {
		t.Fatalf("prepare error = %v, calls = %d", err, provider.calls)
	}
	ddsAssertPayload(t, msg, map[string]any{"ver": "1.0", "database_access_token": "explicit"})

	// the transport is checked again before an already selected context is sent
	conn.ns.(*mockNetworkSession).tls = false
	if msg, _, err := state.preparePiggyback(ctx); msg != nil {
		t.Fatal("insecure transport prepared a context")
	} else {
		ddsAssertError(t, err, oracleErrors.DeepDataSecurityRequiresTLS)
	}
	// should be an error since conn is not secure
	err = conn.SetEndUserSecurityContext(osql.EndUserSecurityContext{DatabaseAccessToken: "other"})
	ddsAssertError(t, err, oracleErrors.DeepDataSecurityRequiresTLS)
	conn.ns.(*mockNetworkSession).tls = true

	// reset clears the prior borrower and the provider can refresh between calls
	if err := conn.ResetSession(ctx); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"provider-one", "provider-two"} {
		provider.selected.DatabaseAccessToken = token
		msg, _, err = state.preparePiggyback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		ddsAssertPayload(t, msg, map[string]any{"ver": "1.0", "database_access_token": token})
	}
	if provider.calls != 2 || provider.ctx != ctx {
		t.Fatalf("provider calls = %d, request context = %v", provider.calls, provider.ctx == ctx)
	}
	provider.err = common.NewOracleError(oracleErrors.InternalError, nil)
	if msg, _, err := state.preparePiggyback(ctx); msg != nil || !errors.Is(err, provider.err) {
		t.Fatalf("message = %v, error = %v", msg, err)
	}
}

// TestDDSExecution checks piggyback ordering and failure before a database call
func TestDDSExecution(t *testing.T) {
	t.Parallel()
	// run the same context scenarios through both statement executors
	for _, operation := range []string{"query", "exec"} {
		// provider: 		queues a valid context before the statement
		// absent: 			queues only the statement when no provider is registered
		// insecure: 		rejects the context because the connection lacks tls
		// unsupported: rejects the context because the server lacks deep data security
		// invalid: 		rejects the context because its database access token is empty
		for _, scenario := range []string{"provider", "absent", "insecure", "unsupported", "invalid"} {
			t.Run(operation+"/"+scenario, func(t *testing.T) {
				conn, _ := ddsTestConnection(scenario != "insecure" && scenario != "absent", scenario != "unsupported" && scenario != "absent")
				provider := &ddsTestProvider{selected: osql.EndUserSecurityContext{DatabaseAccessToken: "db"}}
				if scenario != "absent" {
					registry := common.NewSafeRegistry[providers.Provider]()
					registry.Register(provider)
					conn.shelf.registerProviderRegistry(registry)
				}
				if scenario == "invalid" {
					provider.selected.DatabaseAccessToken = ""
				}
				streamer := &mockStreamer{pullMsg: &mockOer{}}
				conn.shelf.RegisterMessageStreamer(streamer)
				statement := &dummyMsg{}
				messages := []driverCommon.Message[driverCommon.MessageType]{statement}
				var err error
				if operation == "query" {
					streamer.pullMsg = &mockOer{err: common.NewOracleError(oracleErrors.NoDataFound, nil)}
					executor := newStatementExecutorSelect()
					executor.SetShelf(conn.shelf)
					_, _, err = executor.runQuery(context.Background(), messages)
					if isNoDataFoundError(err) {
						err = nil
					}
				} else {
					executor := newStatementExecutorDML()
					executor.SetShelf(conn.shelf)
					_, err = executor.runExec(context.Background(), messages)
				}
				// preparation failures must not queue or flush a statement
				if scenario != "provider" && scenario != "absent" {
					code := oracleErrors.InvalidEndUserSecurityContext
					if scenario == "insecure" {
						code = oracleErrors.DeepDataSecurityRequiresTLS
					}
					if scenario == "unsupported" {
						code = oracleErrors.DeepDataSecurityNotSupported
					}
					ddsAssertError(t, err, code)
					if streamer.pushedMsg.Len() != 0 || streamer.flushCalls != 0 {
						t.Fatal("failed preparation sent a database call")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				// a context is sent before the statement and absence does not require tls
				first := streamer.pushedMsg.Front()
				if scenario == "provider" {
					if first == nil || (*first.Value.(*driverCommon.Message[driverCommon.MessageType])).GetMsgCode() != TTIPFN {
						t.Fatal("context was not queued first")
					}
					first = first.Next()
				}
				if first == nil || *first.Value.(*driverCommon.Message[driverCommon.MessageType]) != statement || first.Next() != nil {
					t.Fatal("unexpected queued messages")
				}
			})
		}
	}
}

// TestDDSPayload checks the context fields and stable role and attribute encoding.
func TestDDSPayload(t *testing.T) {
	t.Parallel()
	attributes := make(map[string]json.JSONObject)
	for _, name := range []string{"Z.CONTEXT", "A.CONTEXT"} {
		doc, err := json.NewJSON(map[string]any{"value": name})
		if err != nil {
			t.Fatal(err)
		}
		attributes[name], err = doc.AsJSONObject()
		if err != nil {
			t.Fatal(err)
		}
	}
	conn, state := ddsTestConnection(true, true)
	selected := osql.EndUserSecurityContext{DatabaseAccessToken: "db", NamedUser: osql.NamedEndUser{Username: "user", LookupKey: "key"}, DataRoles: []string{"z", "a", "a"}, Attributes: attributes}
	if err := conn.SetEndUserSecurityContext(selected); err != nil {
		t.Fatal(err)
	}
	// caller changes must not alter the selected roles or attributes
	selected.DataRoles[0] = "changed"
	delete(attributes, "A.CONTEXT")
	msg, _, err := state.preparePiggyback(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ddsAssertPayload(t, msg, map[string]any{
		"ver": "1.0", "database_access_token": "db", "end_user_name": "user", "end_user_contextid": "key",
		"data_roles": []any{"a", "z"},
		"attributes": []any{map[string]any{"name": "A.CONTEXT", "values": map[string]any{"value": "A.CONTEXT"}}, map[string]any{"name": "Z.CONTEXT", "values": map[string]any{"value": "Z.CONTEXT"}}},
	})

	// a token principal is encoded without a named identity
	if err := conn.SetEndUserSecurityContext(osql.EndUserSecurityContext{DatabaseAccessToken: "db", EndUserToken: "user-token"}); err != nil {
		t.Fatal(err)
	}
	msg, _, err = state.preparePiggyback(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ddsAssertPayload(t, msg, map[string]any{"ver": "1.0", "database_access_token": "db", "end_user_token": "user-token"})
}

// TestDDSOXSCTXWire checks protocol framing and rejects oversized values before writing.
func TestDDSOXSCTXWire(t *testing.T) {
	t.Parallel()
	for _, version := range []int{MinTTCProtocolVersion, 18} {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			// compare the complete piggyback against the expected ttc bytes
			factory := &SimpleFactory{ttcVersion: int8(version), funcregistry: FunctionRegistry}
			message, err := factory.GetMessageForFunction(TTIPFN, oXsctx)
			if err != nil {
				t.Fatal(err)
			}
			msg := message.(*ttiOXSCTX)
			msg.pairs = []*keywordValuePairWithName{newKeywordValuePairWithName("K", "T", []byte("BC"), 7)}
			buf := NewArrayDataBuffer(256)
			engine := NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
			shelf := newShelf[driverCommon.MessageType]()
			shelf.RegisterMarshaller(engine)
			streamer := NewMessageStreamer(shelf)
			if err := streamer.Push(context.Background(), msg); err != nil {
				t.Fatal(err)
			}
			if err := streamer.Flush(context.Background()); err != nil {
				t.Fatal(err)
			}
			want := []byte{0x11, 0xcd, 1}
			if version >= 18 {
				want = append(want, 0)
			}
			want = append(want, 1, 1, 1, 1, 1, 1, 7, 1, 1, 1, 'K', 1, 1, 1, 'T', 1, 2, 2, 'B', 'C')
			if got := buf.bytes[:buf.currentWritePosition]; !bytes.Equal(got, want) {
				t.Fatalf("wire = % X, want % X", got, want)
			}
		})
	}
	// the hash and context must be written as two ordered pairs
	t.Run("two pairs", func(t *testing.T) {
		msg := newOXSCTX().(*ttiOXSCTX)
		msg.pairs = []*keywordValuePairWithName{
			newKeywordValuePairWithName("H", "h", nil, 0),
			newKeywordValuePairWithName("C", "", []byte("P"), 0),
		}
		buf := NewArrayDataBuffer(256)
		engine := NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
		shelf := newShelf[driverCommon.MessageType]()
		shelf.RegisterMarshaller(engine)
		streamer := NewMessageStreamer(shelf)
		if err := streamer.Push(context.Background(), msg); err != nil {
			t.Fatal(err)
		}
		if err := streamer.Flush(context.Background()); err != nil {
			t.Fatal(err)
		}
		want := []byte{0x11, 0xcd, 1, 1, 1, 1, 1, 2,
			0, 1, 1, 1, 'H', 1, 1, 1, 'h', 0,
			0, 1, 1, 1, 'C', 0, 1, 1, 1, 'P'}
		if got := buf.bytes[:buf.currentWritePosition]; !bytes.Equal(got, want) {
			t.Fatalf("wire = % X, want % X", got, want)
		}
	})

	// each field accepts its limit and rejects overflow before writing
	for _, field := range []string{"key", "text", "binary"} {
		for _, delta := range []int{0, 1} {
			t.Run(field+"/"+strconv.Itoa(delta), func(t *testing.T) {
				limit := maxXsctxValueLength
				if field == "key" {
					limit = maxXsctxKeyLength
				}
				value := strings.Repeat("x", limit+delta)
				key, text := "K", ""
				var binary []byte
				switch field {
				case "key":
					key = value
				case "text":
					text = value
				case "binary":
					binary = []byte(value)
				}
				msg := newOXSCTX().(*ttiOXSCTX)
				msg.pairs = []*keywordValuePairWithName{newKeywordValuePairWithName(key, text, binary, 0)}
				buf := NewArrayDataBuffer(140000)
				engine := NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
				err := msg.MarshalTo(context.Background(), engine)
				if delta == 0 {
					if err != nil {
						t.Fatal(err)
					}
					return
				}
				ddsAssertError(t, err, oracleErrors.FailMarshal)
				if buf.currentWritePosition != 0 {
					t.Fatal("oversized value wrote a partial header")
				}
			})
		}
	}
}

// TestDDSHashNegotiation checks the authentication flag used to enable hashing.
func TestDDSHashNegotiation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		value any
		want  bool
	}{
		{name: "absent"},
		{name: "disabled", value: driverCommon.B1Array("0")},
		{name: "enabled", value: driverCommon.B1Array("1"), want: true},
		{name: "other bits", value: "3", want: true},
		{name: "invalid", value: "bad"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := &connection{sessCtx: driverCommon.NewSessionContext()}
			if tc.value != nil {
				conn.sessCtx.GetSessionProperties().SetProperty("AUTH_CLIENT_EUSC_FLAG", tc.value)
			}
			dds := newDeepDataSecurity(conn)
			// reset may clear session properties after the flag is captured
			conn.sessCtx.GetSessionProperties().Reset()
			if dds.hashSupported != tc.want {
				t.Fatalf("hash support = %v, want %v", dds.hashSupported, tc.want)
			}
		})
	}
}

// TestDDSHashVectors checks token order, role normalization, and byte lengths.
func TestDDSHashVectors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		context osql.EndUserSecurityContext
		want    string
	}{
		{name: "database token only", context: osql.EndUserSecurityContext{DatabaseAccessToken: "db"}, want: "e9wl0WlO+YR4Khb28XEMHGvIO6ehMbUVuvUyvqAh0BE="},
		{name: "token and roles", context: osql.EndUserSecurityContext{DatabaseAccessToken: "db", EndUserToken: "user", DataRoles: []string{"reader", "admin", "admin"}}, want: "6obRaUlghHiYSs8+duqfcTdN0ysH2PcuyW52xppckWM="},
		{name: "multibyte role", context: osql.EndUserSecurityContext{DatabaseAccessToken: "db", EndUserToken: "u", DataRoles: []string{"é"}}, want: "3Tmlfugsj+PdZWX87yHCe5J+jWr/syJwqgAC2BsvJCg="},
		{name: "supplementary role order", context: osql.EndUserSecurityContext{DatabaseAccessToken: "db", DataRoles: []string{"\ue000", "\U00010000"}}, want: "JrqBb0blF4f1GVJoFJ65yv98cd2eNlUzWmuXYUExnf8="},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// fixed vectors make the encoded hash independent of the implementation
			roles := slices.Clone(tc.context.DataRoles)
			if got := secCtxHash(tc.context); got != tc.want {
				t.Fatalf("hash = %q, want %q", got, tc.want)
			}
			if !reflect.DeepEqual(roles, tc.context.DataRoles) {
				t.Fatal("hash calculation modified the caller roles")
			}
		})
	}
}

// ddsAssertHashMessage checks the hash pair and decodes the accompanying context.
func ddsAssertHashMessage(t *testing.T, msg *ttiOXSCTX, hash string, payload map[string]any) {
	t.Helper()
	if msg == nil || len(msg.pairs) != 2 || string(msg.pairs[0].key.value) != "ORCL_XS_AUTH_HASH" || string(msg.pairs[0].textValue.value) != hash || len(msg.pairs[0].binaryValue.value) != 0 {
		t.Fatalf("unexpected hash pair: %#v", msg)
	}
	ddsAssertPayload(t, &ttiOXSCTX{pairs: msg.pairs[1:]}, payload)
}

// TestDDSHashRoundTrips checks full and reduced contexts across query and exec.
func TestDDSHashRoundTrips(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"query", "exec"} {
		t.Run(operation, func(t *testing.T) {
			conn, state := ddsTestConnection(true, true)
			state.hashSupported = true
			conn.sessCtx = driverCommon.NewSessionContext()
			provider := &ddsTestProvider{selected: osql.EndUserSecurityContext{DatabaseAccessToken: "db", EndUserToken: "user", DataRoles: []string{"reader", "admin"}}}
			registry := common.NewSafeRegistry[providers.Provider]()
			registry.Register(provider)
			conn.shelf.registerProviderRegistry(registry)
			streamer := &mockStreamer{pullMsg: &mockOer{}}
			conn.shelf.RegisterMessageStreamer(streamer)
			query := newStatementExecutorSelect()
			query.SetShelf(conn.shelf)
			query.resultMetadata.replace([]columnContext{{DataType: DtyVCS}})
			exec := newStatementExecutorDML()
			exec.SetShelf(conn.shelf)
			runCtx := context.Background()
			run := func() (*ttiOXSCTX, error) {
				streamer.pushedMsg.Init()
				messages := []driverCommon.Message[driverCommon.MessageType]{&dummyMsg{}}
				var err error
				if operation == "query" {
					_, _, err = query.runQuery(runCtx, messages)
				} else {
					_, err = exec.runExec(runCtx, messages)
				}
				entry := streamer.pushedMsg.Front()
				if entry == nil {
					return nil, err
				}
				msg, _ := (*entry.Value.(*driverCommon.Message[driverCommon.MessageType])).(*ttiOXSCTX)
				return msg, err
			}
			expectedHash := "6obRaUlghHiYSs8+duqfcTdN0ysH2PcuyW52xppckWM="
			full := map[string]any{"ver": "1.0", "database_access_token": "db", "end_user_token": "user", "data_roles": []any{"admin", "reader"}}
			// the first successful use sends the hash with the full identity
			msg, err := run()
			if err != nil {
				t.Fatal(err)
			}
			ddsAssertHashMessage(t, msg, expectedHash, full)
			// the next use retains the hash and omits unchanged credentials
			msg, err = run()
			if err != nil {
				t.Fatal(err)
			}
			ddsAssertHashMessage(t, msg, expectedHash, map[string]any{"ver": "1.0"})

			// changed attributes require a full context and remain in reduced sends
			doc, err := json.NewJSON(map[string]any{"department": "sales"})
			if err != nil {
				t.Fatal(err)
			}
			object, err := doc.AsJSONObject()
			if err != nil {
				t.Fatal(err)
			}
			provider.selected.Attributes = map[string]json.JSONObject{"APP.CTX": object}
			full["attributes"] = []any{map[string]any{"name": "APP.CTX", "values": map[string]any{"department": "sales"}}}
			msg, err = run()
			if err != nil {
				t.Fatal(err)
			}
			ddsAssertHashMessage(t, msg, expectedHash, full)
			msg, err = run()
			if err != nil {
				t.Fatal(err)
			}
			ddsAssertHashMessage(t, msg, expectedHash, map[string]any{"ver": "1.0", "attributes": full["attributes"]})

			// a refreshed token must replace the full context and its hash
			provider.selected.EndUserToken = "refreshed"
			refreshed := make(map[string]any, len(full))
			for key, value := range full {
				refreshed[key] = value
			}
			refreshed["end_user_token"] = "refreshed"
			msg, err = run()
			if err != nil {
				t.Fatal(err)
			}
			ddsAssertHashMessage(t, msg, secCtxHash(provider.selected), refreshed)
			provider.selected.EndUserToken = "user"
			msg, err = run()
			if err != nil {
				t.Fatal(err)
			}
			ddsAssertHashMessage(t, msg, expectedHash, full)

			// changed roles must be included in a new full context
			provider.selected.DataRoles = []string{"manager", "admin"}
			changedRoles := make(map[string]any, len(full))
			for key, value := range full {
				changedRoles[key] = value
			}
			changedRoles["data_roles"] = []any{"admin", "manager"}
			msg, err = run()
			if err != nil {
				t.Fatal(err)
			}
			ddsAssertHashMessage(t, msg, "JXsAmQHfJ1xp1wNMZHZBP+zIs7S/ubKu5uEgrOEvtJM=", changedRoles)
			provider.selected.DataRoles = []string{"reader", "admin"}
			msg, err = run()
			if err != nil {
				t.Fatal(err)
			}
			ddsAssertHashMessage(t, msg, expectedHash, full)

			// a cancelled request cannot leave reusable state
			cancelled, cancel := context.WithCancel(context.Background())
			cancel()
			runCtx = cancelled
			_, _ = run()
			runCtx = context.Background()
			if state.previousContext != nil {
				t.Fatal("cancelled call retained the previous context")
			}
			msg, err = run()
			if err != nil {
				t.Fatal(err)
			}
			ddsAssertHashMessage(t, msg, expectedHash, full)

			// a failed call forces a full context on the next call
			for _, stage := range []string{"push", "flush", "pull", "server"} {
				failure := common.NewOracleError(oracleErrors.InternalError, nil)
				switch stage {
				case "push":
					streamer.pushErr = failure
				case "flush":
					streamer.flushErr = failure
				case "pull":
					streamer.pullErr = failure
				case "server":
					streamer.pullMsg = &mockOer{err: failure}
				}
				if _, err = run(); err == nil || state.previousContext != nil {
					t.Fatalf("%s failure left reusable state: %v", stage, err)
				}
				streamer.pushErr, streamer.flushErr, streamer.pullErr = nil, nil, nil
				streamer.pullMsg = &mockOer{}
				msg, err = run()
				if err != nil {
					t.Fatal(err)
				}
				ddsAssertHashMessage(t, msg, expectedHash, full)
			}

			// a no data response is treated conservatively as an uncertain result
			if operation == "query" {
				streamer.pullMsg = &mockOer{err: common.NewOracleError(oracleErrors.NoDataFound, nil)}
				if _, err = run(); !isNoDataFoundError(err) || state.previousContext != nil {
					t.Fatalf("no data left reusable state: %v", err)
				}
				streamer.pullMsg = &mockOer{}
				msg, err = run()
				if err != nil {
					t.Fatal(err)
				}
				ddsAssertHashMessage(t, msg, expectedHash, full)
			}

			// explicit clear and session reset forget prior optimization state
			conn.ClearEndUserSecurityContext()
			msg, err = run()
			if err != nil {
				t.Fatal(err)
			}
			ddsAssertHashMessage(t, msg, expectedHash, full)
			if err := conn.ResetSession(context.Background()); err != nil {
				t.Fatal(err)
			}
			msg, err = run()
			if err != nil {
				t.Fatal(err)
			}
			ddsAssertHashMessage(t, msg, expectedHash, full)

			// a call without a provider forgets the previous context
			conn.shelf.registerProviderRegistry(nil)
			if _, err = run(); err != nil || state.previousContext != nil {
				t.Fatalf("call without context retained state: %v", err)
			}
			conn.shelf.registerProviderRegistry(registry)
			msg, err = run()
			if err != nil {
				t.Fatal(err)
			}
			ddsAssertHashMessage(t, msg, expectedHash, full)

			// named users and servers without hash support receive only full context
			provider.selected = osql.EndUserSecurityContext{DatabaseAccessToken: "db", NamedUser: osql.NamedEndUser{Username: "emma"}}
			msg, err = run()
			if err != nil {
				t.Fatal(err)
			}
			ddsAssertPayload(t, msg, map[string]any{"ver": "1.0", "database_access_token": "db", "end_user_name": "emma"})
			state.hashSupported = false
			provider.selected = osql.EndUserSecurityContext{DatabaseAccessToken: "db"}
			msg, err = run()
			if err != nil {
				t.Fatal(err)
			}
			ddsAssertPayload(t, msg, map[string]any{"ver": "1.0", "database_access_token": "db"})
		})
	}
}
