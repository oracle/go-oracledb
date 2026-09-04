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
	"io"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

func TestImplicitResultRowsNextResultSet(t *testing.T) {
	first := newTTCRows([]columnContext{{Name: []byte("FIRST"), DataType: DtyChr}})
	first.rowData = [][]driverCommon.B1Array{{[]byte("one")}}
	first.lobColContext = [][]*lobColumnContext{{nil}}
	first.numOfRows = 1

	second := newTTCRows([]columnContext{{Name: []byte("SECOND"), DataType: DtyChr}})
	second.rowData = [][]driverCommon.B1Array{{[]byte("two")}}
	second.lobColContext = [][]*lobColumnContext{{nil}}
	second.numOfRows = 1

	rows := newImplicitResultRows([]*ttcRows{first, second})
	if got := rows.Columns(); len(got) != 1 || got[0] != "FIRST" {
		t.Fatalf("first result columns = %v, want [FIRST]", got)
	}
	if !rows.HasNextResultSet() {
		t.Fatal("HasNextResultSet() = false, want true")
	}
	if err := rows.NextResultSet(); err != nil {
		t.Fatalf("NextResultSet() error = %v", err)
	}
	if got := rows.Columns(); len(got) != 1 || got[0] != "SECOND" {
		t.Fatalf("second result columns = %v, want [SECOND]", got)
	}
	if rows.HasNextResultSet() {
		t.Fatal("HasNextResultSet() = true after final result")
	}
	if err := rows.NextResultSet(); err != io.EOF {
		t.Fatalf("final NextResultSet() error = %v, want io.EOF", err)
	}
}

func TestTTIimplres_ZeroResultSets(t *testing.T) {
	ctx := context.Background()
	_, mar := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, Universal, Universal, 1024)
	if err := mar.MarshalUB4(ctx, 0); err != nil {
		t.Fatalf("marshal result-set count: %v", err)
	}

	implres := &tTIimplres{
		newDCB: func() (*tTIdcb, error) {
			t.Fatal("newDCB called for an empty implicit-result message")
			return nil, nil
		},
		newRows: func([]columnContext, driverCommon.SB4) *ttcRows {
			t.Fatal("newRows called for an empty implicit-result message")
			return nil
		},
	}
	if err := implres.UnMarshalFrom(ctx, mar); err != nil {
		t.Fatalf("unmarshal empty implicit-result message: %v", err)
	}
	if len(implres.rows) != 0 {
		t.Fatalf("implicit result rows = %d, want 0", len(implres.rows))
	}
}

func TestTTIimplres_MultipleResultSets(t *testing.T) {
	ctx := context.Background()
	_, mar := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, Universal, Universal, 1024)
	if err := mar.MarshalUB4(ctx, 2); err != nil {
		t.Fatalf("marshal result-set count: %v", err)
	}
	for _, cursorID := range []driverCommon.UB4{41, 42} {
		if err := marshalEmptyImplicitResultDCB(ctx, mar, cursorID); err != nil {
			t.Fatalf("marshal implicit result DCB: %v", err)
		}
	}

	implres := &tTIimplres{
		newDCB: func() (*tTIdcb, error) { return &tTIdcb{newUDS: newTTIuds}, nil },
		newRows: func(columns []columnContext, cursorID driverCommon.SB4) *ttcRows {
			if len(columns) != 1 || string(columns[0].Name) != "C" {
				t.Fatalf("columns = %#v, want one column named C", columns)
			}
			rows := newTTCRows(columns)
			rows.cursorID = cursorID
			return rows
		},
	}
	if err := implres.UnMarshalFrom(ctx, mar); err != nil {
		t.Fatalf("unmarshal implicit results: %v", err)
	}
	if len(implres.rows) != 2 {
		t.Fatalf("implicit result rows = %d, want 2", len(implres.rows))
	}
	for i, want := range []driverCommon.SB4{41, 42} {
		if got := implres.rows[i].cursorID; got != want {
			t.Fatalf("cursor %d ID = %d, want %d", i, got, want)
		}
	}
}

func TestTTIimplres_RejectsUnconfiguredAndTruncatedMessages(t *testing.T) {
	ctx := context.Background()
	_, mar := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, Universal, Universal, 1024)
	if err := (&tTIimplres{}).UnMarshalFrom(ctx, mar); err == nil {
		t.Fatal("unconfigured implicit-result decoder returned nil error")
	}

	implres := &tTIimplres{
		newDCB:  func() (*tTIdcb, error) { return &tTIdcb{}, nil },
		newRows: func([]columnContext, driverCommon.SB4) *ttcRows { return &ttcRows{} },
	}
	if err := implres.UnMarshalFrom(ctx, mar); err == nil {
		t.Fatal("truncated implicit-result message returned nil error")
	}
}

func TestTTIimplres_PrefetchCompletion(t *testing.T) {
	ctx := context.Background()
	_, mar := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, Universal, Universal, 1024)
	if err := mar.MarshalUB4(ctx, 1); err != nil {
		t.Fatalf("marshal result-set count: %v", err)
	}
	if err := marshalEmptyImplicitResultDCB(ctx, mar, 41); err != nil {
		t.Fatalf("marshal implicit result DCB: %v", err)
	}
	if err := mar.MarshalUB1(ctx, driverCommon.UB1(TTIRXH)); err != nil {
		t.Fatalf("marshal implicit-result RXH message type: %v", err)
	}
	if err := marshalEmptyRXH(ctx, mar); err != nil {
		t.Fatalf("marshal implicit-result RXH: %v", err)
	}
	if err := mar.MarshalUB1(ctx, driverCommon.UB1(TTIRXD)); err != nil {
		t.Fatalf("marshal implicit-result RXD message type: %v", err)
	}
	if err := mar.MarshalCLR(ctx, []byte("X"), 0, 1); err != nil {
		t.Fatalf("marshal implicit-result RXD value: %v", err)
	}
	if err := mar.MarshalUB1(ctx, driverCommon.UB1(TTIRXD)); err != nil {
		t.Fatalf("marshal second implicit-result RXD message type: %v", err)
	}
	if err := mar.MarshalCLR(ctx, []byte("Y"), 0, 1); err != nil {
		t.Fatalf("marshal second implicit-result RXD value: %v", err)
	}
	if err := mar.MarshalUB1(ctx, driverCommon.UB1(TTIIMPLOER)); err != nil {
		t.Fatalf("marshal implicit-result OER message type: %v", err)
	}
	if err := marshalSuccessfulOER(ctx, mar); err != nil {
		t.Fatalf("marshal successful implicit-result OER: %v", err)
	}

	implres := &tTIimplres{
		newDCB: func() (*tTIdcb, error) { return &tTIdcb{newUDS: newTTIuds}, nil },
		newRows: func(columns []columnContext, cursorID driverCommon.SB4) *ttcRows {
			rows := newTTCRows(columns)
			rows.cursorID = cursorID
			return rows
		},
		newRefCursorRows: func(columns []columnContext, cursorID driverCommon.SB4) *ttcRows {
			return newTTCRows(columns)
		},
		prefetch: true,
	}
	if err := implres.UnMarshalFrom(ctx, mar); err != nil {
		t.Fatalf("unmarshal prefetched implicit result: %v", err)
	}
	if len(implres.rows) != 1 || implres.rows[0].fetch != nil || implres.rows[0].numOfRows != 2 || string(implres.rows[0].rowData[0][0]) != "X" || string(implres.rows[0].rowData[1][0]) != "Y" {
		t.Fatalf("prefetched rows = %#v, want one fully fetched result containing X and Y", implres.rows)
	}
}

func TestTTIimplres_PrefetchColumnPresenceVector(t *testing.T) {
	ctx := context.Background()
	_, mar := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, Universal, Universal, 1024)
	if err := mar.MarshalUB1(ctx, driverCommon.UB1(TTIBVC)); err != nil {
		t.Fatalf("marshal BVC message type: %v", err)
	}
	if err := mar.MarshalUB2(ctx, 0); err != nil {
		t.Fatalf("marshal BVC column count: %v", err)
	}
	if err := mar.MarshalB1Array(ctx, []byte{0}); err != nil {
		t.Fatalf("marshal BVC bit vector: %v", err)
	}
	if err := mar.MarshalUB1(ctx, driverCommon.UB1(TTIIMPLOER)); err != nil {
		t.Fatalf("marshal implicit-result OER message type: %v", err)
	}
	if err := marshalSuccessfulOER(ctx, mar); err != nil {
		t.Fatalf("marshal successful implicit-result OER: %v", err)
	}

	rows := newTTCRows([]columnContext{{Name: []byte("C"), DataType: DtyChr}})
	if err := (&tTIimplres{}).unmarshalPrefetch(ctx, mar, rows, rows.columnContexts); err != nil {
		t.Fatalf("unmarshal implicit-result BVC: %v", err)
	}
	if rows.numOfRows != 0 || rows.fetch != nil {
		t.Fatalf("rows after BVC prefetch = %#v", rows)
	}
}

func TestTTIimplres_ConfigurationAndUnexpectedPrefetchMessage(t *testing.T) {
	implres, ok := newTTIimplres().(*tTIimplres)
	if !ok || implres.GetMsgCode() != TTIIMPLRES {
		t.Fatalf("newTTIimplres() = %T with message code %v", implres, implres.GetMsgCode())
	}
	newDCB := func() (*tTIdcb, error) { return nil, nil }
	newRows := func([]columnContext, driverCommon.SB4) *ttcRows { return nil }
	implres.configure(newDCB, newRows, true)
	implres.setRefCursorRowsFactory(newRows)
	implres.setSessionCharacterSets(873, 2000)
	if implres.newDCB == nil || implres.newRows == nil || implres.newRefCursorRows == nil || !implres.prefetch || implres.sessCharSet != 873 || implres.sessNCharSet != 2000 {
		t.Fatal("implicit-result configuration was not retained")
	}

	ctx := context.Background()
	_, mar := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, Universal, Universal, 1024)
	if err := mar.MarshalUB1(ctx, 0); err != nil {
		t.Fatalf("marshal unsupported prefetch message: %v", err)
	}
	if err := implres.unmarshalPrefetch(ctx, mar, newTTCRows(nil), nil); err == nil {
		t.Fatal("unexpected prefetch message returned nil error")
	}
}

func TestTTIimplres_DecodeErrors(t *testing.T) {
	ctx := context.Background()
	newMarshaller := func() driverCommon.Marshaller {
		_, mar := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, Universal, Universal, 1024)
		return mar
	}
	configured := func() *tTIimplres {
		return &tTIimplres{
			newDCB:  func() (*tTIdcb, error) { return &tTIdcb{newUDS: newTTIuds}, nil },
			newRows: func([]columnContext, driverCommon.SB4) *ttcRows { return newTTCRows(nil) },
		}
	}

	t.Run("DCB factory", func(t *testing.T) {
		mar := newMarshaller()
		if err := mar.MarshalUB4(ctx, 1); err != nil {
			t.Fatal(err)
		}
		implres := configured()
		implres.newDCB = func() (*tTIdcb, error) { return nil, io.ErrUnexpectedEOF }
		if err := implres.UnMarshalFrom(ctx, mar); err == nil {
			t.Fatal("DCB factory failure returned nil error")
		}
	})

	t.Run("truncated DCB", func(t *testing.T) {
		mar := newMarshaller()
		if err := mar.MarshalUB4(ctx, 1); err != nil {
			t.Fatal(err)
		}
		if err := configured().UnMarshalFrom(ctx, mar); err == nil {
			t.Fatal("truncated DCB returned nil error")
		}
	})

	for _, code := range []driverCommon.MessageType{TTIRXH, TTIBVC, TTIRXD, TTIIMPLOER} {
		t.Run("truncated "+TTCMsgTypeName[code], func(t *testing.T) {
			mar := newMarshaller()
			if err := mar.MarshalUB1(ctx, driverCommon.UB1(code)); err != nil {
				t.Fatal(err)
			}
			rows := newTTCRows([]columnContext{{Name: []byte("C"), DataType: DtyChr}})
			if err := configured().unmarshalPrefetch(ctx, mar, rows, rows.columnContexts); err == nil {
				t.Fatalf("truncated %s returned nil error", TTCMsgTypeName[code])
			}
		})
	}
}

// marshalEmptyImplicitResultDCB writes the DCB layout for a result set with
// one minimal VARCHAR column, followed by its server cursor ID. It exercises
// the framing that TTIIMPLRES repeats once per returned cursor.
func marshalEmptyImplicitResultDCB(ctx context.Context, mar driverCommon.Marshaller, cursorID driverCommon.UB4) error {
	if err := mar.MarshalUB1(ctx, 0); err != nil { // KGL length
		return err
	}
	if err := mar.MarshalUB4(ctx, 0); err != nil { // maximum row size
		return err
	}
	if err := mar.MarshalUB4(ctx, 1); err != nil { // column count
		return err
	}
	if err := mar.MarshalUB1(ctx, 0); err != nil { // undocumented DCB marker
		return err
	}
	if err := newTTIoac(DtyChr, 1).MarshalTo(ctx, mar); err != nil {
		return err
	}
	if err := mar.MarshalUB1(ctx, 0); err != nil { // nullable
		return err
	}
	if err := mar.MarshalUB1(ctx, 1); err != nil { // column name length
		return err
	}
	if err := (&dynamicAllocatedArray{value: []byte("C")}).MarshalTo(ctx, mar); err != nil {
		return err
	}
	if err := (&dynamicAllocatedArray{}).MarshalTo(ctx, mar); err != nil { // schema name
		return err
	}
	if err := (&dynamicAllocatedArray{}).MarshalTo(ctx, mar); err != nil { // type name
		return err
	}
	if err := mar.MarshalUB2(ctx, 0); err != nil { // kernel position
		return err
	}
	if err := mar.MarshalUB4(ctx, 0); err != nil { // column flags
		return err
	}
	if err := (&dynamicAllocatedArray{}).MarshalTo(ctx, mar); err != nil { // current date
		return err
	}
	for range 4 { // DCB flags and metadata sizes
		if err := mar.MarshalUB4(ctx, 0); err != nil {
			return err
		}
	}
	if err := (&dynamicAllocatedArray{}).MarshalTo(ctx, mar); err != nil { // query compile key
		return err
	}
	return mar.MarshalUB4(ctx, cursorID)
}

// marshalSuccessfulOER emits the zero-valued OER attributes that terminate a
// prefetched implicit result set without reporting an Oracle error.
func marshalSuccessfulOER(ctx context.Context, mar driverCommon.Marshaller) error {
	marshalUB2 := func() error { return mar.MarshalUB2(ctx, 0) }
	marshalUB4 := func() error { return mar.MarshalUB4(ctx, 0) }
	marshalUB1 := func() error { return mar.MarshalUB1(ctx, 0) }
	if err := marshalUB2(); err != nil { // end-to-end ECID sequence number
		return err
	}
	if err := marshalUB4(); err != nil { // current row number
		return err
	}
	for range 5 { // retCode, array errors, cursor ID, and error position
		if err := marshalUB2(); err != nil {
			return err
		}
	}
	if err := marshalUB1(); err != nil { // SQL type
		return err
	}
	for range 3 { // fatal, flags, user cursor options
		if err := marshalUB2(); err != nil {
			return err
		}
	}
	for range 2 { // UPI parameter and warning flag
		if err := marshalUB1(); err != nil {
			return err
		}
	}
	if err := marshalUB4(); err != nil { // RBA
		return err
	}
	if err := marshalUB2(); err != nil { // partition ID
		return err
	}
	if err := marshalUB1(); err != nil { // table ID
		return err
	}
	if err := marshalUB4(); err != nil { // block number
		return err
	}
	if err := marshalUB2(); err != nil { // slot number
		return err
	}
	if err := marshalUB4(); err != nil { // OS error
		return err
	}
	for range 2 { // statement and call numbers
		if err := marshalUB1(); err != nil {
			return err
		}
	}
	if err := marshalUB2(); err != nil { // padding
		return err
	}
	if err := marshalUB4(); err != nil { // successful iterations
		return err
	}
	for range 3 { // OER dynamic arrays
		if err := (&dynamicAllocatedArray{}).MarshalTo(ctx, mar); err != nil {
			return err
		}
	}
	if err := marshalUB4(); err != nil { // OER message length
		return err
	}
	if err := marshalUB4(); err != nil { // extended error code
		return err
	}
	return mar.MarshalUB8(ctx, 0) // extended row count
}

func marshalEmptyRXH(ctx context.Context, mar driverCommon.Marshaller) error {
	if err := mar.MarshalUB1(ctx, 0); err != nil {
		return err
	}
	if err := mar.MarshalUB2(ctx, 0); err != nil {
		return err
	}
	if err := mar.MarshalUB2(ctx, 0); err != nil {
		return err
	}
	if err := mar.MarshalUB4(ctx, 0); err != nil {
		return err
	}
	if err := mar.MarshalUB2(ctx, 0); err != nil {
		return err
	}
	if err := (&dynamicAllocatedArray{}).MarshalTo(ctx, mar); err != nil {
		return err
	}
	return (&dynamicAllocatedArray{}).MarshalTo(ctx, mar)
}
