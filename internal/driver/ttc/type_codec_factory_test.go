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
	"database/sql"
	"database/sql/driver"
	"reflect"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

func dummyEncoderA(driver.Value) (common.B1Array, error) { return nil, nil }

func dummyEncoderB(driver.Value) (common.B1Array, error) { return common.B1Array{1, 2, 3}, nil }

var dummyDecoderA = newTypeDecoder(func(columnContext, common.B1Array) (driver.Value, error) { return "A", nil }, nil)

var dummyDecoderB = newTypeDecoder(func(columnContext, common.B1Array) (driver.Value, error) { return "B", nil }, nil)

var dummyBindOacA = bindOacType{
	bindOacFunc: func(common.UB4) common.Marshallable {
		return newTTIoac(DtyNum, 8)
	},
	maxLength: 8,
}

var dummyBindOacB = bindOacType{
	bindOacFunc: func(common.UB4) common.Marshallable {
		return newTTIoac(DtyVCS, 16)
	},
	maxLength: 16,
}

func dummyDefineOacA(columnContext, common.UB4) common.Marshallable {
	return newTTIoac(DtyNum, define_maxlength_scalar)
}

func dummyDefineOacB(columnContext, common.UB4) common.Marshallable {
	return newTTIoac(DtyVCS, define_maxlength_varchar)
}

// TestCodecFactory_getEncoder exercises encoder selection and error paths for CodecFactoryImpl.
// It validates that the highest compatible version is returned for a given protocol and Go type,
// and that the factory emits structured Oracle errors when the Go type is nil or unregistered.
func TestCodecFactory_getEncoder(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name        string
		protocol    int8
		value       driver.Value
		setup       func(reg *codecRegistry[reflect.Type, encoderFunc])
		wantValue   common.B1Array
		expectError bool
		errCode     oracleErrors.ErrorCode
	}

	cases := []testCase{
		{
			name:     "selects highest version within protocol",
			protocol: 3,
			value:    int64(0),
			setup: func(reg *codecRegistry[reflect.Type, encoderFunc]) {
				reg.Register(reflect.TypeOf(int64(0)), 1, dummyEncoderA)
				reg.Register(reflect.TypeOf(int64(0)), 3, dummyEncoderB)
			},
			wantValue: common.B1Array{1, 2, 3},
		},
		{
			name:     "skips candidates greater than protocol",
			protocol: 1,
			value:    int64(0),
			setup: func(reg *codecRegistry[reflect.Type, encoderFunc]) {
				reg.Register(reflect.TypeOf(int64(0)), 2, dummyEncoderB)
			},
			expectError: true,
			errCode:     oracleErrors.InternalError,
		},
		{
			name:        "no registered encoder",
			protocol:    1,
			value:       "",
			setup:       func(reg *codecRegistry[reflect.Type, encoderFunc]) {},
			expectError: true,
			errCode:     oracleErrors.InternalError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encReg := newCodecRegistry[reflect.Type, encoderFunc]()
			if tc.setup != nil {
				tc.setup(encReg)
			}
			factory := &CodecFactoryImpl{ttcVersion: tc.protocol, encoders: encReg}

			encoder, err := factory.getEncoder(normalizeBindValue(tc.value))
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				sqlErr, ok := err.(oracleErrors.SQLError)
				if !ok {
					t.Fatalf("expected SQLError, got %T", err)
				}
				if sqlErr.ErrorCode() != string(tc.errCode) {
					t.Fatalf("expected error code %s, got %s", tc.errCode, sqlErr.ErrorCode())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, err := encoder(normalizeBindValue(tc.value).value)
			if err != nil {
				t.Fatalf("unexpected encode error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.wantValue) {
				t.Fatalf("returned encoder mismatch: got %v want %v", got, tc.wantValue)
			}
		})
	}
}

// TestCodecFactory_getDecoder mirrors the encoder suite for decoder selection, covering
// fallback behaviour for lower negotiated protocol versions, and error surfacing when no
// candidate or type registration exists.
func TestCodecFactory_getDecoder(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name        string
		protocol    int8
		dbType      int16
		setup       func(reg *codecRegistry[int16, *typeDecoder])
		wantValue   driver.Value
		expectError bool
		errCode     oracleErrors.ErrorCode
	}

	cases := []testCase{
		{
			name:     "picks highest version within protocol",
			protocol: 5,
			dbType:   DtyNum,
			setup: func(reg *codecRegistry[int16, *typeDecoder]) {
				reg.Register(DtyNum, -1, dummyDecoderA)
				reg.Register(DtyNum, 2, dummyDecoderB)
			},
			wantValue: "B",
		},
		{
			name:     "falls back to lower version when protocol is low",
			protocol: 1,
			dbType:   DtyNum,
			setup: func(reg *codecRegistry[int16, *typeDecoder]) {
				reg.Register(DtyNum, -1, dummyDecoderA)
				reg.Register(DtyNum, 2, dummyDecoderB)
			},
			wantValue: "A",
		},
		{
			name:     "no candidate eligible for negotiated protocol",
			protocol: 1,
			dbType:   DtyNum,
			setup: func(reg *codecRegistry[int16, *typeDecoder]) {
				reg.Register(DtyNum, 3, dummyDecoderB)
			},
			expectError: true,
			errCode:     oracleErrors.InternalError,
		},
		{
			name:     "decoder type not registered",
			protocol: 1,
			dbType:   DtyChr,
			setup: func(reg *codecRegistry[int16, *typeDecoder]) {
				reg.Register(DtyNum, -1, dummyDecoderA)
			},
			expectError: true,
			errCode:     oracleErrors.InternalError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decReg := newCodecRegistry[int16, *typeDecoder]()
			if tc.setup != nil {
				tc.setup(decReg)
			}
			factory := &CodecFactoryImpl{ttcVersion: tc.protocol, decoders: decReg}

			decoder, err := factory.getDecoder(tc.dbType)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				sqlErr, ok := err.(oracleErrors.SQLError)
				if !ok {
					t.Fatalf("expected SQLError, got %T", err)
				}
				if sqlErr.ErrorCode() != string(tc.errCode) {
					t.Fatalf("expected error code %s, got %s", tc.errCode, sqlErr.ErrorCode())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, err := decoder.decodeToType(columnContext{}, nil)
			if err != nil {
				t.Fatalf("unexpected decode error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.wantValue) {
				t.Fatalf("unexpected decoder result selected: got %v want %v", got, tc.wantValue)
			}
		})
	}
}

// TestCodecFactory_RegisterEncoderGeneric verifies the generic RegisterEncoder helper resolves
// the Go type of T and inserts the implementor into the global encoder registry with the
// provided version metadata.
func TestCodecFactory_getBindOac(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name            string
		protocol        int8
		bindValue       driver.Value
		maxLength       common.UB4
		setup           func(reg *codecRegistry[reflect.Type, bindOacType])
		wantDataType    common.UB1
		expectError     bool
		expectedErrCode oracleErrors.ErrorCode
	}

	cases := []testCase{
		{
			name:      "selects highest compatible bind oac constructor",
			protocol:  3,
			bindValue: int64(0),
			maxLength: 32,
			setup: func(reg *codecRegistry[reflect.Type, bindOacType]) {
				reg.Register(reflect.TypeOf(int64(0)), 1, dummyBindOacA)
				reg.Register(reflect.TypeOf(int64(0)), 3, dummyBindOacB)
			},
			wantDataType: common.UB1(DtyChr),
		},
		{
			name:            "missing bind oac candidate",
			protocol:        1,
			bindValue:       "",
			maxLength:       10,
			setup:           func(reg *codecRegistry[reflect.Type, bindOacType]) {},
			expectError:     true,
			expectedErrCode: oracleErrors.InternalError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bindReg := newCodecRegistry[reflect.Type, bindOacType]()
			if tc.setup != nil {
				tc.setup(bindReg)
			}
			factory := &CodecFactoryImpl{ttcVersion: tc.protocol, bindOacs: bindReg}

			oac, err := factory.getBindOac(normalizeBindValue(tc.bindValue), tc.maxLength)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				sqlErr, ok := err.(oracleErrors.SQLError)
				if !ok {
					t.Fatalf("expected SQLError, got %T", err)
				}
				if sqlErr.ErrorCode() != string(tc.expectedErrCode) {
					t.Fatalf("expected error code %s, got %s", tc.expectedErrCode, sqlErr.ErrorCode())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := oac.(*tTIoac)
			if got.dataType != tc.wantDataType {
				t.Fatalf("expected dataType %d, got %d", tc.wantDataType, got.dataType)
			}
		})
	}
}

func TestNormalizeBindValue_SQLNullTypes(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.January, 15, 10, 30, 0, 0, time.UTC)

	cases := []struct {
		name      string
		input     driver.Value
		wantType  reflect.Type
		wantValue driver.Value
		wantIsNil bool
	}{
		{
			name:      "null string valid",
			input:     sql.NullString{String: "abc", Valid: true},
			wantType:  reflect.TypeOf(""),
			wantValue: "abc",
		},
		{
			name:      "null string invalid",
			input:     sql.NullString{String: "abc", Valid: false},
			wantIsNil: true,
		},
		{
			name:      "null int64 valid",
			input:     sql.NullInt64{Int64: 42, Valid: true},
			wantType:  reflect.TypeOf(int64(0)),
			wantValue: int64(42),
		},
		{
			name:      "null int64 invalid",
			input:     sql.NullInt64{Int64: 42, Valid: false},
			wantIsNil: true,
		},
		{
			name:      "null int32 valid",
			input:     sql.NullInt32{Int32: 32, Valid: true},
			wantType:  reflect.TypeOf(int32(0)),
			wantValue: int32(32),
		},
		{
			name:      "null int32 invalid",
			input:     sql.NullInt32{Int32: 32, Valid: false},
			wantIsNil: true,
		},
		{
			name:      "null int16 valid",
			input:     sql.NullInt16{Int16: 16, Valid: true},
			wantType:  reflect.TypeOf(int16(0)),
			wantValue: int16(16),
		},
		{
			name:      "null int16 invalid",
			input:     sql.NullInt16{Int16: 16, Valid: false},
			wantIsNil: true,
		},
		{
			name:      "null byte valid",
			input:     sql.NullByte{Byte: 7, Valid: true},
			wantType:  reflect.TypeOf(byte(0)),
			wantValue: byte(7),
		},
		{
			name:      "null byte invalid",
			input:     sql.NullByte{Byte: 7, Valid: false},
			wantIsNil: true,
		},
		{
			name:      "null float64 valid",
			input:     sql.NullFloat64{Float64: 12.5, Valid: true},
			wantType:  reflect.TypeOf(float64(0)),
			wantValue: 12.5,
		},
		{
			name:      "null float64 invalid",
			input:     sql.NullFloat64{Float64: 12.5, Valid: false},
			wantIsNil: true,
		},
		{
			name:      "null bool valid",
			input:     sql.NullBool{Bool: true, Valid: true},
			wantType:  reflect.TypeOf(true),
			wantValue: true,
		},
		{
			name:      "null bool invalid",
			input:     sql.NullBool{Bool: true, Valid: false},
			wantIsNil: true,
		},
		{
			name:      "null time valid",
			input:     sql.NullTime{Time: now, Valid: true},
			wantType:  reflect.TypeOf(time.Time{}),
			wantValue: now,
		},
		{
			name:      "null time invalid",
			input:     sql.NullTime{Time: now, Valid: false},
			wantIsNil: true,
		},
		{
			name:      "sql out unwraps null string",
			input:     sql.Out{Dest: &sql.NullString{String: "out", Valid: true}, In: true},
			wantType:  reflect.TypeOf(""),
			wantValue: "out",
		},
		{
			name:      "sql out unwraps invalid null string to nil",
			input:     sql.Out{Dest: &sql.NullString{String: "out", Valid: false}, In: true},
			wantIsNil: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeBindValue(tc.input)
			if tc.wantIsNil {
				if got.value != nil {
					t.Fatalf("expected nil normalized value, got %v", got.value)
				}
				return
			}
			if got.goType != tc.wantType {
				t.Fatalf("expected normalized type %v, got %v", tc.wantType, got.goType)
			}
			if !reflect.DeepEqual(got.value, tc.wantValue) {
				t.Fatalf("expected normalized value %v, got %v", tc.wantValue, got.value)
			}
		})
	}
}

func TestCodecFactory_getDefineOac(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name         string
		protocol     int8
		dbType       DtyType
		columnCtx    columnContext
		setup        func(reg *codecRegistry[DtyType, defineOacFunc])
		wantMaxLen   common.UB4
		wantDataType common.UB1
	}

	cases := []testCase{
		{
			name:      "selects highest compatible define oac constructor",
			protocol:  3,
			dbType:    DtyVCS,
			columnCtx: columnContext{DataType: DtyVCS},
			setup: func(reg *codecRegistry[DtyType, defineOacFunc]) {
				reg.Register(DtyVCS, 1, dummyDefineOacA)
				reg.Register(DtyVCS, 3, dummyDefineOacB)
			},
			wantMaxLen:   define_maxlength_varchar,
			wantDataType: common.UB1(DtyChr),
		},
		{
			name:         "falls back to scalar define oac when missing",
			protocol:     1,
			dbType:       DtyNum,
			columnCtx:    columnContext{DataType: DtyNum},
			setup:        func(reg *codecRegistry[DtyType, defineOacFunc]) {},
			wantMaxLen:   define_maxlength_scalar,
			wantDataType: common.UB1(DtyNum),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defineReg := newCodecRegistry[DtyType, defineOacFunc]()
			if tc.setup != nil {
				tc.setup(defineReg)
			}
			factory := &CodecFactoryImpl{
				ttcVersion: tc.protocol,
				defineOacs: defineReg,
			}

			got := factory.getDefineOac(tc.dbType, tc.columnCtx, nil).(*tTIoac)
			if got.maxLength != tc.wantMaxLen {
				t.Fatalf("expected maxLength %d, got %d", tc.wantMaxLen, got.maxLength)
			}
			if got.dataType != tc.wantDataType {
				t.Fatalf("expected dataType %d, got %d", tc.wantDataType, got.dataType)
			}
		})
	}
}

func TestCodecFactory_RegisterEncoderGeneric(t *testing.T) {
	t.Parallel()
	orig := EncoderRegistry
	EncoderRegistry = newCodecRegistry[reflect.Type, encoderFunc]()
	defer func() {
		EncoderRegistry = orig
	}()

	encodeString := func(driver.Value) (common.B1Array, error) {
		return common.B1Array{0x1}, nil
	}

	if err := EncoderRegistry.Register(reflect.TypeOf(""), 2, encodeString); err != nil {
		t.Fatalf("RegisterEncoder returned unexpected error: %v", err)
	}

	candidates := EncoderRegistry.getCandidates(reflect.TypeOf(""))
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].fromTTCProtocolVersion != 2 {
		t.Fatalf("expected from protocol 2, got %d", candidates[0].fromTTCProtocolVersion)
	}
	if reflect.ValueOf(candidates[0].makeFunc).Pointer() != reflect.ValueOf(encodeString).Pointer() {
		t.Fatalf("registered encoder function mismatch")
	}
}
