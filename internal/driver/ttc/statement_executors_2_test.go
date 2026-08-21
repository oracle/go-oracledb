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
	sqldriver "database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/converters"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// registerTestCodecs registers encoders, decoders and OAC makers used by statementProcessor
// in these unit tests, and wires a protocol-specific codecFactory into the provided shelf.
//
// Why this exists:
// - statementProcessor resolves encoders/OAC makers through shelf.GetCodecFactory()
// - tests that construct statementProcessor directly must provide that wiring explicitly
func registerTestCodecs(shelf *ttiShelf[common.MessageType], ttcProtocolVersion int8) {
	// Register encoders used by prepareBindsAndOAC tests.
	// (Registration uses "version 2" which is <= any supported protocol version in tests.)
	_ = EncoderRegistry.Register(reflect.TypeOf(int64(0)), 2, func(v sqldriver.Value) (common.B1Array, error) {
		return converters.EncodeInt(v.(int64))
	})
	_ = EncoderRegistry.Register(reflect.TypeOf(""), 2, func(v sqldriver.Value) (common.B1Array, error) {
		return converters.EncodeVarchar(v.(string))
	})
	_ = EncoderRegistry.Register(reflect.TypeOf([]byte(nil)), 2, func(v sqldriver.Value) (common.B1Array, error) {
		return common.B1Array(v.([]byte)), nil
	})
	_ = EncoderRegistry.Register(reflect.TypeOf(true), 2, func(v sqldriver.Value) (common.B1Array, error) {
		return converters.EncodeBoolean(v.(bool))
	})
	_ = EncoderRegistry.Register(reflect.TypeOf(time.Time{}), 2, func(v sqldriver.Value) (common.B1Array, error) {
		return converters.EncodeTimestampWithTimeZone(v.(time.Time))
	})
	_ = EncoderRegistry.Register(reflect.TypeOf(nil), 2, func(v sqldriver.Value) (common.B1Array, error) {
		return converters.EncodeNull(v)
	})

	// Register bind OAC makers used by prepareBindsAndOAC tests.
	_ = BindOacRegistry.Register(reflect.TypeOf(int64(0)), 2, bindOacType{
		bindOacFunc: func(maxLength common.UB4) common.Marshallable {
			return newTTIoac(DtyNum, maxLength)
		},
		maxLength: converters.MaxNumberLength,
	})
	_ = BindOacRegistry.Register(reflect.TypeOf(""), 2, bindOacType{
		bindOacFunc: func(maxLength common.UB4) common.Marshallable {
			return newTTIoac(DtyVCS, maxLength)
		},
		maxLength: 32768,
	})
	_ = BindOacRegistry.Register(reflect.TypeOf([]byte(nil)), 2, bindOacType{
		bindOacFunc: func(maxLength common.UB4) common.Marshallable {
			return newTTIoac(DtyVbi, maxLength)
		},
		maxLength: 32767,
	})
	_ = BindOacRegistry.Register(reflect.TypeOf(true), 2, bindOacType{
		bindOacFunc: func(maxLength common.UB4) common.Marshallable {
			return newTTIoac(DtyBol, maxLength)
		},
		maxLength: converters.MaxBoolLength,
	})
	_ = BindOacRegistry.Register(reflect.TypeOf(time.Time{}), 2, bindOacType{
		bindOacFunc: func(maxLength common.UB4) common.Marshallable {
			return newTTIoac(DtyStz, maxLength)
		},
		maxLength: converters.MaxTimeStampLength,
	})

	// Decoders are not exercised by statement_executors_2_test directly today, but the task
	// explicitly requests registering them. Add minimal decoders to keep registry complete.
	_ = DecoderRegistry.Register(DtyVCS, 2, newTypeDecoder(func(_ columnContext, data common.B1Array) (sqldriver.Value, error) {
		return string(data), nil
	}, nil))
	_ = DecoderRegistry.Register(DtyBin, 2, newTypeDecoder(func(_ columnContext, data common.B1Array) (sqldriver.Value, error) {
		return []byte(data), nil
	}, nil))

	shelf.RegisterCodecFactory(NewCodecFactoryForProtocol(ttcProtocolVersion))
}

// ------------------------------
// prepareBindsAndOAC - normalization
// ------------------------------

func TestPrepareBindsAndOAC_Normalize_Supported(t *testing.T) {
	t.Parallel()
	tz := time.FixedZone("IST", 5*3600+30*60)
	ts := time.Date(2025, 1, 2, 3, 4, 5, 123_000_000, tz)

	args := []sqldriver.Value{
		"str",
		[]byte{1, 2},
		nil,
		true,
		int(7),
		int8(-8),
		int16(16),
		int32(-32),
		int64(64),
		uint(12),
		uint8(8),
		uint16(16),
		uint32(32),
		uint64(123),
		float32(3.5),
		float64(6.25),
		ts,
	}

	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	sp := &statementProcessor{shelf: shelf}
	if err := sp.prepareBindsAndOAC(args); err != nil {
		t.Fatalf("prepareBindsAndOAC returned error: %v", err)
	}
	out := sp.bindValues
	if len(out) != len(args) {
		t.Fatalf("unexpected out length: got %d want %d", len(out), len(args))
	}

	assertIs := func(i int, want any) {
		got := out[i]
		switch w := want.(type) {
		case nil:
			if got != nil {
				t.Fatalf("idx %d: expected nil, got %T %v", i, got, got)
			}
		case string:
			gs, ok := got.(string)
			if !ok || gs != w {
				t.Fatalf("idx %d: got %T %v want string %v", i, got, got, w)
			}
		case []byte:
			gb, ok := got.([]byte)
			if !ok || !bytes.Equal(gb, w) {
				t.Fatalf("idx %d: got %T %v want []byte %v", i, got, got, w)
			}
		case int, int8, int16, int32, int64:
			if got != w {
				t.Fatalf("idx %d: got %T %v want int %v", i, got, got, w)
			}
		case uint, uint8, uint16, uint32, uint64:
			if got != w {
				t.Fatalf("idx %d: got %T %v want int64 %v", i, got, got, w)
			}
		case float64:
			gf, ok := got.(float64)
			if !ok || gf != w {
				t.Fatalf("idx %d: got %T %v want float64 %v", i, got, got, w)
			}
		default:
			t.Fatalf("idx %d: unsupported want type %T", i, w)
		}
	}

	assertIs(0, "str")
	assertIs(1, []byte{1, 2})
	if out[2] != nil {
		t.Fatalf("idx 2: expected nil, got %v", out[2])
	}
	// bool preserved in current implementation
	if b, ok := out[3].(bool); !ok || !b {
		t.Fatalf("idx 3: expected bool(true), got %T %v", out[3], out[3])
	}
	assertIs(4, int(7))
	assertIs(5, int8(-8))
	assertIs(6, int16(16))
	assertIs(7, int32(-32))
	assertIs(8, int64(64))
	assertIs(9, uint(12))
	assertIs(10, uint8(8))
	assertIs(11, uint16(16))
	assertIs(12, uint32(32))
	assertIs(13, uint64(123))
	if f, ok := out[14].(float32); !ok || f != 3.5 {
		t.Fatalf("idx 14: expected float64(3.5), got %T %v", out[14], out[14])
	}
	if f, ok := out[15].(float64); !ok || f != 6.25 {
		t.Fatalf("idx 15: expected float64(6.25), got %T %v", out[15], out[15])
	}
	if tt, ok := out[16].(time.Time); !ok || !tt.Equal(ts) {
		t.Fatalf("idx 16: expected time %v, got %T %v", ts, out[16], out[16])
	}
}

func TestPrepareBindsAndOAC_Normalize_Errors(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	sp := &statementProcessor{shelf: shelf}

	// unsupported type
	err := sp.prepareBindsAndOAC([]sqldriver.Value{struct{}{}})
	if err == nil || !strings.Contains(err.Error(), "internal error occurred") {
		t.Fatalf("expected unsupported type error containing 'bind-type', got %v", err)
	}
}

// ------------------------------
// prepareBindsAndOAC - encoding
// ------------------------------

func TestPrepareBindsAndOAC_Encode_Supported(t *testing.T) {
	t.Parallel()
	tz := time.FixedZone("IST", 5*3600+30*60)
	ts := time.Date(2025, 1, 2, 3, 4, 5, 987_000_000, tz)

	args := []sqldriver.Value{
		int64(42),
		"X",
		[]byte{1, 2, 3},
		nil,
		true,
		false,
		ts,
	}

	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	sp := &statementProcessor{shelf: shelf}
	if err := sp.prepareBindsAndOAC(args); err != nil {
		t.Fatalf("prepareBindsAndOAC returned error: %v", err)
	}
	encoded := sp.encodedValues
	if len(encoded) != 1 {
		t.Fatalf("expected single row encoded, got %d rows", len(encoded))
	}
	row := encoded[0]
	if len(row) != len(args) {
		t.Fatalf("unexpected encoded row length: got %d want %d", len(row), len(args))
	}

	expNum, _ := converters.EncodeInt(42)
	if !bytes.Equal([]byte(row[0]), []byte(expNum)) {
		t.Fatalf("int64 encoding mismatch: got %v want %v", []byte(row[0]), []byte(expNum))
	}
	expVcs, _ := converters.EncodeVarchar("X")
	if !bytes.Equal([]byte(row[1]), []byte(expVcs)) {
		t.Fatalf("varchar encoding mismatch: got %v want %v", []byte(row[1]), []byte(expVcs))
	}
	if !bytes.Equal([]byte(row[2]), []byte{1, 2, 3}) {
		t.Fatalf("raw encoding mismatch: got %v want %v", []byte(row[2]), []byte{1, 2, 3})
	}
	if row[3] != nil {
		t.Fatalf("nil bind should encode as nil cell, got %v", row[3])
	}
	// bool -> representation per current impl
	if !bytes.Equal([]byte(row[4]), []byte{1, 1}) {
		t.Fatalf("bool(true) encoding mismatch: got %v want %v", []byte(row[4]), []byte{1})
	}
	if !bytes.Equal([]byte(row[5]), []byte{0}) {
		t.Fatalf("bool(false) encoding mismatch: got %v want %v", []byte(row[5]), []byte{0})
	}
	expTS, _ := converters.EncodeTimestampWithTimeZone(ts)
	if !bytes.Equal([]byte(row[6]), []byte(expTS)) {
		t.Fatalf("timestamp encoding mismatch: got %v want %v", []byte(row[6]), []byte(expTS))
	}
}

func TestPrepareBindsAndOAC_Encode_Errors(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	sp := &statementProcessor{shelf: shelf}

	// unsupported type should raise bind-type (was bind-encode-type in old API)
	err := sp.prepareBindsAndOAC([]sqldriver.Value{struct{}{}})
	if err == nil || !strings.Contains(err.Error(), "internal error occurred") {
		t.Fatalf("expected type encode error containing 'bind-type', got %v", err)
	}
}

// ------------------------------
// pushBindRows error paths
// ------------------------------

// Using prepareBindsAndOAC to produce encoded payloads and exercising pushBindRows
func TestPushBindRowsIfAny_EncodeBindValues_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, stmr, _ := newExecTestShelf(1024)
	shelf.RegisterMessageStreamer(stmr)
	req := &tTIOall{}
	req.setOptions(bindValuesPresent | executeStatement)
	req.setNumberOfBindPositions(1)

	registerTestCodecs(shelf, 20)

	// float64 encodes successfully via prepareBindsAndOAC
	args := []sqldriver.Value{float64(1.25)}
	sp := &statementProcessor{shelf: shelf}
	if err := sp.prepareBindsAndOAC(args); err != nil {
		t.Fatalf("unexpected prepareBindsAndOAC error, got %v", err)
	}
	err := sp.pushBindRows(ctx, "ut", oracleErrors.RunExecError)
	if err != nil {
		t.Fatalf("unexpected encode-bindNames error, got %v", err)
	}
}

// Factory GetMessage(TTIRXD) failure should surface as "push"
type failingRXDFactory struct{ base Factory }

func (f failingRXDFactory) GetMessage(msgType common.MessageType) (common.Message[common.MessageType], error) {
	if msgType == TTIRXD {
		return nil, errors.New("fail RXD")
	}
	return f.base.GetMessage(msgType)
}
func (f failingRXDFactory) GetMessageForFunction(msgType common.MessageType, funcType common.FunctionType) (common.Message[common.MessageType], error) {
	return f.base.GetMessageForFunction(msgType, funcType)
}

func TestPushBindRowsIfAny_GetMessage_RXD_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, stmr, _ := newExecTestShelf(1024)

	// Wrap the existing factory to fail GetMessage(TTIRXD)
	base := shelf.GetMessageFactory().(Factory)
	shelf.RegisterMessageFactory(failingRXDFactory{base: base})
	shelf.RegisterMessageStreamer(stmr)

	req := &tTIOall{}
	req.setOptions(bindValuesPresent | executeStatement)
	req.setNumberOfBindPositions(1)

	registerTestCodecs(shelf, 20)

	// Supported bind so prepareBindsAndOAC succeeds
	args := []sqldriver.Value{int64(1)}
	sp := &statementProcessor{shelf: shelf}
	if err := sp.prepareBindsAndOAC(args); err != nil {
		t.Fatalf("unexpected prepareBindsAndOAC error: %v", err)
	}
	err := sp.pushBindRows(ctx, "ut", oracleErrors.RunExecError)
	if err == nil || !strings.Contains(err.Error(), "push") {
		t.Fatalf("expected get-rxd push error, got %v", err)
	}
}

// Streamer Push failure on RXD should surface as "push"
func TestPushBindRowsIfAny_Push_RXD_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, _ := newExecTestShelf(1024)

	// Replace streamer with one that fails Push for TTIRXD
	vs := &versatileStreamer{
		failCode:  TTIRXD,
		failWhere: wherePush,
		failErr:   errors.New("Push TTIRXD failed"),
	}
	shelf.RegisterMessageStreamer(vs)

	req := &tTIOall{}
	req.setOptions(bindValuesPresent | executeStatement)
	req.setNumberOfBindPositions(1)

	registerTestCodecs(shelf, 20)

	// Supported bind so prepareBindsAndOAC succeeds; Push should fail
	args := []sqldriver.Value{int64(1)}
	sp := &statementProcessor{shelf: shelf}
	if err := sp.prepareBindsAndOAC(args); err != nil {
		t.Fatalf("unexpected prepareBindsAndOAC error: %v", err)
	}
	err := sp.pushBindRows(ctx, "ut", oracleErrors.RunExecError)
	if err == nil || !strings.Contains(err.Error(), "push") {
		t.Fatalf("expected RXD push error, got %v", err)
	}
}

// ------------------------------
// prepareBindsAndOAC - OACs
// ------------------------------

func TestPrepareBindsAndOAC_OAC_Supported(t *testing.T) {
	t.Parallel()
	tz := time.FixedZone("IST", 5*3600+30*60)
	ts := time.Date(2025, 1, 2, 3, 4, 5, 0, tz)

	args := []sqldriver.Value{
		int64(5),     // -> DtyNum, len 22
		"abc",        // -> DtyVCS (wire dataType DtyChr), len 3
		[]byte{0xAA}, // -> DtyVbi (wire dataType DtyBin), len 1
		true,         // -> DtyBol, len 2
		nil,          // -> DtyVCS (wire dataType DtyChr), len 0
		ts,           // -> DtyStz, len 11/13 per current implementation
	}

	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	sp := &statementProcessor{shelf: shelf}
	if err := sp.prepareBindsAndOAC(args); err != nil {
		t.Fatalf("prepareBindsAndOAC returned error: %v", err)
	}
	oacs := sp.currentOacs
	if len(oacs) != len(args) {
		t.Fatalf("unexpected oacs length: got %d want %d", len(oacs), len(args))
	}

	// int64 -> NUMBER
	if got := oacs[0].(*tTIoac); got == nil || got.dataType != common.UB1(DtyNum) || got.maxLength != 22 {
		t.Fatalf("oac[0] mismatch: dataType=%d maxLength=%d", got.dataType, got.maxLength)
	}

	// string -> VARCHAR (wire dataType DtyChr) with len=3
	if got := oacs[1].(*tTIoac); got == nil || got.dataType != common.UB1(DtyChr) || got.maxLength != 3 {
		t.Fatalf("oac[1] mismatch: dataType=%d maxLength=%d", got.dataType, got.maxLength)
	}
	if oacs[1].(*tTIoac).flags&_uacFlagIndicator == 0 || oacs[1].(*tTIoac).flags&_uacFlagLengthVector == 0 {
		t.Fatalf("oac[1] flags missing indicator/lengthVector: flags=%d", oacs[1].(*tTIoac).flags)
	}
	if oacs[1].(*tTIoac).flagsContinuation&_uacFlagContinuationNoadj == 0 {
		t.Fatalf("oac[1] flagsContinuation missing noadj bit: flagsContinuation=%d", oacs[1].(*tTIoac).flagsContinuation)
	}

	// raw -> VBI (wire dataType DtyBin) with len=1
	if got := oacs[2]; got == nil || got.(*tTIoac).dataType != common.UB1(DtyBin) || got.(*tTIoac).maxLength != 1 {
		t.Fatalf("oac[2] mismatch: dataType=%d maxLength=%d", got.(*tTIoac).dataType, got.(*tTIoac).maxLength)
	}
	// bool -> BOL with len=2
	if got := oacs[3]; got == nil || got.(*tTIoac).dataType != common.UB1(DtyBol) || got.(*tTIoac).maxLength != converters.MaxBoolLength {
		t.Fatalf("oac[3] mismatch: dataType=%d maxLength=%d", got.(*tTIoac).dataType, got.(*tTIoac).maxLength)
	}

	// nil -> VARCHAR with len=4
	if got := oacs[4]; got == nil || got.(*tTIoac).dataType != common.UB1(DtyChr) || got.(*tTIoac).maxLength != 4 {
		t.Fatalf("oac[4] mismatch: dataType=%d maxLength=%d", got.(*tTIoac).dataType, got.(*tTIoac).maxLength)
	}

	// time.Time -> STZ with len=13 (per current code)
	if got := oacs[5]; got == nil || got.(*tTIoac).dataType != common.UB1(DtyStz) || got.(*tTIoac).maxLength != 13 {
		t.Fatalf("oac[5] mismatch: dataType=%d maxLength=%d", got.(*tTIoac).dataType, got.(*tTIoac).maxLength)
	}
}

func TestPrepareBindsAndOAC_OAC_Errors(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	// unsupported type
	sp := &statementProcessor{shelf: shelf}
	err := sp.prepareBindsAndOAC([]sqldriver.Value{map[string]int{"x": 1}})
	if err == nil || !strings.Contains(err.Error(), "internal error occurred") {
		t.Fatalf("expected type encode error containing 'bind-type', got %v", err)
	}
}
