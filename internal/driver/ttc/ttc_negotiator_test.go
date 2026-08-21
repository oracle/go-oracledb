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
	"encoding/binary"
	"errors"
	"regexp"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

var WriteCount int = 0

type failStep string

const (
	stepProtocol failStep = "protocol"
	stepDatatype failStep = "datatype"
)

type failWhere string

const (
	whereGet   failWhere = "get"
	wherePush  failWhere = "push"
	whereFlush failWhere = "flush"
	wherePull  failWhere = "pull"
)

type versatileFactory struct {
	failCode  common.MessageType
	failWhere failWhere
	failErr   error
}

func (f *versatileFactory) GetMessage(msgType common.MessageType) (common.Message[common.MessageType], error) {
	if msgType == f.failCode && f.failWhere == whereGet {
		return nil, f.failErr
	}
	if msgType == TTIPRO && f.failWhere != whereGet {
		return &tTIpro{}, nil
	}
	if msgType == TTIDTY && f.failWhere != whereGet {
		return &tTIdty{}, nil
	}
	return nil, errors.New("Requested message type is not available")
}
func (f *versatileFactory) GetMessageForFunction(msgType common.MessageType, funcType common.FunctionType) (common.Message[common.MessageType], error) {
	return nil, errors.New("unimplemented")
}

type versatileStreamer struct {
	failCode  common.MessageType
	failWhere failWhere
	failErr   error
}

func (s *versatileStreamer) Push(ctx context.Context, msg common.Message[common.MessageType]) error {
	if msg.GetMsgCode() == s.failCode && s.failWhere == wherePush {
		return s.failErr
	}
	return nil
}
func (s *versatileStreamer) Flush(ctx context.Context) error {
	if s.failWhere == whereFlush {
		return s.failErr
	}
	return nil
}
func (s *versatileStreamer) Pull(ctx context.Context, msgTypes ...common.MessageType) (common.Message[common.MessageType], error) {
	if len(msgTypes) == 0 {
		return nil, errors.New("no message type provided")
	}
	msgType := msgTypes[0]
	if msgType == s.failCode && s.failWhere == wherePull {
		return nil, s.failErr
	}
	// For TTIDTY/wherePull, never return a stub, to avoid panic on type assertion in code under test.
	if msgType == TTIDTY && s.failWhere == wherePull {
		return nil, s.failErr
	}
	if msgType == TTIPRO {
		return &tTIpro{}, nil
	}
	if msgType == TTIDTY {
		return &tTIdty{}, nil
	}
	return nil, errors.New("unimplemented")
}
func (s *versatileStreamer) Drain(ctx context.Context, direction common.StreamDirection) (int, int) {
	return -1, -1
}

// RegisterPreUnmarshallCallback is a no-op for this test mock.
func (s *versatileStreamer) RegisterPreUnmarshallCallback(msgType common.MessageType, cb StreamerPreUnmarshallCallback) {
}

// RegisterPostUnmarshallCallback is a no-op for this test mock.
func (s *versatileStreamer) RegisterPostUnmarshallCallback(msgType common.MessageType, cb StreamerPostUnmarshallCallback) {
}

// UnRegisterPreUnmarshallCallback is a no-op for this test mock.
func (s *versatileStreamer) UnRegisterPreUnmarshallCallback(msgType common.MessageType) {}

// UnRegisterPostUnmarshallCallback is a no-op for this test mock.
func (s *versatileStreamer) UnRegisterPostUnmarshallCallback(msgType common.MessageType) {}

func assertNegotiationError(t *testing.T, err error, expectedErr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error but got none")
	}
	match, matchErr := regexp.MatchString(expectedErr, err.Error())
	if matchErr != nil {
		t.Fatalf("failed to compile error pattern: %v", matchErr)
	}
	if !match {
		t.Errorf("expected error to match %q, got %q", expectedErr, err.Error())
	}
}

func TestConnectionNegotiator_Negotiate_Fail(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		step        failStep
		failCode    common.MessageType
		failWhere   failWhere
		expectedErr string
	}{
		{
			name:        "Invalid_Code_Protocol",
			step:        stepProtocol,
			failCode:    0xFF, // Nonexistent type
			failWhere:   whereGet,
			expectedErr: "Requested message type is not available",
		},
		{
			name:        "Invalid_Code_Datatype",
			step:        stepDatatype,
			failCode:    0xFE, // Nonexistent type
			failWhere:   whereGet,
			expectedErr: "Requested message type is not available",
		},
		{
			name:        "TTIPRO_Unavailable",
			step:        stepProtocol,
			failCode:    TTIPRO,
			failWhere:   whereGet,
			expectedErr: ".*An error occurred while negotiating the connection",
		},
		{
			name:        "TTIDTY_Unavailable",
			step:        stepDatatype,
			failCode:    TTIDTY,
			failWhere:   whereGet,
			expectedErr: ".*An error occurred while negotiating the connection",
		},
		{
			name:        "TTIPRO_Push",
			step:        stepProtocol,
			failCode:    TTIPRO,
			failWhere:   wherePush,
			expectedErr: "Push TTIPRO failed",
		},
		{
			name:        "TTIPRO_Flush",
			step:        stepProtocol,
			failCode:    TTIPRO,
			failWhere:   whereFlush,
			expectedErr: "Flush TTIPRO failed",
		},
		{
			name:        "TTIPRO_Pull",
			step:        stepProtocol,
			failCode:    TTIPRO,
			failWhere:   wherePull,
			expectedErr: "Pull TTIPRO failed",
		},
		{
			name:        "TTIDTY_Push",
			step:        stepDatatype,
			failCode:    TTIDTY,
			failWhere:   wherePush,
			expectedErr: "Push TTIDTY failed",
		},
		{
			name:        "TTIDTY_Flush",
			step:        stepDatatype,
			failCode:    TTIDTY,
			failWhere:   whereFlush,
			expectedErr: "Flush TTIDTY failed",
		},
		{
			name:        "TTIDTY_Pull",
			step:        stepDatatype,
			failCode:    TTIDTY,
			failWhere:   wherePull,
			expectedErr: "Pull TTIDTY failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			factory := &versatileFactory{
				failCode:  tc.failCode,
				failWhere: tc.failWhere,
				failErr:   errors.New(tc.expectedErr),
			}
			streamer := &versatileStreamer{
				failCode:  tc.failCode,
				failWhere: tc.failWhere,
				failErr:   errors.New(tc.expectedErr),
			}
			negotiator := newConnectionNegotiator()
			var err error
			switch tc.step {
			case stepProtocol:
				_, err = negotiator._negotiateProtocol(ctx, streamer, factory)
			case stepDatatype:
				var pro *tTIpro
				// Use non-nil ClientCaps except in GetMessage(TTIDTY) fail case, where GetMessage error is expected.
				if tc.failWhere == whereGet {
					pro = nil
				} else {
					pro = &tTIpro{clientCaps: &capability{}}
					pro.svrCharSet = al32Utf8CharSet
				}
				_, err = negotiator._negotiateDatatype(ctx, streamer, factory, pro)
			}
			assertNegotiationError(t, err, tc.expectedErr)
		})
	}
}

func successOnWriteDefaults(protocolVer byte, includeFDO, addArrays bool) func(tb *TestDataBuffer) {
	return func(tb *TestDataBuffer) {
		WriteCount++
		if WriteCount == 1 {
			def := DefaultTTIproSuccessPayload(protocolVer, includeFDO, addArrays)
			tb.ResetBuf()
			tb.WriteBuf(def)
		} else if WriteCount == 2 {
			WriteCount = 0
		}
	}
}

func DefaultTTIproSuccessPayload(protoVer byte, includeFDO bool, addArrays bool) []byte {
	var buf []byte
	// Append TTC message type, protocol version, reserved byte.
	buf = append(buf, byte(TTIPRO), protoVer, 0)
	// Append server port description string ("test"), followed by null-terminator.
	desc := []byte("test")
	buf = append(buf, desc...)
	buf = append(buf, 0)
	// Append server character set (UB2) and server flags (UB1).
	buf = binary.LittleEndian.AppendUint16(buf, 871)
	buf = append(buf, 1)
	// Append svrCharSetElem (UB2): number of charset elements.
	buf = binary.LittleEndian.AppendUint16(buf, 0)
	if includeFDO {
		// Append FDO length (UB2): feature data object.
		buf = binary.BigEndian.AppendUint16(buf, 11)
		// Append FDO bytes (dummy/test values).
		fdo := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 7}
		buf = append(buf, fdo...)
		if addArrays {
			// Append compile capabilities array length and data.
			buf = append(buf, 8)
			buf = append(buf, []byte{1, 2, 3, 4, 5, 6, 7, 8}...)
			// Append run time capabilities array length and data.
			buf = append(buf, 3)
			buf = append(buf, []byte{9, 10, 11}...)
		}
	}
	return buf
}

func TestConnectionNegotiator_Negotiate_Success(t *testing.T) {
	t.Parallel()
	type successTest struct {
		name   string
		buffer func() common.DataBuffer
	}
	cases := []successTest{
		{
			name: "Version6",
			buffer: func() common.DataBuffer {
				buf := NewTestDataBuffer()
				buf.OnWriteDefaults = successOnWriteDefaults(6, true, true)
				return buf
			},
		},
		{
			name: "Version5",
			buffer: func() common.DataBuffer {
				buf := NewTestDataBuffer()
				buf.OnWriteDefaults = successOnWriteDefaults(5, true, false)
				return buf
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			negotiator := newConnectionNegotiator()
			ctx := context.Background()
			data := tc.buffer()
			negotiator.SetDataBuffer(data)
			sessCtx, shelf, err := negotiator.Negotiate(ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sessCtx == nil {
				t.Error("SessionContext is nil")
			}
			if shelf == nil {
				t.Error("Shelf is nil")
			}
		})
	}
}
