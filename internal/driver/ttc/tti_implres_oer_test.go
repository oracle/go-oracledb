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
** is contributed to or provided by such licensors),
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
	"errors"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// implresOERMarshaller injects failures after the shared implicit-result OER
// body. It allows the tests to exercise the versioned suffix independently of
// the base decoder.
type implresOERMarshaller struct {
	driverCommon.Marshaller
	afterExtendedRowCount bool
	ub4AfterBody          int
	failUB4AfterBody      int
}

func (m *implresOERMarshaller) UnmarshalUB8(ctx context.Context) (driverCommon.UB8, error) {
	value, err := m.Marshaller.UnmarshalUB8(ctx)
	if err == nil {
		m.afterExtendedRowCount = true
	}
	return value, err
}

func (m *implresOERMarshaller) UnmarshalUB4(ctx context.Context) (driverCommon.UB4, error) {
	if m.afterExtendedRowCount {
		m.ub4AfterBody++
		if m.ub4AfterBody == m.failUB4AfterBody {
			return 0, errors.New("injected implicit-result OER UB4 error")
		}
	}
	return m.Marshaller.UnmarshalUB4(ctx)
}

// newImplicitResultOERTestMarshaller returns a marshaller positioned at a
// complete TTC 14 implicit-result OER payload.
func newImplicitResultOERTestMarshaller(t *testing.T, errorCode driverCommon.UB2, errorText []byte) driverCommon.Marshaller {
	t.Helper()
	ctx := context.Background()
	_, mar := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, Universal, Universal, 1024)
	if err := marshalOER(ctx, mar, errorCode, errorText); err != nil {
		t.Fatalf("marshal implicit-result OER: %v", err)
	}
	return mar
}

// TestTTIimplresOER_Constructors validates that both implicit-result OER
// constructors select the correct decoder and message code.
func TestTTIimplresOER_Constructors(t *testing.T) {
	t.Parallel()

	base, ok := newTTIimplresOer().(*tTIimplresOer)
	if !ok || base.tTIoer == nil {
		t.Fatalf("newTTIimplresOer() = %T, want initialized *tTIimplresOer", base)
	}
	if got := base.GetMsgCode(); got != TTIIMPLOER {
		t.Fatalf("base message code = %v, want %v", got, TTIIMPLOER)
	}

	versioned, ok := newTTIimplresOer14().(*tTIimplresOer14)
	if !ok || versioned.tTIimplresOer == nil || versioned.tTIoer == nil {
		t.Fatalf("newTTIimplresOer14() = %#v, want initialized versioned decoder", versioned)
	}
	if got := versioned.GetMsgCode(); got != TTIIMPLOER {
		t.Fatalf("versioned message code = %v, want %v", got, TTIIMPLOER)
	}
}

// TestTTIimplresOER_BaseDecode covers successful base decoding, base body
// failures, and conditional CLR error-message consumption.
func TestTTIimplresOER_BaseDecode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success without error message", func(t *testing.T) {
		oer := newTTIimplresOer().(*tTIimplresOer)
		if err := oer.UnMarshalFrom(ctx, newImplicitResultOERTestMarshaller(t, 0, nil)); err != nil {
			t.Fatalf("UnMarshalFrom() error = %v", err)
		}
		if oer.retCode != 0 || oer.oerrcd2 != 0 || len(oer.errorMsg) != 0 {
			t.Fatalf("decoded success OER = %#v, want no Oracle error", oer)
		}
	})

	t.Run("error message is consumed only for error OER", func(t *testing.T) {
		_, mar := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, Universal, Universal, 128)
		if err := mar.MarshalCLR(ctx, []byte("ORA-00942"), 0, len("ORA-00942")); err != nil {
			t.Fatalf("marshal error text: %v", err)
		}
		oer := newTTIimplresOer().(*tTIimplresOer)
		oer.retCode = 942
		if err := oer.unmarshalErrorMessage(ctx, mar); err != nil {
			t.Fatalf("unmarshal error message: %v", err)
		}
		if got := string(oer.errorMsg); got != "ORA-00942" {
			t.Fatalf("error message = %q, want ORA-00942", got)
		}

		oer = newTTIimplresOer().(*tTIimplresOer)
		if err := oer.unmarshalErrorMessage(ctx, newImplicitResultOERTestMarshaller(t, 0, nil)); err != nil {
			t.Fatalf("success OER consumed an error message: %v", err)
		}
	})

	t.Run("truncated attributes", func(t *testing.T) {
		oer := newTTIimplresOer().(*tTIimplresOer)
		_, mar := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, Universal, Universal, 8)
		if err := oer.UnMarshalFrom(ctx, mar); err == nil {
			t.Fatal("truncated implicit-result OER returned nil error")
		}
	})
}

// TestTTIimplresOER14Decode validates the TTC 14 suffix and its independent
// failure paths after a successfully decoded shared OER body.
func TestTTIimplresOER14Decode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("truncated shared attributes", func(t *testing.T) {
		oer := newTTIimplresOer14().(*tTIimplresOer14)
		_, mar := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, Universal, Universal, 8)
		if err := oer.UnMarshalFrom(ctx, mar); err == nil {
			t.Fatal("truncated versioned implicit-result OER returned nil error")
		}
	})

	t.Run("success and error message", func(t *testing.T) {
		oer := newTTIimplresOer14().(*tTIimplresOer14)
		if err := oer.UnMarshalFrom(ctx, newImplicitResultOERTestMarshaller(t, 942, []byte("ORA-00942"))); err != nil {
			t.Fatalf("UnMarshalFrom() error = %v", err)
		}
		if oer.sqlCommandType != 0 || oer.checksum != 0 {
			t.Fatalf("versioned suffix = command %d checksum %d, want zero", oer.sqlCommandType, oer.checksum)
		}
		if got := string(oer.errorMsg); got != "ORA-00942" {
			t.Fatalf("error message = %q, want ORA-00942", got)
		}
	})

	for _, test := range []struct {
		name      string
		failUB4At int
	}{
		{name: "SQL command type", failUB4At: 1},
		{name: "checksum", failUB4At: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			mar := &implresOERMarshaller{
				Marshaller:       newImplicitResultOERTestMarshaller(t, 0, nil),
				failUB4AfterBody: test.failUB4At,
			}
			oer := newTTIimplresOer14().(*tTIimplresOer14)
			if err := oer.UnMarshalFrom(ctx, mar); err == nil {
				t.Fatalf("%s failure returned nil error", test.name)
			}
		})
	}
}
