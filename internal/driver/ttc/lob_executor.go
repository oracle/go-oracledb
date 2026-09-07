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
	"errors"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// completedLobResponseError marks a failed LOB operation whose terminal TTC
// response was consumed. Unlike ambiguous push, flush, and pull failures, it
// proves the exchange was fully delimited and does not by itself make the TTC
// stream unsafe for reuse.
type completedLobResponseError struct {
	err error
}

// Error implements error.
func (e *completedLobResponseError) Error() string { return e.err.Error() }

// ErrorCode preserves the public SQLError contract while retaining the private
// synchronization marker. It forwards the first coded error in the chain.
func (e *completedLobResponseError) ErrorCode() string {
	var sqlErr oracleErrors.SQLError
	if errors.As(e.err, &sqlErr) {
		return sqlErr.ErrorCode()
	}
	return string(oracleErrors.LobExecError)
}

// Unwrap exposes the original Oracle error to errors.Is/errors.As callers.
func (e *completedLobResponseError) Unwrap() error { return e.err }

// isCompletedLobResponseError reports whether err contains proof that the
// failed LOB operation's terminal TTC response was consumed.
func isCompletedLobResponseError(err error) bool {
	var completed *completedLobResponseError
	return errors.As(err, &completed)
}

// operationError wraps a failed LOB transport stage. If the operation context
// was canceled, it first performs break/reset and consumes the terminal TTIOER.
// A successful recovery is marked as completed so callers may keep the
// physical connection; failed recovery remains unmarked and requires discard.
func (e *lobExecutor) operationError(ctx context.Context, err error, stage string) error {
	wrapped := common.NewOracleError(oracleErrors.LobExecError, err, stage)
	if ctx.Err() == nil {
		return wrapped
	}
	streamer, ok := e.shelf.GetMessageStreamer().(MessageStreamerInterface)
	if !ok {
		return wrapped
	}
	terminal, restoreErr := restoreTTCStreamAfterCancellation(ctx, streamer)
	if restoreErr != nil {
		return common.NewOracleError(oracleErrors.LobExecError, errors.Join(err, restoreErr), stage)
	}
	if terminal != nil {
		if oer, ok := terminal.(tTIOerIface); ok {
			if serverErr := oer.getError(); serverErr != nil {
				wrapped = common.NewOracleError(oracleErrors.LobExecError, errors.Join(ctx.Err(), serverErr), stage)
			}
		}
	}
	return &completedLobResponseError{err: wrapped}
}

// lobExecutor provides the base TTC RPC implementation that every LOB executor builds
// on. It owns the message plumbing and shared helpers so higher-level BLOB/CLOB
// code can reuse the core RPC flow without re-implementing or validation logic.
type lobExecutor struct {
	// shelf provides access to cached TTC message factories and streamers scoped to a connection.
	shelf *driverCommon.Shelf[driverCommon.MessageType]
	// openOperation stores the lob operation code used for open requests.
	openOperation lobOperationCode
	// closeOperation stores the lob operation code used for close requests.
	closeOperation lobOperationCode
	// isOpenOperation stores the lob operation code used for is-open probes.
	isOpenOperation lobOperationCode
}

// newLobExecutor allocates a lobExecutor pre-configured with the default LOB operation codes
// (open, close, and is-open) and the required message shelf. Callers typically embed the returned
// instance inside type-specific executors such as clobExecutor and rely on the TTC defaults unless
// a specialised verb is required.
//
// Parameters:
//   - shelf: message shelf shared by the executor.
//
// Returns:
//   - *lobExecutor: executor instance wired with the shelf and default operation codes.
func newLobExecutor(shelf *driverCommon.Shelf[driverCommon.MessageType]) *lobExecutor {
	return &lobExecutor{
		shelf:           shelf,
		openOperation:   kplobOpen,
		closeOperation:  kplobClose,
		isOpenOperation: kplobIsOpen,
	}
}

// read constructs and dispatches an OLOBOPS request using the kplobRead function code.
//
// Parameters:
//   - lobLocator: locator for the server-side LOB to read from.
//   - offset: position within the LOB where the read should begin.
//   - numBytes: maximum number of bytes to transfer from the LOB.
//   - outBuffer: destination buffer that receives the LOB data.
//
// Returns:
//   - driverCommon.UB8: number of bytes written into outBuffer for the request.
//   - driverCommon.UB8: server-reported lobAmt from TTIRPA.
//
// Errors:
//   - LobExecError("read") wrapping TTC, protocol, or network failures raised while exchanging
//     TTIRPA/TTILOBD messages to satisfy the read.
func (e *lobExecutor) read(ctx context.Context, lobLocator *locator, numBytes driverCommon.UB8, outBuffer driverCommon.B1Array) (driverCommon.UB8, driverCommon.UB8, error) {
	common.Odl.Debug("lobExecutor.read: begin", "offset", lobLocator.offset, "numBytes", numBytes)
	def := newLobDefinitionForReadOperation(lobLocator, numBytes)

	if err := e.executeRead(ctx, def, outBuffer); err != nil {
		common.Odl.Error("lobExecutor.read: executeRead failed", "error", err)
		return 0, 0, common.NewOracleError(oracleErrors.LobExecError, err, "read")
	}

	common.Odl.Debug("lobExecutor.read: completed", "bytesRead", def.bytesTransferred, "lobAmt", def.lobAmt)
	return def.bytesTransferred, def.lobAmt, nil
}

// write builds and sends an OLOBOPS request using the kplobWrite function code along with the
// accompanying TTILOBD payload carrying client data.
//
// Parameters:
//   - lobLocator: locator that identifies the destination LOB.
//   - offset: position within the LOB where the write should begin.
//   - inBuffer: client buffer that supplies the bytes to be written.
//   - numBytes: number of bytes requested for transfer.
//
// Returns:
//   - driverCommon.UB8: number of bytes written into the destination LOB.
//
// Errors:
//   - Returns buffer validation failures or any TTC/protocol/network errors encountered while
//     exchanging TTIRPA and TTILOBD messages required to complete the write.
//   - LobExecError("write") wraps TTC/protocol/network failures raised while executing OLOBOPS.
func (e *lobExecutor) write(ctx context.Context, lobLocator *locator, inBuffer driverCommon.B1Array,
	numBytes driverCommon.UB8) (driverCommon.UB8, error) {
	common.Odl.Debug("lobExecutor.write: begin", "offset", lobLocator.offset, "numBytes", numBytes)
	// validateLobOperation ensures mutating operations honor locator capabilities.
	if err := validateLobOperation(lobLocator, kplobWrite); err != nil {
		common.Odl.Error("lobExecutor.write: validate failed", "error", err)
		return 0, err
	}

	def := newLobDefinitionForWriteOperation(lobLocator, numBytes)

	if err := e.executeWrite(ctx, def, inBuffer); err != nil {
		common.Odl.Error("lobExecutor.write: executeWrite failed", "error", err)
		return 0, common.NewOracleError(oracleErrors.LobExecError, err, "write")
	}

	common.Odl.Debug("lobExecutor.write: completed", "bytesWritten", def.lobAmt)
	return def.lobAmt, nil
}

// GetLength builds and sends an OLOBOPS request using the kplobGetLength function code and
// expects a TTIRPA response conveying the result.
//
// Parameters:
//   - lobLocator: locator identifying the LOB whose length should be retrieved.
//
// Returns:
//   - driverCommon.UB8: length of the specified LOB as reported by the server.
//
// Errors:
//   - LobExecError("get-length") wraps TTC/protocol/network failures raised while exchanging
//     TTIRPA messages required to obtain the length.
func (e *lobExecutor) getLength(ctx context.Context, lobLocator *locator) (driverCommon.UB8, error) {
	common.Odl.Debug("lobExecutor.getLength: begin")
	def := newLobDefinitionForGetLengthOperation(lobLocator)

	if err := e.execute(ctx, def); err != nil {
		common.Odl.Error("lobExecutor.getLength: execute failed", "error", err)
		return 0, common.NewOracleError(oracleErrors.LobExecError, err, "get-length")
	}

	common.Odl.Debug("lobExecutor.getLength: completed", "length", def.lobAmt)
	return def.lobAmt, nil
}

// GetChunkSize sends an OLOBOPS request with the kplobPageSize function code and expects a
// TTIRPA response carrying the server page size.
//
// Parameters:
//   - lobLocator: locator identifying the LOB for which the chunk size is requested.
//
// Returns:
//   - driverCommon.UB8: page size reported by the server for the specified LOB.
//
// Errors:
//   - LobExecError("get-chunk-size") wraps TTC/protocol/network failures raised while exchanging
//     TTIRPA messages required to obtain the chunk size.
func (e *lobExecutor) getChunkSize(ctx context.Context, lobLocator *locator) (driverCommon.UB8, error) {
	common.Odl.Debug("lobExecutor.getChunkSize: begin")
	def := newLobDefinitionForGetChunkSizeOperation(lobLocator)

	if err := e.execute(ctx, def); err != nil {
		common.Odl.Error("lobExecutor.getChunkSize: execute failed", "error", err)
		return 0, common.NewOracleError(oracleErrors.LobExecError, err, "get-chunk-size")
	}

	common.Odl.Debug("lobExecutor.getChunkSize: completed", "chunkSize", def.lobAmt)
	return def.lobAmt, nil
}

// Trim sends an OLOBOPS request using the kplobTrim function code and expects a TTIRPA response
// that reports the resulting size.
//
// Parameters:
//   - lobLocator: locator identifying the LOB to be truncated or extended.
//   - newLength: length the server should apply to the target LOB.
//
// Returns:
//   - driverCommon.UB8: resulting LOB length returned by the server.
//
// Errors:
//   - LobExecError("trim") wraps TTC/protocol/network failures raised while exchanging TTIRPA
//     messages required to perform the Trim.
func (e *lobExecutor) trim(ctx context.Context, lobLocator *locator, newLength driverCommon.UB8) (driverCommon.UB8, error) {
	common.Odl.Debug("lobExecutor.trim: begin", "newLength", newLength)
	if err := validateLobOperation(lobLocator, kplobTrim); err != nil {
		common.Odl.Error("lobExecutor.trim: validate failed", "error", err)
		return 0, err
	}
	def := newLobDefinitionForTrimOperation(lobLocator, newLength)

	if err := e.execute(ctx, def); err != nil {
		common.Odl.Error("lobExecutor.trim: execute failed", "error", err)
		return 0, common.NewOracleError(oracleErrors.LobExecError, err, "trim")
	}

	common.Odl.Debug("lobExecutor.trim: completed", "resultLength", def.lobAmt)
	return def.lobAmt, nil
}

// open transitions the supplied LOB or BFILE locator into an open state that matches the
// requested marshaling mode. The TTC verb defaults to kplobOpen for persistent LOBs; callers can
// adjust the operation stored on the executor when alternate TTC verbs (for example, BFILE open)
// are required.
//
// Parameters:
//   - ctx: execution context propagated to TTC calls.
//   - locator: locator identifying the LOB/BFILE whose open state should be established.
//   - mode: marshaling mode requested for the open (for example, read/write).
//
// Returns:
//   - bool: true when the locator was opened locally (temporary/abstract locator) and the open flag was toggled.
//     Persistent locators return false because the open state is held on the server.
//   - error: error describing validation or TTC failures encountered while attempting the open.
//
// Behaviour:
//   - Temporary or abstract locators are opened locally by mutating the flag bytes—no server call.
//   - Persistent locators trigger an OLOBOPS request so the database opens the LOB/BFILE.
//   - Quasi locators (value-based) are treated as no-ops because they cannot be opened server-side.
//
// Errors:
//   - InvalidLOBBuffer when the locator is empty or already open in a context that disallows it.
//   - LobExecError("open") wrapping TTC/protocol/network failures raised while executing OLOBOPS.

func (e *lobExecutor) open(ctx context.Context, lobLocator *locator, mode lobOpenMode) (bool, error) {
	didOpen := false
	common.Odl.Debug("lobExecutor.open: begin", "mode", mode, "operation", e.openOperation)
	if lobLocator.isQuasiLocator() {
		return didOpen, nil
	}

	// if Lob is temporary then open is a local operation only.
	// the open state is merely held in the locator. If LOB
	// is not open this is an error (ORA22293).
	if lobLocator.isTemporaryLocator() || lobLocator.isAbstractLocator() {
		// if LOB is already open then this is an error
		if lobLocator.isOpenLocator() {
			err := common.NewOracleError(oracleErrors.LobOpen, nil)
			common.Odl.Error("lobExecutor.open: locator already open", "error", err)
			return didOpen, err
		}
		// set open state
		lobLocator.setOpenState()
		// set mode
		lobLocator.setAccessMode(mode)

		didOpen = true
		return didOpen, nil
	}

	// Lob is not temporary -- must send message to server
	// initialize lobdefinition structure
	def := newLobDefinitionForOpenOperation(lobLocator, mode, e.openOperation)

	if err := e.execute(ctx, def); err != nil {
		common.Odl.Error("lobExecutor.open: execute failed", "error", err)
		return didOpen, common.NewOracleError(oracleErrors.LobExecError, err, "open")
	}

	// retrieve open status
	if def.lobAmt != 0 {
		didOpen = true
	}

	common.Odl.Debug("lobExecutor.open: completed", "serverReportedOpen", didOpen)
	return didOpen, nil
}

// close resets the open flag for the locator and optionally issues a server side
// close request if the locator represents a persistent LOB/BFILE.
//
// Parameters:
//   - ctx: execution context propagated to TTC calls.
//   - locator: locator that identifies the LOB or BFILE whose open state should be cleared.
//
// Errors:
//   - InvalidLOBBuffer when the locator is already closed for local-only operations.
//   - LobExecError("close") wrapping TTC/protocol/network failures raised while executing OLOBOPS.
func (e *lobExecutor) close(ctx context.Context, lobLocator *locator) error {
	common.Odl.Debug("lobExecutor.close: begin", "operation", e.closeOperation)
	if lobLocator.isQuasiLocator() {
		return nil
	}

	// if Lob is temporary then close is a local operation only.
	// LOB must be open -- if not this is an error. Otherwise,
	// the open state is merely set to closed in the locator.
	if lobLocator.isTemporaryLocator() || lobLocator.isAbstractLocator() {
		// if LOB is not open then this is an error
		if !lobLocator.isOpenLocator() {
			err := common.NewOracleError(oracleErrors.LobClosed, nil)
			common.Odl.Error("lobExecutor.close: locator already closed", "error", err)
			return err
		}
		// set open state off
		lobLocator.clearAccessState()
		return nil
	}

	// Lob is not temporary -- must send message to server
	// initialize lobdef structure
	def := newLobDefinitionForCloseOperation(lobLocator, e.closeOperation)

	if err := e.execute(ctx, def); err != nil {
		common.Odl.Error("lobExecutor.close: execute failed", "error", err)
		return common.NewOracleError(oracleErrors.LobExecError, err, "close")
	}

	common.Odl.Debug("lobExecutor.close: completed")
	return nil
}

// IsOpen builds and sends the OLOBOPS message to test the open state of a LOB or
// BFILE. When the locator references a persistent LOB/BFILE, it expects the server
// to return a TTIRPA response conveying the open state.
//
// Parameters:
//   - ctx: execution context propagated to TTC calls.
//   - locator: locator referencing the LOB whose open state should be queried.
//
// Errors:
//   - LobExecError("is-open") wrapping TTC/protocol/network failures raised while executing OLOBOPS.
func (e *lobExecutor) isOpen(ctx context.Context, lobLocator *locator) (bool, error) {
	common.Odl.Debug("lobExecutor.isOpen: begin", "operation", e.isOpenOperation)
	if lobLocator.isQuasiLocator() {
		return false, nil
	}

	// if Lob is temporary then open state is in Lob Locator
	// and thus no trip to the server is required. If not
	// temporary Lob then must send OLOBOPS isOpen message.
	if lobLocator.isTemporaryLocator() || lobLocator.isAbstractLocator() {
		// see if temporary LOB is open
		return lobLocator.isOpenLocator(), nil
	}

	// Lob is persistent so must send message to server
	def := newLobDefinitionForIsOpenOperation(lobLocator, e.isOpenOperation)

	if err := e.execute(ctx, def); err != nil {
		common.Odl.Error("lobExecutor.isOpen: execute failed", "error", err)
		return false, common.NewOracleError(oracleErrors.LobExecError, err, "is-open")
	}

	common.Odl.Debug("lobExecutor.isOpen: completed", "isOpen", def.lobNull)
	return def.lobNull, nil
}

// validateLobOperation enforces locator capability checks for mutating LOB operations.
// It intentionally ignores read-only operations because they do not change server state.
//
// Restrictions (applied only when operation is kplobWrite or kplobTrim):
//   - Value-based locators (quasi locators) are inherently read-only and cannot participate in
//     mutating operations such as write or trim.
//   - Read-only locators similarly reject mutating operations.
//
// Errors:
//   - Returns an InvalidLOBBuffer error with a reason keyword aligned to the caller in case of
//     unsupported operations.
func validateLobOperation(lobLocator *locator, operation lobOperationCode) error {
	operationName := operation.String()

	if !operation.IsValid() {
		err := common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, operationName, "lob", "unsupported operation")
		common.Odl.Error("validateLobOperation: unsupported operation", "operation", operation, "error", err)
		return err
	}

	if operation != kplobWrite && operation != kplobTrim {
		return nil
	}

	if lobLocator.isValueBasedLocator() {
		err := common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, operationName, "lob", "value-based locator not supported")
		common.Odl.Error("validateLobOperation: value-based locator", "operation", operation, "error", err)
		return err
	}

	if lobLocator.isReadOnlyLocator() {
		err := common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, operationName, "lob", "locator is read-only")
		common.Odl.Error("validateLobOperation: read-only locator", "operation", operation, "error", err)
		return err
	}

	return nil
}

// ==============================================================================================================
// ==============================================================================================================

// execute dispatches a TTC OLOBOPS request using the generic execution path shared by metadata
// operations. It registers only the TTIRPA unmarshalling callback, pushes the request, flushes the
// streamer, and drains TTC responses until completion or failure.
func (e *lobExecutor) execute(ctx context.Context, lobDefinition *lobDefinition) error {
	common.Odl.Debug("lobExecutor.execute: start", "operation", lobDefinition.operation, "definition", lobDefinition)

	stmr, _ := e.shelf.GetMessageStreamer().(MessageStreamerInterface)

	e._registerLobRPACallback(lobDefinition)
	common.Odl.Debug("lobExecutor.execute: registered RPA callback")
	defer stmr.UnRegisterPreUnmarshallCallback(TTIRPA)

	if err := e._pushLobRequest(ctx, lobDefinition); err != nil {
		common.Odl.Error("lobExecutor.execute: push lob request failed", "error", err)
		return e.operationError(ctx, err, "push")
	}
	common.Odl.Debug("lobExecutor.execute: pushed lob request")

	if err := stmr.Flush(ctx); err != nil {
		common.Odl.Error("lobExecutor.execute: Flush failed", "error", err)
		return e.operationError(ctx, err, "flush")
	}
	common.Odl.Debug("lobExecutor.execute: flush completed")

	return e._consumeLobResponses(ctx)
}

// executeRead dispatches a TTC OLOBOPS request for read-style operations, installs TTILOBD
// callbacks to stream payload bytes into the caller-provided buffer, and coordinates TTIRPA
// responses to populate the supplied lobDefinition.
//
// The method validates the mandatory collaborators, wires unmarshalling callbacks specific to read
// verbs, pushes the request, flushes the streamer, and then drains TTC responses until completion
// or failure.
func (e *lobExecutor) executeRead(ctx context.Context, lobDefinition *lobDefinition, buffer driverCommon.B1Array) error {
	common.Odl.Debug("lobExecutor.executeRead: start", "operation", lobDefinition.operation, "definition", lobDefinition)

	stmr, _ := e.shelf.GetMessageStreamer().(MessageStreamerInterface)

	// Read operations stream payload bytes from TTILOBD frames into the caller-provided buffer.
	// Register the callbacks only when the request expects those frames to avoid unnecessary handlers.
	if err := e._registerLobdCallback(buffer, lobDefinition); err != nil {
		common.Odl.Error("lobExecutor.executeRead: register LOBD callback failed", "error", err)
		return err
	}
	common.Odl.Debug("lobExecutor.executeRead: registered LOBD callbacks")
	defer stmr.UnRegisterPreUnmarshallCallback(TTILOBD)
	defer stmr.UnRegisterPostUnmarshallCallback(TTILOBD)

	e._registerLobRPACallback(lobDefinition)
	common.Odl.Debug("lobExecutor.executeRead: registered RPA callback")
	defer stmr.UnRegisterPreUnmarshallCallback(TTIRPA)

	if err := e._pushLobRequest(ctx, lobDefinition); err != nil {
		common.Odl.Error("lobExecutor.executeRead: push lob request failed", "error", err)
		return e.operationError(ctx, err, "push")
	}
	common.Odl.Debug("lobExecutor.executeRead: pushed lob request")

	if err := stmr.Flush(ctx); err != nil {
		common.Odl.Error("lobExecutor.executeRead: Flush failed", "error", err)
		return e.operationError(ctx, err, "flush")
	}
	common.Odl.Debug("lobExecutor.executeRead: flush completed")

	return e._consumeLobResponses(ctx)
}

// executeWrite dispatches a TTC OLOBOPS request for write-style operations, coordinates TTIRPA
// responses to populate the supplied lobDefinition, and pushes the client payload via TTILOBD.
//
// The method validates the mandatory collaborators, registers the TTIRPA callback, pushes the
// request followed by the write payload, flushes the streamer, and then drains TTC responses until
// completion or failure.
func (e *lobExecutor) executeWrite(ctx context.Context, lobDefinition *lobDefinition, buffer driverCommon.B1Array) error {
	common.Odl.Debug("lobExecutor.executeWrite: start", "operation", lobDefinition.operation, "definition", lobDefinition)
	common.Odl.Debug("lobExecutor.executeWrite: request", "operation", lobDefinition.operation,
		"locatorOffset", lobDefinition.sourceLocator.offsetForLog(),
		"lobAmount", lobDefinition.lobAmt, "sendLobAmount", lobDefinition.sendLobAmt,
		"sourceLocatorLength", lobDefinition.sourceLocator.lengthForLog(),
		"destinationLocatorLength", lobDefinition.destinationLocator.lengthForLog(),
		"payloadLength", len(buffer))
	stmr, _ := e.shelf.GetMessageStreamer().(MessageStreamerInterface)

	e._registerLobRPACallback(lobDefinition)
	common.Odl.Debug("lobExecutor.executeWrite: registered RPA callback")
	defer stmr.UnRegisterPreUnmarshallCallback(TTIRPA)

	if err := e._pushLobRequest(ctx, lobDefinition); err != nil {
		common.Odl.Error("lobExecutor.executeWrite: push lob request failed", "error", err)
		return e.operationError(ctx, err, "push")
	}
	common.Odl.Debug("lobExecutor.executeWrite: pushed lob request")

	// Write operations send client bytes inside a TTILOBD payload flushed immediately after the request.
	// Non-write verbs (read/metadata) bypass this path because no client data needs to be transmitted.
	if err := e._pushWritePayload(ctx, buffer); err != nil {
		common.Odl.Error("lobExecutor.executeWrite: push write payload failed", "error", err)
		return e.operationError(ctx, err, "push-data")
	}
	common.Odl.Debug("lobExecutor.executeWrite: pushed write payload", "payloadLength", len(buffer))

	if err := stmr.Flush(ctx); err != nil {
		common.Odl.Error("lobExecutor.executeWrite: Flush failed", "error", err)
		return e.operationError(ctx, err, "flush")
	}
	common.Odl.Debug("lobExecutor.executeWrite: flush completed")

	return e._consumeLobResponses(ctx)
}

// _registerLobdCallback wires the TTILOBD unmarshalling callback for one read
// operation and ensures its output buffer is available. The byte count is
// stored on def so concurrent executor instances cannot share operation state.
func (e *lobExecutor) _registerLobdCallback(buffer driverCommon.B1Array, def *lobDefinition) error {
	stmr, _ := e.shelf.GetMessageStreamer().(MessageStreamerInterface)
	stmr.RegisterPreUnmarshallCallback(TTILOBD, func(*messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		msg, err := e.shelf.GetMessageFactory().(Factory).GetMessage(TTILOBD)
		if err != nil {
			common.Odl.Error("_registerLobdCallback: GetMessage(TTILOBD) failed", "error", err, "stage", "get-lobd")
			return nil, common.NewOracleError(oracleErrors.CallbackFactoryError, err, "registerLobdCallback failed")
		}
		lobd, _ := msg.(*tTIlobd)
		if def.bytesTransferred > driverCommon.UB8(len(buffer)) {
			return nil, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "read", "lob", "LOBD total exceeds buffer")
		}
		lobd.setBuffer(buffer[int(def.bytesTransferred):])
		return lobd, nil
	})

	stmr.RegisterPostUnmarshallCallback(TTILOBD, func(msg driverCommon.Message[driverCommon.MessageType], prevErr error) (bool, error) {
		lobd, _ := msg.(*tTIlobd)
		if prevErr != nil {
			return false, prevErr
		}
		read := lobd.getLastBytesRead()
		remaining := driverCommon.UB8(len(buffer)) - def.bytesTransferred
		if read > remaining {
			return false, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "read", "lob", "LOBD frame exceeds buffer")
		}
		def.bytesTransferred += read
		return true, nil
	})
	return nil
}

// _registerLobRPACallback sets up the TTIRPA pre-unmarshal callback so response payloads populate
// the provided lobDefinition instance.
func (e *lobExecutor) _registerLobRPACallback(def *lobDefinition) {
	stmr, _ := e.shelf.GetMessageStreamer().(MessageStreamerInterface)
	stmr.RegisterPreUnmarshallCallback(TTIRPA, func(*messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		msg, err := e.shelf.GetMessageFactory().(Factory).GetMessageForFunction(TTIRPA, oLobOps)
		if err != nil {
			common.Odl.Error("_registerLobRpaCallback: GetMessageForFunction(TTIRPA,oLobOps) failed", "error", err, "stage", "get-olobopsrpa")
			return nil, common.NewOracleError(oracleErrors.CallbackFactoryError, err, "registerLobRpaCallback failed")
		}
		lobRpa := msg.(*ttiLobRpa)
		lobRpa.SetDefinition(def)
		return lobRpa, nil
	})
}

// _pushLobRequest places the primary TTILOB message with the requested operation onto the stream.
func (e *lobExecutor) _pushLobRequest(ctx context.Context, def *lobDefinition) error {
	stmr, _ := e.shelf.GetMessageStreamer().(MessageStreamerInterface)
	msg := newTTIlob().(*tTIlob)
	msg.SetDefinition(def)
	if err := stmr.Push(ctx, msg); err != nil {
		common.Odl.Error("lobExecutor.pushLobRequest: Push failed", "error", err)
		return common.NewOracleError(oracleErrors.LobExecError, err, "push")
	}
	return nil
}

// _pushWritePayload sends the TTILOBD payload.
func (e *lobExecutor) _pushWritePayload(ctx context.Context, buffer driverCommon.B1Array) error {
	stmr, _ := e.shelf.GetMessageStreamer().(MessageStreamerInterface)
	lobMessage := newTTIlobd()
	lobd, _ := lobMessage.(*tTIlobd)

	lobd.setBuffer(buffer)
	if err := stmr.Push(ctx, lobMessage); err != nil {
		common.Odl.Error("lobExecutor.pushWritePayload: TTILOBD push failed", "error", err)
		return common.NewOracleError(oracleErrors.LobExecError, err, "push")
	}
	return nil
}

// _consumeLobResponses drains the message stream through the terminal status for an OLOBOPS call.
// TTIRPA carries returned LOB values but is not terminal; TTIOER and TTISTA are terminal status
// messages and must be fully consumed before the operation completes.
func (e *lobExecutor) _consumeLobResponses(ctx context.Context) error {
	stmr, _ := e.shelf.GetMessageStreamer().(MessageStreamerInterface)
	for {
		msg, err := stmr.Pull(ctx, TTILOBD, TTIRPA, TTIOER, TTISTA)
		if err != nil {
			common.Odl.Error("lobExecutor.consumeLobResponses: Pull failed", "error", err)
			return e.operationError(ctx, err, "pull")
		}

		switch msg.GetMsgCode() {
		case TTILOBD:
			common.Odl.Debug("lobExecutor.consumeLobResponses: TTILOBD received")
		case TTIRPA:
			common.Odl.Debug("lobExecutor.consumeLobResponses: TTIRPA received")
		case TTIOER:
			common.Odl.Debug("lobExecutor.consumeLobResponses: TTIOER received")
			oer := msg.(tTIOerIface)
			if lobErr := oer.getError(); lobErr != nil {
				common.Odl.Error("lobExecutor.consumeLobResponses: TTIOER error", "error", lobErr)
				return &completedLobResponseError{err: lobErr}
			}
			common.Odl.Debug("lobExecutor.consumeLobResponses: completed with TTIOER")
			return nil
		case TTISTA:
			// TODO: nothing to do with this information for now
			common.Odl.Debug("lobExecutor.consumeLobResponses: completed with TTISTA")
			return nil
		}
	}
}
