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
	"context"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// authPacketDump is the decoded, comparison-oriented representation of an
// OSESSKEY or OAUTH request used by the golden tests in this package.
//
// It retains the stable PISDEF/header fields and the ordered authentication
// key/value list. The username bytes and their length are intentionally not
// retained because they come from the current OS user and may legitimately
// differ from the machine on which a golden packet was captured.
type authPacketDump struct {
	funcCode      common.UB1
	seqNo         common.UB1
	token         common.UB8
	hasUser       bool
	logonMode     common.UB4
	authivlPtr    common.UB1
	keyValueCount int
	authovlPtr    common.UB1
	authovlnPtr   common.UB1
	keyValues     []*common.KeyValue
}

// defaultAuthMarshallerTypes describes the numeric representations used by the
// saved authentication packets. Keep this aligned with the marshaller passed to
// OSESSKEY and OAUTH MarshalTo calls before parsing either an actual packet or a
// golden packet with parseAuthPacketDump.
var defaultAuthMarshallerTypes = [5]byte{Native, Universal, Universal, Universal, Universal}

// parseLegacyAuthPacketDump decodes one of the historical authentication
// golden arrays without requiring the fixture itself to be changed.
//
// The original tests copied each golden array into a larger zero-filled buffer.
// Consequently, the final zero-valued key/value flag was supplied by buffer
// padding rather than stored in the array. This helper recreates that single
// implicit byte on a private copy and delegates all parsing to
// parseAuthPacketDump.
//
// Use this helper only for the existing validOSesskeyMarshalDump and
// validOauthMarshalDump fixtures. Newly marshaled packets are complete payloads
// and should be passed directly to parseAuthPacketDump.
func parseLegacyAuthPacketDump(t *testing.T, payload []byte) authPacketDump {
	t.Helper()
	paddedPayload := append([]byte(nil), payload...)
	paddedPayload = append(paddedPayload, 0)
	return parseAuthPacketDump(t, paddedPayload)
}

// parseAuthPacketDump decodes a complete TTC authentication request into the
// fields that the golden tests compare.
//
// The decoder reads the function header and PISDEF metadata first, consumes the
// optional username using the length declared by PISDEF, and then unmarshals the
// declared number of key/value pairs with the production keyValueList decoder.
// Username bytes are consumed but discarded because they are runtime-specific.
// All other decoded fields are returned in wire order, and trailing bytes are
// rejected so a malformed or partially decoded packet cannot pass silently.
//
// Pass the exact written portion of a newly marshaled buffer, for example
// buf.bytes[:buf.currentWritePosition]. For legacy golden arrays whose last
// zero flag is implicit, use parseLegacyAuthPacketDump instead. Any decoding
// problem fails t immediately and is attributed to the calling test.
func parseAuthPacketDump(t *testing.T, payload []byte) authPacketDump {
	t.Helper()

	buf := &ArrayBasedDataBuffer{
		bytes:                append([]byte(nil), payload...),
		currentWritePosition: len(payload),
		currentReadPosition:  0,
	}

	engine := NewMarshalEngine(buf, common.BIG_ENDIAN, defaultAuthMarshallerTypes)
	ctx := context.Background()

	funcCode, err := engine.UnmarshalUB1(ctx)
	if err != nil {
		t.Fatalf("unmarshal funcCode failed: %v", err)
	}

	seqNo, err := engine.UnmarshalUB1(ctx)
	if err != nil {
		t.Fatalf("unmarshal seqNo failed: %v", err)
	}

	token, err := engine.UnmarshalUB8(ctx)
	if err != nil {
		t.Fatalf("unmarshal token failed: %v", err)
	}

	userPtr, err := engine.UnmarshalUB1(ctx)
	if err != nil {
		t.Fatalf("unmarshal user ptr failed: %v", err)
	}

	userLenSB4, err := engine.UnmarshalSB4(ctx)
	if err != nil {
		t.Fatalf("unmarshal user len failed: %v", err)
	}
	intUserLen := int(userLenSB4)

	logonMode, err := engine.UnmarshalUB4(ctx)
	if err != nil {
		t.Fatalf("unmarshal logon mode failed: %v", err)
	}

	authivlPtr, err := engine.UnmarshalUB1(ctx)
	if err != nil {
		t.Fatalf("unmarshal authivl ptr failed: %v", err)
	}

	keyValueCountUB4, err := engine.UnmarshalUB4(ctx)
	if err != nil {
		t.Fatalf("unmarshal key-value count failed: %v", err)
	}
	keyValueCount := int(keyValueCountUB4)

	authovlPtr, err := engine.UnmarshalUB1(ctx)
	if err != nil {
		t.Fatalf("unmarshal authovl ptr failed: %v", err)
	}

	authovlnPtr, err := engine.UnmarshalUB1(ctx)
	if err != nil {
		t.Fatalf("unmarshal authovln ptr failed: %v", err)
	}

	if userPtr != 0 {
		if intUserLen < 0 {
			t.Fatalf("negative user len %d", intUserLen)
		}
		// MarshalChar writes raw bytes when TTCLXMULTI is active (the package
		// default), so consume the length declared in the PISDEF directly.
		if _, err := engine.UnmarshalB1Array(ctx, intUserLen); err != nil {
			t.Fatalf("unmarshal user failed: %v", err)
		}
	}

	remaining := buf.currentWritePosition - buf.currentReadPosition
	if remaining == 0 && keyValueCount > 0 {
		t.Fatalf("no bytes left to decode %d key/value pairs", keyValueCount)
	}
	keyValueBytes := make([]byte, remaining)
	copy(keyValueBytes, buf.bytes[buf.currentReadPosition:buf.currentWritePosition])

	payloadBuf := &ArrayBasedDataBuffer{
		bytes:                keyValueBytes,
		currentWritePosition: len(keyValueBytes),
		currentReadPosition:  0,
	}
	engineKV := NewMarshalEngine(payloadBuf, common.BIG_ENDIAN, defaultAuthMarshallerTypes)
	list := newPreallocatedKeyValueList(keyValueCount)
	if err := list.UnMarshalFrom(ctx, engineKV); err != nil {
		t.Fatalf("unmarshal key-value list failed: %v", err)
	}
	if payloadBuf.currentReadPosition != payloadBuf.currentWritePosition {
		t.Fatalf("auth dump has %d trailing bytes", payloadBuf.currentWritePosition-payloadBuf.currentReadPosition)
	}
	keyValues := make([]*common.KeyValue, 0, list.Len())
	for e := list.Front(); e != nil; e = e.Next() {
		keyValues = append(keyValues, e.Value.(*common.KeyValue))
	}

	return authPacketDump{
		funcCode:      funcCode,
		seqNo:         seqNo,
		token:         token,
		hasUser:       userPtr != 0,
		logonMode:     logonMode,
		authivlPtr:    authivlPtr,
		keyValueCount: keyValueCount,
		authovlPtr:    authovlPtr,
		authovlnPtr:   authovlnPtr,
		keyValues:     keyValues,
	}
}

// assertAuthPacketMetadata verifies the stable header and PISDEF contract of
// two decoded authentication packets.
//
// It compares the function, sequence, token, logon mode, pointer presence and
// key/value count. It deliberately compares only whether a username is present,
// not its bytes or encoded length, because those depend on the current OS user.
// Key/value contents are outside this helper; callers should follow it with
// assertKeyValuesMatch to validate the payload.
func assertAuthPacketMetadata(t *testing.T, got, want authPacketDump) {
	t.Helper()

	if got.funcCode != want.funcCode {
		t.Fatalf("func code mismatch: got %d want %d", got.funcCode, want.funcCode)
	}
	if got.seqNo != want.seqNo {
		t.Fatalf("sequence number mismatch: got %d want %d", got.seqNo, want.seqNo)
	}
	if got.token != want.token {
		t.Fatalf("token mismatch: got %d want %d", got.token, want.token)
	}
	if got.logonMode != want.logonMode {
		t.Fatalf("logon mode mismatch: got %d want %d", got.logonMode, want.logonMode)
	}
	if got.hasUser != want.hasUser {
		t.Fatalf("user pointer mismatch: got %v want %v", got.hasUser, want.hasUser)
	}
	if got.authivlPtr != want.authivlPtr {
		t.Fatalf("authivl pointer mismatch: got %d want %d", got.authivlPtr, want.authivlPtr)
	}
	if got.keyValueCount != want.keyValueCount {
		t.Fatalf("key-value count mismatch: got %d want %d", got.keyValueCount, want.keyValueCount)
	}
	if got.authovlPtr != want.authovlPtr {
		t.Fatalf("authovl pointer mismatch: got %d want %d", got.authovlPtr, want.authovlPtr)
	}
	if got.authovlnPtr != want.authovlnPtr {
		t.Fatalf("authovln pointer mismatch: got %d want %d", got.authovlnPtr, want.authovlnPtr)
	}
}

// assertKeyValuesMatch compares two decoded authentication key/value lists in
// wire order while allowing selected values to vary across test environments.
//
// List length, key name, key position and flag are always checked. A key present
// in skipValues skips only its Value comparison; the key and flag remain fully
// validated. Callers should therefore include only fields whose values are
// inherently runtime- or cryptography-dependent, such as AUTH_MACHINE,
// AUTH_PID, AUTH_PASSWORD or AUTH_SESSKEY. Stable protocol values should not be
// skipped, so the golden test continues to detect meaningful regressions.
func assertKeyValuesMatch(t *testing.T, got, want []*common.KeyValue, skipValues map[string]struct{}) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("key-value length mismatch: got %d want %d", len(got), len(want))
	}

	for i := range want {
		gk := common.B1ArrayToString(got[i].Key)
		wk := common.B1ArrayToString(want[i].Key)

		if gk != wk {
			t.Fatalf("key mismatch at index %d: got %s want %s", i, gk, wk)
		}

		if got[i].Flag != want[i].Flag {
			t.Fatalf("flag mismatch for key %s: got %d want %d", wk, got[i].Flag, want[i].Flag)
		}

		if _, ok := skipValues[wk]; !ok && !got[i].Value.Equals(want[i].Value) {
			t.Fatalf("value mismatch for key %s: got %v want %v", wk, got[i].Value, want[i].Value)
		}
	}
}

// skipKeySet builds the value-skip set consumed by assertKeyValuesMatch.
//
// Pass authentication key constants rather than literal strings so renames stay
// coupled to the production protocol definitions. An empty call returns an
// empty set and causes every value to be compared.
func skipKeySet(keys ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		set[key] = struct{}{}
	}
	return set
}
