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
	"sync"
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
	server-provided logical length and locator used when the row includes LOB data.

Fields:
  - CharsetForm: Oracle character set form for the LOB payload (for example, database or national character set).
  - CharsetID: Oracle character set identifier used to decode textual LOB payloads.
  - LocatorByteLength: Byte length of the locator representation in the RXD row.
  - TotalLobLength: Total logical LOB length reported by the server.
  - LobLocator: Opaque locator bytes that identify the server-side LOB and allow follow-up reads.
*/
type lobColumnContext struct {
	// JSON values use a separate LOB wire layout from BLOB/CLOB locators.
	LobLength         driverCommon.UB4
	PrefetchLength    driverCommon.UB8
	PrefetchChunkSize driverCommon.UB4
	CharsetForm       driverCommon.UB1
	CharsetID         driverCommon.UB2
	LocatorByteLength driverCommon.UB4
	TotalLobLength    driverCommon.UB8
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
	Index          int
	Name           driverCommon.B1Array
	SchemaName     driverCommon.B1Array
	DBTypeName     driverCommon.B1Array
	ScanType       *reflect.Type
	Length         int64
	DataType       DtyType
	Precision      int64
	Scale          int8
	KernelPosition int
	ColumnFlags    uint32
	CharsetForm    uint8
	CharsetID      uint16
	Nullable       bool
	// LobContext is populated from the per-row payload before decoding.
	LobContext *lobColumnContext
	// serverTimeZoneOffset is copied from the shelf for timestamp decoding.
	serverTimeZoneOffset int16
}

// rowsLifecycle groups the state that controls whether Rows and its
// locator-backed values may continue to use the physical session. It is
// protected by ttcRows.mu.
type rowsLifecycle struct {
	// closed makes Close idempotent and invalidates child locators.
	closed bool
	// ctx follows the caller's QueryContext and cancel stops queued or in-flight
	// locator work when Rows closes.
	ctx    context.Context
	cancel context.CancelFunc
	// statement is the Statement which produced this result. ownsStatement is
	// true only for the direct Connection.QueryContext path.
	statement     *Statement
	ownsStatement bool
	// lobs owns every locator value that remains usable. Rows.Close and owner
	// invalidation end their lifetime; Rows.Next deliberately does not.
	lobs map[*streamedLob]struct{}
	// decodingLobs records values created while decoding one row, allowing a
	// partial row-decode failure to invalidate only those values.
	decodingLobs map[*streamedLob]struct{}
}

// ttcRows implements database/sql/driver.Rows and owns one completed query
// result. The current executor buffers row payloads, while ttcRows also retains
// the Statement/session lifetime required for future locator-backed LOB reads.
//
// A direct Connection.QueryContext result owns its internally-created
// Statement and closes it from Rows.Close. Rows produced by a prepared
// Statement hold a non-owning reference and merely detach on close so that the
// Statement can be reused. The mutex protects only lifecycle fields; row
// iteration remains governed by database/sql's single-consumer Rows contract.
//
// Implemented interfaces:
//   - driver.Rows
//   - RowsColumnTypeDatabaseTypeName
//   - RowsColumnTypeLength
//   - RowsColumnTypeNullable
//   - RowsColumnTypePrecisionScale
//   - RowsColumnTypeScanType
type ttcRows struct {
	// mu protects lifecycle, row index, and LOB ownership registries. Next also
	// snapshots row buffers under this lock. It must not be held while calling
	// Statement or streamedLob methods or performing TTC cleanup.
	mu        sync.Mutex
	lifecycle rowsLifecycle

	// rowData stores each column's value and optional LOB metadata for every row.
	rowData [][]columnPayload
	// currentRowIdx is the zero-based row returned by the next call to Next.
	currentRowIdx int
	// numOfRows caches the completed result row count for Next.
	numOfRows int

	// columnContexts stores immutable column metadata and backs ColumnType APIs.
	columnContexts []columnContext
	// shelf supplies codecs, localization, connection properties, and the shared
	// physical-session operation guard.
	shelf *ttiShelf[driverCommon.MessageType]
	// sessionContext supplies negotiated database and national character sets to
	// the existing clobExecutor.
	sessionContext *driverCommon.SessionContext
	// strictNullHandlingValue controls whether SQL NULL is preserved or mapped to
	// legacy type-specific zero values.
	strictNullHandlingValue bool
}

// SetShelf injects the shared TTC shelf instance used to resolve codecs and
// connection-level properties during row decoding. It is called during result
// setup, before Rows is exposed to the caller.
func (r *ttcRows) SetShelf(shelf *ttiShelf[driverCommon.MessageType]) {
	r.shelf = shelf
	r.strictNullHandlingValue = true
	if r.shelf.Shelf.GetConnectionProperties() != nil {
		r.strictNullHandlingValue = r.shelf.Shelf.GetConnectionProperties().IsStrictNullValueHandling()
	}
}

// SetSessionContext injects immutable negotiated character-set state used by
// locator-backed CLOB and NCLOB values.
func (r *ttcRows) SetSessionContext(sessionContext *driverCommon.SessionContext) {
	r.sessionContext = sessionContext
}

// setContext gives Rows a lifecycle context derived from the original public
// QueryContext context, not the statement's exchange-only child context.
func (r *ttcRows) setContext(parent context.Context) {
	r.mu.Lock()
	oldCancel := r.lifecycle.cancel
	r.lifecycle.ctx, r.lifecycle.cancel = context.WithCancel(parent)
	closed := r.lifecycle.closed
	if closed {
		r.lifecycle.cancel()
	}
	r.mu.Unlock()
	if oldCancel != nil {
		oldCancel()
	}
}

// Columns implements driver.Rows.Columns. It returns the column names prepared
// during construction from decoded column metadata. The returned slice is a
// fresh allocation, so callers cannot mutate the stored descriptors.
func (r *ttcRows) Columns() []string {
	res := make([]string, len(r.columnContexts))
	for i, colCtx := range r.columnContexts {
		res[i] = driverCommon.B1ArrayToString(colCtx.Name)
	}
	return res
}

// Next implements driver.Rows.Next. It snapshots the current row and its LOB
// metadata while holding mu, then decodes outside the lock. This lets Close
// clear the parent result buffers without invalidating a row already in flight.
func (r *ttcRows) Next(dest []driver.Value) error {
	r.mu.Lock()
	if r.lifecycle.closed {
		r.mu.Unlock()
		return io.EOF
	}
	if r.lifecycle.ctx != nil {
		if err := r.lifecycle.ctx.Err(); err != nil {
			r.mu.Unlock()
			return err
		}
	}
	rowIndex := r.currentRowIdx
	if rowIndex >= r.numOfRows {
		r.mu.Unlock()
		return io.EOF
	}
	rawRow := r.rowData[rowIndex]
	clear(r.lifecycle.decodingLobs)
	r.mu.Unlock()
	for i := range rawRow {
		val, err := r.decodeColumnPayload(rawRow, i)
		if err != nil {
			r.invalidateCurrentRowLobs()
			return r.shelf.LocalizeError(err)
		}
		dest[i] = val
	}
	r.mu.Lock()
	if r.lifecycle.closed {
		r.mu.Unlock()
		r.invalidateCurrentRowLobs()
		return io.EOF
	}
	r.currentRowIdx++
	r.mu.Unlock()
	return nil
}

// decodeColumnPayload returns the decoded driver.Value for columnIndex in a
// previously snapshotted row. The row snapshot keeps decoding independent of
// Rows.Close clearing the parent result buffers.
//
// Behaviour:
//   - Handlers normalize Oracle NULL payloads to nil.
//   - Constructed values pass through without byte decoding.
//   - When a decoder is available for the column's TTC datatype, it is invoked; otherwise
//     the raw protocol bytes are surfaced unchanged.
//
// Errors:
//   - Any malformed LOB metadata or error propagated from the registered TTC codec.
func (r *ttcRows) decodeColumnPayload(rawRow []columnPayload, columnIndex int) (driver.Value, error) {
	// Copy the column context because row-specific values must not mutate the
	// shared descriptor stored on ttcRows.
	colCtx := r.columnContexts[columnIndex]
	dtype := colCtx.DataType
	scale := colCtx.Scale
	payload := rawRow[columnIndex]
	colCtx.LobContext = payload.lob
	colCtx.serverTimeZoneOffset = r.shelf.getServerTimeZoneOffset()
	// Non-NULL BLOB, CLOB, and NCLOB columns are represented by a locator source.
	// The database/sql scan destination selects streaming or materialization.
	// JSON uses a LOB transport internally but retains its historical string.
	if dtype == DtyBlob || dtype == DtyClob {
		metadata := colCtx.LobContext
		if metadata == nil {
			return nil, common.NewOracleError(
				oracleErrors.InvalidLobSource,
				nil,
				"row metadata",
			)
		}
		// The current RXD protocol represents SQL NULL with a zero locator length and
		// does not send the remaining locator fields. Do this check before
		// requiring a locator, but never treat a non-empty prefix as NULL. Keep
		// LOB NULLs as nil regardless of strictNullHandling so typed scanners can
		// distinguish SQL NULL from a non-NULL empty LOB locator.
		if payload.data == nil && metadata.LocatorByteLength == 0 && len(metadata.LobLocator) == 0 {
			return nil, nil
		}
		if len(metadata.LobLocator) == 0 {
			return nil, common.NewOracleError(
				oracleErrors.InvalidLobSource,
				nil,
				"LOB locator",
			)
		}
		var data driverCommon.B1Array
		if payload.data != nil {
			var ok bool
			data, ok = payload.data.(driverCommon.B1Array)
			if !ok {
				return nil, common.NewOracleError(oracleErrors.InvalidLobSource, nil, "LOB prefix")
			}
		}
		value, err := newStreamedLob(r, dtype, data, *metadata)
		if err != nil {
			return nil, err
		}
		if !r.registerLob(value) {
			value.invalidate()
			return nil, common.NewOracleError(oracleErrors.LobValueInvalidated, nil, "Rows decode")
		}
		return value, nil
	}
	// Handle Oracle SQL NULL (typically raw length zero is NULL).
	if payload.data == nil {
		return r.handleNull(dtype, scale), nil
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

// registerLob records a newly decoded locator until it reaches EOF, is closed,
// or its owning Rows closes. It returns false if Rows already closed.
func (r *ttcRows) registerLob(value *streamedLob) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lifecycle.closed {
		return false
	}
	if r.lifecycle.lobs == nil {
		r.lifecycle.lobs = make(map[*streamedLob]struct{})
	}
	if r.lifecycle.decodingLobs == nil {
		r.lifecycle.decodingLobs = make(map[*streamedLob]struct{})
	}
	r.lifecycle.lobs[value] = struct{}{}
	r.lifecycle.decodingLobs[value] = struct{}{}
	return true
}

// releaseLob removes a completed, closed, or detached locator from Rows
// ownership. It no longer needs invalidation when Rows subsequently closes.
func (r *ttcRows) releaseLob(value *streamedLob) {
	r.mu.Lock()
	delete(r.lifecycle.lobs, value)
	delete(r.lifecycle.decodingLobs, value)
	r.mu.Unlock()
}

// invalidateCurrentRowLobs detaches and invalidates values created while
// decoding the current row. It is used for partial row-decode failures.
func (r *ttcRows) invalidateCurrentRowLobs() {
	r.mu.Lock()
	values := make([]*streamedLob, 0, len(r.lifecycle.decodingLobs))
	for value := range r.lifecycle.decodingLobs {
		values = append(values, value)
		delete(r.lifecycle.lobs, value)
	}
	clear(r.lifecycle.decodingLobs)
	r.mu.Unlock()
	for _, value := range values {
		value.invalidate()
	}
}

// beginLobOperation validates Rows, snapshots its context, and acquires the
// connection-wide TTC guard. It rechecks Rows after acquiring so Close can win
// while an operation is queued without allowing a later RPC.
func (r *ttcRows) beginLobOperation() (context.Context, func(), error) {
	r.mu.Lock()
	if r.lifecycle.closed {
		r.mu.Unlock()
		return nil, nil, common.NewOracleError(oracleErrors.LobValueInvalidated, nil, "Rows owner")
	}
	ctx := r.lifecycle.ctx
	shelf := r.shelf
	r.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	if shelf == nil || shelf.lobState == nil || shelf.lobState.isInvalidated() {
		return nil, nil, common.NewOracleError(oracleErrors.LobValueInvalidated, nil, "LOB session")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	release, err := shelf.synchronizer.begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	r.mu.Lock()
	closed := r.lifecycle.closed
	r.mu.Unlock()
	if shelf.lobState.isInvalidated() {
		release()
		return nil, nil, common.NewOracleError(oracleErrors.LobValueInvalidated, nil, "LOB session")
	}
	if closed || ctx.Err() != nil {
		release()
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		return nil, nil, common.NewOracleError(oracleErrors.LobValueInvalidated, nil, "Rows owner")
	}
	return ctx, release, nil
}

// isClosed reports whether the owning Rows has invalidated its child locators.
// It is safe to call while streamedLob.mu is held because Rows never holds r.mu
// while acquiring a streamedLob mutex.
func (r *ttcRows) isClosed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lifecycle.closed
}

// contextErr returns the query/Rows lifecycle error without exposing the
// mutable context field to locator-backed values.
func (r *ttcRows) contextErr() error {
	r.mu.Lock()
	ctx := r.lifecycle.ctx
	r.mu.Unlock()
	return ctx.Err()
}

// invalidateAfterUnsafeLobRPC closes Rows locally after a canceled, transport,
// or protocol-level locator exchange. It deliberately does not call into any
// streamedLob while an RPC caller may still hold that value's mutex. Posting the
// streamer-stale event marks the physical connection invalid, preventing
// database/sql from reusing a TTC stream whose terminal response was not
// proven to have been consumed.
func (r *ttcRows) invalidateAfterUnsafeLobRPC() {
	stmt, _, first := r.transitionToClosed(nil)
	if first {
		if stmt != nil {
			stmt.detachRows(r)
		}
	}
	if r.shelf != nil {
		r.shelf.getEventService().post(streamerStaleEvent)
	}
}

// handleNull determines the driver.Value to surface for a NULL result-set
// column. The caller has already identified the wire value as NULL.
func (r *ttcRows) handleNull(dtype DtyType, scale int8) driver.Value {
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
// Parameters:
//   - dtype: TTC datatype negotiated for the column being read.
//   - scale: numeric scale metadata used to refine NUMBER defaults.
//
// Returns:
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
// Parameters:
//   - scale: TTC scale metadata used to discriminate between integer,
//     floating-point, and arbitrary-precision defaults.
//
// Returns:
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

// Close implements driver.Rows.Close. It detaches reusable prepared statements
// and closes a Statement only when Connection.QueryContext transferred direct-
// query ownership to these Rows.
func (r *ttcRows) Close() error {
	common.Odl.Debug("closing rows")
	stmt, owned, first := r.transitionToClosed(nil)
	if !first {
		return nil
	}
	if stmt == nil {
		return nil
	}
	stmt.detachRows(r)
	if owned {
		return stmt.closeAfterRows()
	}
	return nil
}

// transitionToClosed atomically closes Rows, cancels queued/in-flight locator work,
// detaches statement ownership, and clears the outstanding LOB registry. It
// does not acquire any streamedLob mutex: owner.closed is the authoritative
// invalidation signal, which keeps Rows.Close from blocking behind a stalled
// network read. If expectedStatement is non-nil, the transition occurs only
// for that statement.
func (r *ttcRows) transitionToClosed(expectedStatement *Statement) (*Statement, bool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lifecycle.closed || (expectedStatement != nil && r.lifecycle.statement != expectedStatement) {
		return nil, false, false
	}
	r.lifecycle.closed = true
	if r.lifecycle.cancel != nil {
		r.lifecycle.cancel()
	}
	stmt := r.lifecycle.statement
	owned := r.lifecycle.ownsStatement
	clear(r.lifecycle.lobs)
	clear(r.lifecycle.decodingLobs)
	r.lifecycle.statement = nil
	r.lifecycle.ownsStatement = false
	// Escaped Lob values retain their small Rows owner to observe invalidation.
	// Drop buffered result payloads here so one closed Lob cannot retain the
	// complete query result in memory.
	r.rowData = nil
	return stmt, owned, true
}

// attachStatement records the prepared Statement which produced these Rows.
// The reference is non-owning until the direct Connection query path promotes
// it with takeStatementOwnership.
func (r *ttcRows) attachStatement(stmt *Statement) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.lifecycle.closed {
		r.lifecycle.statement = stmt
	}
}

// takeStatementOwnership transfers an internally-created direct-query
// Statement to Rows so database/sql keeps both cursor and connection alive
// until Rows.Close. It returns false if Rows was already closed or was not
// attached to stmt, allowing the connection path to close stmt rather than
// leak it.
func (r *ttcRows) takeStatementOwnership(stmt *Statement) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.lifecycle.closed && r.lifecycle.statement == stmt {
		r.lifecycle.ownsStatement = true
		return true
	}
	return false
}

// closeFromStatement marks Rows closed without calling back into Statement.
// Statement.Close uses this path to avoid a recursive close cycle.
func (r *ttcRows) closeFromStatement(stmt *Statement) error {
	r.transitionToClosed(stmt)
	return nil
}

// newTTCRows constructs a ttcRows instance from decoded column metadata and
// initializes the lifecycle context and LOB ownership registries.
func newTTCRows(columnContexts []columnContext) *ttcRows {
	n := len(columnContexts)
	rows := &ttcRows{
		strictNullHandlingValue: true,
		lifecycle: rowsLifecycle{
			ctx:          context.Background(),
			lobs:         make(map[*streamedLob]struct{}),
			decodingLobs: make(map[*streamedLob]struct{}),
		},
	}
	if n == 0 {
		return rows
	}

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
// into which database values will be scanned. LOB columns use any because the
// private TTC package returns an internal locator source for database/sql to
// transfer to the public LOB API.
func (r *ttcRows) ColumnTypeScanType(index int) reflect.Type {
	dtype := r.columnContexts[index].DataType
	if dtype == DtyBlob || dtype == DtyClob {
		// For non-NULL values, database/sql receives this private locator source as
		// the driver value; the public datatype scanner performs ownership transfer.
		// This is a go-oracledb-specific database/sql extension: *streamedLob is
		// outside the documented Scanner source types and standard driver.Value
		// result types, but it preserves direct Scan into typed LOB handles.
		return reflect.TypeFor[any]()
	}
	if r.columnContexts[index].ScanType == nil {
		decoder, err := r.shelf.GetCodecFactory().getDecoder(dtype)
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
	statement        *Statement
	ownsStatement    bool
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
	r.ttcRows.SetSessionContext(sessCtx)
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
	if err := r.ttcRows.Close(); err != nil && r.closeErr == nil {
		r.closeErr = err
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
		if resultSet != nil {
			resultSet.ttcRows.setContext(ctx)
		}
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

// setContext binds each child result set to the public query context rather
// than the short-lived execution context used while the PL/SQL call runs.
func (r *ttcRowsRefCursorImplicitFetch) setContext(parent context.Context) {
	for _, rows := range r.implicitRows {
		if rows != nil && rows.ttcRows != nil {
			rows.ttcRows.setContext(parent)
		}
	}
}

// attachStatement records the Statement that produced these implicit results.
func (r *ttcRowsRefCursorImplicitFetch) attachStatement(stmt *Statement) {
	if !r.closed {
		r.statement = stmt
	}
}

// takeStatementOwnership transfers a direct PL/SQL query's Statement to Rows.
func (r *ttcRowsRefCursorImplicitFetch) takeStatementOwnership(stmt *Statement) bool {
	if r.closed || r.statement != stmt {
		return false
	}
	r.ownsStatement = true
	return true
}

// closeFromStatement closes these results without recursively closing their
// Statement, which is already performing its own cleanup.
func (r *ttcRowsRefCursorImplicitFetch) closeFromStatement(stmt *Statement) error {
	if r.statement != stmt {
		return nil
	}
	r.statement = nil
	r.ownsStatement = false
	return r.Close()
}

// Close closes every implicit result set, retaining the first close error for
// later calls. Closing this wrapper more than once is idempotent.
func (r *ttcRowsRefCursorImplicitFetch) Close() error {
	if r.closed {
		return r.closeErr
	}
	r.closed = true
	stmt, ownsStatement := r.statement, r.ownsStatement
	r.statement = nil
	r.ownsStatement = false
	for _, rows := range r.implicitRows {
		if rows != nil {
			if err := rows.Close(); err != nil && r.closeErr == nil {
				r.closeErr = err
			}
		}
	}
	if stmt != nil {
		stmt.detachRows(r)
	}
	if ownsStatement && stmt != nil {
		if err := stmt.closeAfterRows(); err != nil && r.closeErr == nil {
			r.closeErr = err
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
