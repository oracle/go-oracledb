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

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/session"
)

// oexfenGoldenPayload extracts the TTC OEXFEN function payload (post header)
// from hex dump using the 11-byte offset convention used across TTIFUN tests.
func oexfenGoldenPayload(lines []string) []byte {
	buf, _ := ExtractBytesFromDump(lines)
	if len(buf) <= 11 {
		return []byte{}
	}
	return buf[11:24]
}

func newOexfenEngine(capacity int) (*ArrayBasedDataBuffer, *MarshalEngine) {
	buf := NewArrayDataBuffer(capacity)
	// Match representation profile used by other TTC marshalling tests
	engine := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	return buf, engine
}

func buildOexfen(commit bool) *tTIOexfen {
	m := newOexfen18().(*tTIOexfen)
	m.setCursorId(8) // as per dump
	m.setFetchSize() // 2_000_000_000 = 0x77 35 94 00 per dump
	if commit {
		m.setOptions(commitAfterExecution)
	} else {
		m.setOptions(0)
	}
	return m
}

// ============ Constructor/Getters ============

func TestOexfen_New_Getters(t *testing.T) {
	t.Parallel()
	// 18+ header variant
	msg := newOexfen18()
	if msg == nil {
		t.Fatal("newOexfen18 returned nil")
	}
	if msg.GetMsgCode() != TTIFUN {
		t.Errorf("expected TTIFUN, got %v", msg.GetMsgCode())
	}
	if msg.(interface{ GetFuncCode() common.FunctionType }).GetFuncCode() != oExfen {
		t.Errorf("expected oExfen, got %v", msg.(interface{ GetFuncCode() common.FunctionType }).GetFuncCode())
	}

	// pre-18 header variant (smoke)
	msg2 := newOexfen()
	if msg2 == nil {
		t.Fatal("newOexfen returned nil")
	}
	if msg2.GetMsgCode() != TTIFUN {
		t.Errorf("expected TTIFUN, got %v", msg2.GetMsgCode())
	}
}

// ============ Positive: golden match (commit-on-success set) ============

func TestOexfen_MarshalTo_MatchesGolden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := buildOexfen(false)

	buf, engine := newOexfenEngine(256)
	if err := m.MarshalTo(ctx, engine); err != nil {
		t.Fatalf("MarshalTo failed: %v", err)
	}
	got := buf.bytes[:buf.currentWritePosition]
	want := oexfenGoldenPayload(oexfenMarshalToDump)
	if len(want) == 0 {
		t.Fatal("golden OEXFEN payload decode returned empty")
	}
	if !bytes.Equal(got, want) {
		t.Logf("Got OEXFEN packet:")
		session.PrintPacket(got, 0, len(got))
		t.Logf("Want OEXFEN packet:")
		session.PrintPacket(want, 0, len(want))
		t.Fatalf("OEXFEN mismatch:\n got (%d bytes): % X\nwant (%d bytes): % X", len(got), got, len(want), want)
	}
}

// ============ Positive: exeflg branches (commit vs no-commit) ============

// decodeOexfenExeflg scans the marshalled OEXFEN payload, skipping the
// TTIFUN18 header (3 bytes), then decoding:
//   - cursorId (SB4 UNIVERSAL)
//   - iterations/exenit (UB4 UNIVERSAL)
//   - exerof (UB4 UNIVERSAL)
//
// and returns the final exeflg (UB4 UNIVERSAL as int).
func decodeOexfenExeflg(pkt []byte) (int, bool) {
	if len(pkt) < 3 {
		return 0, false
	}
	idx := 3 // skip ttiFunHeader18 header: func, seq, token

	// cursorId SB4 (UNIVERSAL)
	_, n, ok := decodeUniversalAt(pkt, idx)
	if !ok {
		return 0, false
	}
	idx += n

	// iterations UB4 (UNIVERSAL)
	_, n, ok = decodeUniversalAt(pkt, idx)
	if !ok {
		return 0, false
	}
	idx += n

	// exerof UB4 (UNIVERSAL)
	_, n, ok = decodeUniversalAt(pkt, idx)
	if !ok {
		return 0, false
	}
	idx += n

	// exeflg UB4 (UNIVERSAL)
	val, _, ok := decodeUniversalAt(pkt, idx)
	return val, ok
}

func TestOexfen_MarshalTo_Exeflg_NoCommit_And_Commit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("NoCommit", func(t *testing.T) {
		m := buildOexfen(false)
		buf, engine := newOexfenEngine(256)
		if err := m.MarshalTo(ctx, engine); err != nil {
			t.Fatalf("MarshalTo failed: %v", err)
		}
		got := buf.bytes[:buf.currentWritePosition]
		if val, ok := decodeOexfenExeflg(got); !ok {
			t.Fatal("failed to decode exeflg (no-commit)")
		} else if val != 0 {
			t.Fatalf("expected exeflg 0 (no commit), got %d", val)
		}
	})

	t.Run("Commit", func(t *testing.T) {
		m := buildOexfen(true)
		buf, engine := newOexfenEngine(256)
		if err := m.MarshalTo(ctx, engine); err != nil {
			t.Fatalf("MarshalTo failed: %v", err)
		}
		got := buf.bytes[:buf.currentWritePosition]
		if val, ok := decodeOexfenExeflg(got); !ok {
			t.Fatal("failed to decode exeflg (commit)")
		} else if val != 1 { // _exeCommitOnSuccess == 1
			t.Fatalf("expected exeflg 1 (commit-on-success), got %d", val)
		}
	})
}

// ============ Negative: faulty data buffer (deterministic capacity-based failures) ============

func TestOexfen_MarshalTo_Failures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Fail header via WriteByte injection.
	t.Run("Fail_Header_WriteByte", func(t *testing.T) {
		m := buildOexfen(true)
		faulty := &FaultyArrayBasedDataBuffer{
			ArrayBasedDataBuffer: NewArrayDataBuffer(64),
			FailOnWriteByteCall:  1, // first WriteByte (func code) fails
		}
		engine := NewMarshalEngine(faulty, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
		err := m.MarshalTo(ctx, engine)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Failed to marshal message") {
			t.Fatalf("expected header error, got %v", err)
		}
	})

	// Capacity-based failures to hit subsequent stages exactly:
	// Header (3 bytes), cursorId (universal {0x01,0x04} => 2 bytes),
	// iterations (universal {0x04,0x77,0x35,0x94,0x00} => 5 bytes),
	// exerof (universal {0x01,0x60} => 2 bytes), exeflg (universal {0x01,0x01} or {0x00})
	tests := []struct {
		name     string
		capacity int
		wantSub  string
	}{
		{"Fail_Cursor", 3, "Failed to marshal message"},
		{"Fail_Iterations", 5, "Failed to marshal message"},
		{"Fail_Exerof", 10, "Failed to marshal message"},
		{"Fail_Exeflg", 12, "Failed to marshal message"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := buildOexfen(true) // commit -> exeflg universal {0x01,0x01}
			buf := NewArrayDataBuffer(tc.capacity)
			engine := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
			err := m.MarshalTo(ctx, engine)
			if err == nil {
				t.Fatalf("expected error, got nil (capacity=%d)", tc.capacity)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("expected %q, got %v", tc.wantSub, err)
			}
		})
	}
}
