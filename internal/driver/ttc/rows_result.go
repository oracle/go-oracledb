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
	"database/sql/driver"
	"io"
	"reflect"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

/*
lobColumnContext captures LOB-specific metadata needed to decode pre-fetched
LOB payloads.

Description:

	LOB columns carry additional metadata beyond the generic columnContext fields.
	This metadata describes the LOB character set for text-based LOBs and the
	server-provided prefetch details used when the row already includes LOB data.

Fields:
  - CharsetForm: Oracle character set form for the LOB payload (for example, database or national character set).
  - CharsetID: Oracle character set identifier used to decode textual LOB payloads.
  - LobLength: Total logical length of the LOB value reported by the server.
  - PrefetchLength: Number of bytes included inline with the row as pre-fetched LOB data.
  - PrefetchChunkSize: Suggested chunk size for subsequent LOB reads when additional data must be fetched.
  - LobLocator: Opaque locator bytes that identify the server-side LOB and allow follow-up read operations.
*/
type lobColumnContext struct {
	CharsetForm       driverCommon.UB1
	CharsetID         driverCommon.UB2
	LobLength         driverCommon.UB4
	PrefetchLength    driverCommon.UB8
	PrefetchChunkSize driverCommon.UB4
	LobLocator        driverCommon.B1Array
}

// columnContext aggregates the metadata required to interpret a column's value
// at runtime. The TTC protocol delivers column descriptors separately from row
// payloads, so columnContext instances are constructed on-demand while scanning
// rows.
//
// Only the fields required by the decode helpers are surfaced here. Additional
// metadata can be appended without impacting existing decode helpers because
// the struct is passed by value.
type columnContext struct {
	Index                int
	Name                 driverCommon.B1Array
	SchemaName           driverCommon.B1Array
	DBTypeName           driverCommon.B1Array
	ScanType             *reflect.Type
	Length               int64
	DataType             DtyType
	Precision            int64
	Scale                int8
	KernelPosition       int
	ColumnFlags          uint32
	CharsetForm          uint8
	CharsetID            uint16
	Nullable             bool
	LobContext           *lobColumnContext
	serverTimeZoneOffset int16
}

// ttcRows implements database/sql/driver.Rows and optional ColumnType* interfaces.
// It buffers row data as raw protocol B1Array slices and exposes column metadata
// via the RowsColumnType* methods. Values returned by Next are currently []byte
// for each column; ColumnTypeScanType reflects this choice until type mapping evolves.
//
// Implemented interfaces:
//   - driver.Rows
//   - RowsColumnTypeDatabaseTypeName
//   - RowsColumnTypeLength
//   - RowsColumnTypeNullable
//   - RowsColumnTypePrecisionScale
//   - RowsColumnTypeScanType
type ttcRows struct {
	// row buffer
	rowData       [][]columnPayload
	currentRowIdx int
	numOfRows     int

	// metadata caches for ColumnType* interfaces
	columnContexts []columnContext
	shelf          *ttiShelf[driverCommon.MessageType]

	strictNullHandlingValue bool
}

// SetShelf injects the shared TTC shelf instance used to resolve codecs and
// connection-level properties during row decoding.
func (r *ttcRows) SetShelf(shelf *ttiShelf[driverCommon.MessageType]) {
	r.shelf = shelf
	r.strictNullHandlingValue = true
	if r.shelf.Shelf.GetConnectionProperties() != nil {
		r.strictNullHandlingValue = r.shelf.Shelf.GetConnectionProperties().IsStrictNullValueHandling()
	}
}

// Columns returns the column names for use by sql/database
// Columns implements driver.Rows.Columns. It returns the column names prepared
// during construction (newTTCRows) based on decoded column metadata.
func (r *ttcRows) Columns() []string {
	res := make([]string, len(r.columnContexts))
	for i, colCtx := range r.columnContexts {
		res[i] = driverCommon.B1ArrayToString(colCtx.Name)
	}
	// TODO : shall we cache this ?
	return res
}

// Next implements driver.Rows.Next. It advances the cursorId and assigns each
// column's payload as a value in dest. Row count is
// computed once and cached to avoid repeated len() calls.
func (r *ttcRows) Next(dest []driver.Value) error {
	if r.currentRowIdx >= r.numOfRows {
		return io.EOF
	}
	rawRow := r.rowData[r.currentRowIdx]
	for i := range rawRow {
		val, err := r.decodeColumnValue(i)
		if err != nil {
			return r.shelf.LocalizeError(err)
		}
		dest[i] = val
	}
	r.currentRowIdx++
	return nil
}

// decodeColumnValue returns the decoded driver.Value for the current row's column i.
//
// Behaviour:
//   - Handlers normalize Oracle NULL payloads to nil.
//   - Constructed values pass through without byte decoding.
//   - When a decoder is available for the column's TTC datatype, it is invoked; otherwise
//     the raw protocol bytes are surfaced unchanged.
//
// Errors:
//   - Any error propagated from the registered TTC codec for the column's datatype.
func (r *ttcRows) decodeColumnValue(i int) (driver.Value, error) {
	// Fast-path locals to minimize repeated bounds checks and lookups
	colCtx := r.columnContexts[i]
	dtype := colCtx.DataType
	scale := colCtx.Scale
	payload := r.rowData[r.currentRowIdx][i]
	colCtx.LobContext = payload.lob
	colCtx.serverTimeZoneOffset = r.shelf.getServerTimeZoneOffset()
	// Handle Oracle SQL NULL (typically raw length zero is NULL).
	if payload.data == nil {
		return r.handleNull(i, dtype, scale), nil
	}

	data, raw := payload.data.(driverCommon.B1Array)
	if !raw {
		return payload.data, nil
	}

	decoder, err := r.shelf.GetCodecFactory().getDecoder(dtype)
	if err != nil || decoder == nil {
		// Preserve unknown types as raw bytes
		return data, nil
	}

	val, err := decoder.decodeToType(colCtx, data)
	if err != nil {
		// Preserve the decoder error for localization.
		return nil, r.shelf.LocalizeError(err)
	}

	return val, nil
}

// handleNull determines the driver.Value to surface for a NULL result-set column.
//
// Preconditions:
//   - Column nullability is checked by decodeColumnValue before this helper is invoked.
//
// Inputs:
//   - i: zero-based column index within the current row buffer.
//   - dtype: TTC datatype negotiated for the column.
//   - scale: TTC scale metadata used for numeric default evaluation.
//
// Outputs:
//   - driver.Value representing the NULL substitution (for example 0, "", time.Time{}),
//     or nil when no specific default applies.
func (r *ttcRows) handleNull(i int, dtype DtyType, scale int8) driver.Value {
	if !r.strictNullHandlingValue {
		if val, ok := _defaultValueForNull(dtype, scale); ok {
			return val
		}
	}
	// Fallback to nil when the type is unrecognised; behaviour matches legacy default.
	return nil
}

// _defaultValueForNull returns the driver.Value substitution that should be used
// for a NULL column based on its negotiated TTC datatype metadata.
//
// Inputs:
//   - dtype: TTC datatype negotiated for the column being read.
//   - scale: numeric scale metadata used to refine NUMBER defaults.
//
// Outputs:
//   - driver.Value providing the default representation for the NULL column.
//   - bool flag indicating whether a default was found. When false, callers
//     should surface a nil value.
//
// Errors:
//   - This helper does not return errors; defaults are mapped deterministically.
//
// Numeric defaults consider scale to decide between integer, floating-point, or
// decimal string representations.
func _defaultValueForNull(dtype DtyType, scale int8) (driver.Value, bool) {
	if resolver, ok := defaultNullValueResolvers[dtype]; ok {
		return resolver(scale)
	}

	return nil, false
}

// defaultNullValueResolver resolves the default substitution for a TTC datatype.
// Implementations may inspect the column scale (for numbers). The bool return
// indicates whether the resolver produced a value.
type defaultNullValueResolver func(scale int8) (driver.Value, bool)

// constantDefaultNullValue returns a resolver that always yields the provided
// driver.Value, regardless of scale metadata.
func constantDefaultNullValue(value driver.Value) defaultNullValueResolver {
	return func(int8) (driver.Value, bool) {
		return value, true
	}
}

// defaultNullValueResolvers defines the default substitution for TTC datatypes when
// strict null handling is disabled. Numeric types are handled separately because the
// default representation depends on scale metadata.
var defaultNullValueResolvers = map[DtyType]defaultNullValueResolver{
	DtyNum:      defaultNumericValue,
	DtyVnu:      defaultNumericValue,
	DtyInt:      defaultNumericValue,
	DtyPdn:      defaultNumericValue,
	DtyUin:      defaultNumericValue,
	DtySls:      defaultNumericValue,
	DtyIbFloat:  constantDefaultNullValue(float64(0)),
	DtyIbDouble: constantDefaultNullValue(float64(0)),
	DtyChr:      constantDefaultNullValue(""),
	DtyStr:      constantDefaultNullValue(""),
	DtyVCS:      constantDefaultNullValue(""),
	DtyAfc:      constantDefaultNullValue(""),
	DtyAvc:      constantDefaultNullValue(""),
	DtyBin:      constantDefaultNullValue(driverCommon.B1Array{}),
	DtyVbi:      constantDefaultNullValue(driverCommon.B1Array{}),
	DtyLbi:      constantDefaultNullValue(driverCommon.B1Array{}),
	DtyBlob:     constantDefaultNullValue(driverCommon.B1Array{}),
	DtyDblob:    constantDefaultNullValue(driverCommon.B1Array{}),
	DtyBol:      constantDefaultNullValue(false),
	DtyDat:      constantDefaultNullValue(time.Time{}),
	DtyEdate:    constantDefaultNullValue(time.Time{}),
	DtyStamp:    constantDefaultNullValue(time.Time{}),
	DtyEstamp:   constantDefaultNullValue(time.Time{}),
	DtyStz:      constantDefaultNullValue(time.Time{}),
	DtyEstz:     constantDefaultNullValue(time.Time{}),
	DtySitz:     constantDefaultNullValue(time.Time{}),
	DtyEsitz:    constantDefaultNullValue(time.Time{}),
	DtyTime:     constantDefaultNullValue(time.Time{}),
	DtyEtime:    constantDefaultNullValue(time.Time{}),
	DtyTtz:      constantDefaultNullValue(time.Time{}),
	DtyEttz:     constantDefaultNullValue(time.Time{}),
	DtyIym:      constantDefaultNullValue("00-00"),
	DtyEiym:     constantDefaultNullValue("00-00"),
	DtyIds:      constantDefaultNullValue("00 00:00:00.0"),
	DtyEids:     constantDefaultNullValue("00 00:00:00.0"),
}

// defaultNumericValue calculates the default driver.Value to surface for
// numeric TTC datatypes when the column value is NULL.
//
// Inputs:
//   - scale: TTC scale metadata used to discriminate between integer,
//     floating-point, and arbitrary-precision defaults.
//
// Outputs:
//   - driver.Value representing the numeric default (int64, float64, or
//     decimal string).
//   - bool indicating whether the resolver produced a value. This allows the
//     method to be used directly as a defaultNullValueResolver implementation.
//
// Errors:
//   - This helper does not return errors; the mapping is deterministic.
func defaultNumericValue(scale int8) (driver.Value, bool) {
	switch scale {
	case 0:
		return int64(0), true
	case NumberScaleFloatSentinel:
		return float64(0), true
	default:
		return "0", true
	}
}

// Close implements driver.Rows.Close. No network/protocol resources are owned
// by this object currently, so this is a no-op.
func (r *ttcRows) Close() error {
	common.Odl.Fine("closing rows")
	return nil
}

// newTTCRows constructs a ttcRows instance from decoded column metadata.
// It precomputes and caches fields backing the RowsColumnType* interfaces
// for fast access during scanning and introspection.
func newTTCRows(columnContexts []columnContext) *ttcRows {
	n := len(columnContexts)
	if n == 0 {
		return &ttcRows{strictNullHandlingValue: true}
	}
	rows := &ttcRows{strictNullHandlingValue: true}

	rows.columnContexts = make([]columnContext, n)
	for i := 0; i < n; i++ {
		rows.columnContexts[i] = columnContexts[i]
	}

	return rows
}

// ColumnTypeDatabaseTypeName implements RowsColumnTypeDatabaseTypeName.
// It returns the database-specific type name (e.g., VARCHAR2, NUMBER).
func (r *ttcRows) ColumnTypeDatabaseTypeName(index int) string {
	// inline translation here waiting for refactor of our type registry
	switch r.columnContexts[index].DataType {
	case DtyChr:
		if r.columnContexts[index].CharsetForm == 2 {
			return "NVARCHAR2"
		}
		return "VARCHAR2"
	case DtyNum, DtyVnu:
		if r.columnContexts[index].Precision != 0 && r.columnContexts[index].Precision == -127 {
			return "FLOAT"
		}
		return "NUMBER"
	case DtyLng:
		return "LONG"
	case DtyDat:
		return "DATE"
	case DtyBin:
		return "RAW"
	case DtyLbi:
		return "LONG RAW"
	case DtyAfc:
		if r.columnContexts[index].CharsetForm == 2 {
			return "NCHAR"
		}
		return "CHAR"
	case DtyIbFloat:
		return "BINARY_FLOAT"
	case DtyIbDouble:
		return "BINARY_DOUBLE"
	case DtyCur:
		return "REFCURSOR"
	case DtyRdd, DtyBuri:
		return "ROWID"
	case DtyINty:
		return "Internal Named Type" // enough for now
	case DtyIref:
		return "Internal Named Type" // enough for now
	case DtyClob:
		if r.columnContexts[index].CharsetForm == 2 {
			return "NCLOB"
		}
		return "CLOB"
	case DtyBlob:
		return "BLOB"
	case DtyBFil:
		return "BFILE"
	case DtyJSON:
		return "JSON"
	case DtyVec:
		return "VECTOR"
	case DtyStamp:
		return "TIMESTAMP"
	case DtyStz:
		return "TIMESTAMP WITH TIME ZONE"
	case DtyIym:
		return "INTERVALYM"
	case DtyIds:
		return "INTERVALDS"
	case DtySitz:
		return "TIMESTAMP WITH LOCAL TIME ZONE"
	case DtyBol:
		return "BOOLEAN"
	default:
		common.Odl.Warn("Do not have name mapping", "type", r.columnContexts[index].DataType)
		return ""
	}
}

// ColumnTypeLength implements RowsColumnTypeLength. It returns the byte length
// for variable-length types when available.
func (r *ttcRows) ColumnTypeLength(index int) (int64, bool) {
	if index < 0 || index >= len(r.columnContexts) {
		return 0, false
	}
	if r.columnContexts[index].Length <= 0 {
		return 0, false
	}
	return r.columnContexts[index].Length, true
}

// ColumnTypeNullable implements RowsColumnTypeNullable. It returns whether the
// column may be NULL and whether the information is available.
func (r *ttcRows) ColumnTypeNullable(index int) (bool, bool) {
	return r.columnContexts[index].Nullable, true
}

// ColumnTypePrecisionScale implements RowsColumnTypePrecisionScale. It should return
// the precision and scale for decimal types. If not applicable, ok should be false.
func (r *ttcRows) ColumnTypePrecisionScale(index int) (int64, int64, bool) {
	dty := r.columnContexts[index].DataType
	if dty == DtyNum || dty == DtyVnu {
		return r.columnContexts[index].Precision, int64(r.columnContexts[index].Scale), true
	}

	return 0, 0, false
}

// ColumnTypeScanType implements RowsColumnTypeScanType. It returns the Go type
// into which database values will be scanned. Currently, []byte is used for all
// columns, matching the raw protocol representation returned by Next.
func (r *ttcRows) ColumnTypeScanType(index int) reflect.Type {
	if r.columnContexts[index].ScanType == nil {
		decoder, err := r.shelf.GetCodecFactory().getDecoder(r.columnContexts[index].DataType)
		if err != nil {
			common.Odl.Warn("Do not have decode mapping", "type", r.columnContexts[index].DataType)
			return reflect.TypeOf([]byte(nil))
		}
		r.columnContexts[index].ScanType = new(decoder.getScanType(r.columnContexts[index]))
	}
	return *r.columnContexts[index].ScanType
}

// ttcRowsRefCursor owns state specific to an already-open server REF CURSOR.
type ttcRowsRefCursor struct {
	*ttcRows
	cursorID driverCommon.SB4
	executor *refCursorExecutor
	fetched  bool
	closed   bool
	closeErr error
}

// ttcRowsRefCursorImplicitFetch exposes ordered implicit cursors as result sets.
type ttcRowsRefCursorImplicitFetch struct {
	*ttcRowsRefCursor
	implicitRows     []*ttcRowsRefCursor
	currentResultSet int
}

// newRefCursorResultRows adds REF CURSOR ownership and cleanup to base rows.
func newRefCursorResultRows(rows *ttcRows, cursorID driverCommon.SB4) *ttcRowsRefCursor {
	return &ttcRowsRefCursor{ttcRows: rows, cursorID: cursorID}
}

// newRefCursorRows creates a REF CURSOR rows object and associates its fetch
// executor. The rows object owns the executor so a cursor has one fetch path.
func newRefCursorRows(cursorID driverCommon.SB4, columns []columnContext) *ttcRowsRefCursor {
	rows := newRefCursorResultRows(newTTCRows(columns), cursorID)
	rows.executor = newRefCursorExecutor(cursorID, columns)
	return rows
}

// attachShelf supplies the shelf for row decoding, cursor cleanup, and fetching.
func (r *ttcRowsRefCursor) attachShelf(shelf *ttiShelf[driverCommon.MessageType]) {
	r.ttcRows.SetShelf(shelf)
	r.executor.SetShelf(shelf)
}

// attachSessionContext forwards the session context to the cursor's fetch executor.
func (r *ttcRowsRefCursor) attachSessionContext(sessCtx *driverCommon.SessionContext) {
	r.executor.SetSessionContext(sessCtx)
}

// Next delegates row decoding to the common rows implementation.
func (r *ttcRowsRefCursor) Next(dest []driver.Value) error {
	if r.closed {
		return io.EOF
	}
	return r.ttcRows.Next(dest)
}

// Close queues server cleanup after closing nested cursor values.
func (r *ttcRowsRefCursor) Close() error {
	if r.closed {
		return r.closeErr
	}
	r.closed = true
	for _, row := range r.rowData {
		for _, payload := range row {
			if child, ok := payload.data.(*ttcRowsRefCursor); ok && child != nil {
				if err := child.Close(); err != nil && r.closeErr == nil {
					r.closeErr = err
				}
			}
		}
	}
	if r.cursorID != 0 && r.shelf != nil {
		if err := r.closeServerCursor(); err != nil && r.closeErr == nil {
			r.closeErr = err
		}
	}
	return r.closeErr
}

// closeServerCursor queues the OCCA for the next round trip.
func (r *ttcRowsRefCursor) closeServerCursor() error {
	msg, err := r.shelf.GetMessageFactory().(Factory).GetMessageForFunction(TTIPFN, occa)
	if err != nil {
		return err
	}
	msg.(*tTIOcca).setCursorIDs([]driverCommon.UB4{driverCommon.UB4(r.cursorID)})
	if err = r.shelf.GetMessageStreamer().Push(context.Background(), msg); err != nil {
		return err
	}
	r.shelf.removeRefCursor(r)
	r.cursorID = 0
	return nil
}

// Fetch obtains data for an opened REF CURSOR exactly once.
func (r *ttcRowsRefCursor) Fetch(ctx context.Context) error {
	if r.fetched {
		return nil
	}
	if !r.shelf.hasRefCursor(r) {
		return common.NewOracleError(oracleErrors.RefCursorConnectionMismatch, nil)
	}
	if err := r.executor.Fetch(ctx, r); err != nil {
		return err
	}
	r.fetched = true
	return nil
}

// newImplicitResultRows fetches any implicit cursors not prefetched by the
// server and build the instance for ttcRowsRefCursorImplicitFetch.
func newImplicitResultRows(ctx context.Context, resultSets []*ttcRowsRefCursor) (*ttcRowsRefCursorImplicitFetch, error) {
	for _, resultSet := range resultSets {
		if resultSet != nil && !resultSet.fetched {
			if err := resultSet.Fetch(ctx); err != nil {
				return nil, err
			}
		}
	}
	return &ttcRowsRefCursorImplicitFetch{ttcRowsRefCursor: newRefCursorResultRows(newTTCRows(nil), 0), implicitRows: resultSets}, nil
}

// getActive returns the implicit result set selected by currentResultSet. It
// returns nil after the final result set has been consumed.
func (r *ttcRowsRefCursorImplicitFetch) getActive() *ttcRowsRefCursor {
	if r.currentResultSet < len(r.implicitRows) {
		return r.implicitRows[r.currentResultSet]
	}
	return nil
}

// Columns returns the column names of the active implicit result set. It
// returns nil when there is no active result set.
func (r *ttcRowsRefCursorImplicitFetch) Columns() []string {
	if rows := r.getActive(); rows != nil {
		return rows.Columns()
	}
	return nil
}

// Next reads the next row from the active implicit result set. It returns
// io.EOF when this wrapper is closed, there is no active result set, or the
// active result set has no more rows.
func (r *ttcRowsRefCursorImplicitFetch) Next(dest []driver.Value) error {
	if r.closed {
		return io.EOF
	}
	if rows := r.getActive(); rows != nil {
		return rows.Next(dest)
	}
	return io.EOF
}

// HasNextResultSet reports whether another implicit result set follows the
// currently active result set.
func (r *ttcRowsRefCursorImplicitFetch) HasNextResultSet() bool {
	return r.currentResultSet+1 < len(r.implicitRows)
}

// NextResultSet advances to the next implicit result set. It returns io.EOF
// when the active result set is already the last one.
func (r *ttcRowsRefCursorImplicitFetch) NextResultSet() error {
	if !r.HasNextResultSet() {
		return io.EOF
	}
	r.currentResultSet++
	return nil
}

// Close closes every implicit result set, retaining the first close error for
// later calls. Closing this wrapper more than once is idempotent.
func (r *ttcRowsRefCursorImplicitFetch) Close() error {
	if r.closed {
		return r.closeErr
	}
	r.closed = true
	for _, rows := range r.implicitRows {
		if rows != nil {
			if err := rows.Close(); err != nil && r.closeErr == nil {
				r.closeErr = err
			}
		}
	}
	return r.closeErr
}

// ttcResult implements database/sql/driver.Result, used for DML or exec results.
type ttcResult struct {
	rowsAffected int64
	shelf        *ttiShelf[driverCommon.MessageType]
}

// RowsAffected returns the number of rows affected by the last exec.
func (r *ttcResult) RowsAffected() (int64, error) {
	return r.rowsAffected, nil
}

// LastInsertId reports that Oracle does not support retrieving a last insert ID
// through this driver path and returns a shelf-localized error.
func (r *ttcResult) LastInsertId() (int64, error) {
	return 0, r.shelf.LocalizeError(common.NewOracleError(oracleErrors.UnsupportedFeature, nil, "LastInsertId"))
}
