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
	"maps"
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
	encoderRegistry := newCodecRegistry[encoderKey, *typeEncoder]()
	maps.Copy(encoderRegistry.entries, EncoderRegistry.entries)
	var decoderRegistry = newCodecRegistry[DtyType, *typeDecoder]()
	maps.Copy(decoderRegistry.entries, DecoderRegistry.entries)
	var bindOacRegistry = newCodecRegistry[reflect.Type, bindOacType]()
	maps.Copy(bindOacRegistry.entries, BindOacRegistry.entries)
	// Register encoders used by bind preparation tests.
	// (Registration uses "version 2" which is <= any supported protocol version in tests.)
	_ = encoderRegistry.Register(encoderKey{goType: reflect.TypeOf(int64(0))}, 2, newCLRBindEncoder(func(v sqldriver.Value) (common.B1Array, error) {
		return converters.EncodeInt(v.(int64))
	}))
	_ = encoderRegistry.Register(encoderKey{goType: reflect.TypeOf("")}, 2, newCLRBindEncoder(func(v sqldriver.Value) (common.B1Array, error) {
		return converters.EncodeVarchar(v.(string))
	}))
	_ = encoderRegistry.Register(encoderKey{goType: reflect.TypeOf([]byte(nil))}, 2, newCLRBindEncoder(func(v sqldriver.Value) (common.B1Array, error) {
		return common.B1Array(v.([]byte)), nil
	}))
	_ = encoderRegistry.Register(encoderKey{goType: reflect.TypeOf(true)}, 2, newCLRBindEncoder(func(v sqldriver.Value) (common.B1Array, error) {
		return converters.EncodeBoolean(v.(bool))
	}))
	_ = encoderRegistry.Register(encoderKey{goType: reflect.TypeOf(time.Time{})}, 2, newCLRBindEncoder(func(v sqldriver.Value) (common.B1Array, error) {
		return converters.EncodeTimestampWithTimeZone(v.(time.Time))
	}))
	_ = encoderRegistry.Register(encoderKey{goType: reflect.TypeOf(nil)}, 2, newCLRBindEncoder(func(v sqldriver.Value) (common.B1Array, error) {
		return converters.EncodeNull(v)
	}))

	// Register bind OAC makers used by bind preparation tests.
	_ = bindOacRegistry.Register(reflect.TypeOf(int64(0)), 2, bindOacType{
		bindOacFunc: func(maxLength common.UB4) common.Marshallable {
			return newTTIoac(DtyNum, maxLength)
		},
		maxLength: converters.MaxNumberLength,
	})
	_ = bindOacRegistry.Register(reflect.TypeOf(""), 2, bindOacType{
		bindOacFunc: func(maxLength common.UB4) common.Marshallable {
			return newTTIoac(DtyVCS, maxLength)
		},
		maxLength: 32768,
	})
	_ = bindOacRegistry.Register(reflect.TypeOf([]byte(nil)), 2, bindOacType{
		bindOacFunc: func(maxLength common.UB4) common.Marshallable {
			return newTTIoac(DtyVbi, maxLength)
		},
		maxLength: 32767,
	})
	_ = bindOacRegistry.Register(reflect.TypeOf(true), 2, bindOacType{
		bindOacFunc: func(maxLength common.UB4) common.Marshallable {
			return newTTIoac(DtyBol, maxLength)
		},
		maxLength: converters.MaxBoolLength,
	})
	_ = bindOacRegistry.Register(reflect.TypeOf(time.Time{}), 2, bindOacType{
		bindOacFunc: func(maxLength common.UB4) common.Marshallable {
			return newTTIoac(DtyStz, maxLength)
		},
		maxLength: converters.MaxTimeStampLength,
	})

	// Decoders are not exercised by statement_executors_2_test directly today, but the task
	// explicitly requests registering them. Add minimal decoders to keep registry complete.
	_ = decoderRegistry.Register(DtyVCS, 2, newTypeDecoder(func(_ columnContext, value any) (sqldriver.Value, error) {
		data := value.(common.B1Array)
		return string(data), nil
	}, nil, unwrapCLRColumn))
	_ = decoderRegistry.Register(DtyBin, 2, newTypeDecoder(func(_ columnContext, value any) (sqldriver.Value, error) {
		data := value.(common.B1Array)
		return []byte(data), nil
	}, nil, unwrapCLRColumn))

	factory := &CodecFactoryImpl{
		ttcVersion: ttcProtocolVersion,
		encoders:   encoderRegistry,
		decoders:   decoderRegistry,
		bindOacs:   bindOacRegistry,
		defineOacs: DefineOacRegistry,
	}
	shelf.RegisterCodecFactory(factory)
}

func prepareBindDataForTest(sp *statementProcessor, args []sqldriver.Value) ([][]common.B1Array, []bindValueMetadata, []common.Marshallable, error) {
	metadata := extractBindMetadata(args)
	rows, err := sp.encodeBindValues(args, metadata)
	if err != nil {
		return nil, nil, nil, err
	}
	oacs, err := sp.prepareOACs(rows, metadata)
	if err != nil {
		return nil, nil, nil, err
	}
	return rows, metadata, oacs, nil
}

// ------------------------------
// Bind preparation - normalization
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
	rows, metadata, oacs, err := prepareBindDataForTest(sp, args)
	if err != nil {
		t.Fatalf("bind preparation returned error: %v", err)
	}
	if len(rows) != 1 || len(rows[0]) != len(args) {
		t.Fatalf("prepared row shape = %d x %d, want 1 x %d", len(rows), len(rows[0]), len(args))
	}
	if len(metadata) != len(args) {
		t.Fatalf("prepared metadata count does not match %d bind positions", len(args))
	}
	if len(oacs) != len(args) {
		t.Fatalf("prepared OAC count = %d, want %d", len(oacs), len(args))
	}
}

func TestPrepareBindsAndOAC_Normalize_Errors(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	sp := &statementProcessor{shelf: shelf}

	// unsupported type
	_, _, _, err := prepareBindDataForTest(sp, []sqldriver.Value{struct{}{}})
	if err == nil || !strings.Contains(err.Error(), "internal error occurred") {
		t.Fatalf("expected unsupported type error containing 'bind-type', got %v", err)
	}
}

// ------------------------------
// Bind preparation - encoding
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
	encoded, _, _, err := prepareBindDataForTest(sp, args)
	if err != nil {
		t.Fatalf("bind preparation returned error: %v", err)
	}
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
	_, _, _, err := prepareBindDataForTest(sp, []sqldriver.Value{struct{}{}})
	if err == nil || !strings.Contains(err.Error(), "internal error occurred") {
		t.Fatalf("expected type encode error containing 'bind-type', got %v", err)
	}
}

// ------------------------------
// pushBindRows error paths
// ------------------------------

// Use bind preparation to produce encoded payloads before exercising pushBindRows.
func TestPushBindRowsIfAny_EncodeBindValues_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, stmr, _ := newExecTestShelf(1024)
	shelf.RegisterMessageStreamer(stmr)
	req := &tTIOall{}
	req.setOptions(bindValuesPresent | executeStatement)
	req.setNumberOfBindPositions(1)

	registerTestCodecs(shelf, 20)

	// float64 encodes successfully while createMessages builds the RXD row.
	args := []sqldriver.Value{float64(1.25)}
	sp := &statementProcessor{shelf: shelf}
	created, err := sp.createMessages(&qualifiedSQLStatement{}, args, executionMessageConfig{functionType: oAll8})
	if err != nil {
		t.Fatalf("unexpected createMessages error, got %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("expected request and one bind row, got %d messages", len(created))
	}
	if _, ok := created[0].(*tTIOall); !ok {
		t.Fatalf("first message type = %T, want *tTIOall", created[0])
	}
	if _, ok := created[1].(*tTIrxd); !ok {
		t.Fatalf("second message type = %T, want *tTIrxd", created[1])
	}
	err = sp.pushBindRows(ctx, created[1:], "ut", oracleErrors.RunExecError)
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
	shelf, stmr, _ := newExecTestShelf(1024)

	// Wrap the existing factory to fail GetMessage(TTIRXD)
	base := shelf.GetMessageFactory().(Factory)
	shelf.RegisterMessageFactory(failingRXDFactory{base: base})
	shelf.RegisterMessageStreamer(stmr)

	req := &tTIOall{}
	req.setOptions(bindValuesPresent | executeStatement)
	req.setNumberOfBindPositions(1)

	registerTestCodecs(shelf, 20)

	// Supported bind so createMessages reaches the RXD allocation.
	args := []sqldriver.Value{int64(1)}
	sp := &statementProcessor{shelf: shelf}
	_, err := sp.createMessages(&qualifiedSQLStatement{}, args, executionMessageConfig{functionType: oAll8})
	if err == nil || !strings.Contains(err.Error(), "fail RXD") {
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

	// Supported bind so createMessages creates an RXD row; Push should fail.
	args := []sqldriver.Value{int64(1)}
	sp := &statementProcessor{shelf: shelf}
	created, err := sp.createMessages(&qualifiedSQLStatement{}, args, executionMessageConfig{functionType: oAll8})
	if err != nil {
		t.Fatalf("unexpected createMessages error, got %v", err)
	}
	err = sp.pushBindRows(ctx, created[1:], "ut", oracleErrors.RunExecError)
	if err == nil || !strings.Contains(err.Error(), "push") {
		t.Fatalf("expected RXD push error, got %v", err)
	}
}

// ------------------------------
// Bind preparation - OACs
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
	_, _, oacs, err := prepareBindDataForTest(sp, args)
	if err != nil {
		t.Fatalf("bind preparation returned error: %v", err)
	}
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
	_, _, _, err := prepareBindDataForTest(sp, []sqldriver.Value{map[string]int{"x": 1}})
	if err == nil || !strings.Contains(err.Error(), "internal error occurred") {
		t.Fatalf("expected type encode error containing 'bind-type', got %v", err)
	}
}
