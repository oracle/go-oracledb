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
	"sync"

	"maps"
	"weak"

	internalCommon "github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/oracle/errors"
	"github.com/oracle/go-oracledb/v26/oracle/providers"
)

// ttiShelfUser declares a dependency on a TTC shelf.
// Implementors receive the shared TTC shelf to execute their actions.
type ttiShelfUser interface {
	// SetShelf injects the TTC shelf used by the implementor to perform operations.
	SetShelf(shelf *ttiShelf[driverCommon.MessageType])
}

// StmtCancellationFunction requests an out-of-band break/reset for the active
// statement. Implementations must honor ctx and must not acquire the session
// synchronizer, because cancellation has to interrupt its exchange.
type StmtCancellationFunction func(ctx context.Context) error

// ttiShelf wraps common.Shelf with state shared by every object using one
// physical Oracle session. In addition to codec and statement registries, it
// owns the connection-wide TTC operation mutex. That mutex is necessary because
// locator-backed Lob methods are invoked directly by application code, outside
// database/sql's driver-connection lock, but still share the same message
// stream with queries, cursor close, ping, commit, rollback, and logoff.
type ttiShelf[T any] struct {
	// Shelf provides common marshalling, streaming, localization, capabilities,
	// and connection configuration.
	*driverCommon.Shelf[T]
	// codecFactory selects codecs and OAC builders for the negotiated protocol.
	codecFactory      codecFactory
	_providerRegistry internalCommon.Registry[providers.Provider]
	// _statements tracks open statements weakly for connection shutdown.
	_statements map[*Statement]weak.Pointer[Statement]
	// _currentTransaction is non-nil while one transaction owns the session.
	_currentTransaction *transaction
	// _cancelExecutionFunction performs out-of-band break/reset without taking
	// the session synchronizer, allowing cancellation to interrupt its
	// current owner.
	_cancelExecutionFunction StmtCancellationFunction
	// _serverTimeZoneOffset is the negotiated server offset in seconds.
	_serverTimeZoneOffset int16 // server time zone in seconds
	// _eventService distributes streamer and connection invalidation events.
	_eventService      *eventService
	_validatorRegistry internalCommon.Registry[stateValidator]
	// synchronizer owns admission to the physical session's TTC stream. It is a
	// pointer because authentication copies ttiShelf values; every copy must
	// coordinate with the same physical session.
	synchronizer *sessionSynchronizer
	// cancellation owns cancellation recovery for one TTC exchange. It is shared
	// by shelf copies for the same physical session.
	cancellation *operationCancellation
	// lobState contains LOB-specific state whose lifetime is exactly the
	// physical Oracle session. It is shared by copied shelves during
	// authentication and is abandoned on session teardown.
	lobState *lobSessionState
	// stateMu protects statement and transaction registries which can now be
	// reached both through database/sql and locator lifecycle callbacks. It is a
	// pointer because authentication currently copies ttiShelf values; all
	// copies must guard the shared maps and pointers with the same mutex.
	stateMu *sync.RWMutex
}

// newShelf creates a new TTC shelf wrapping a fresh common.Shelf[T].
// TTC-specific registries (codecs and OAC makers) are initialized to nil and
// can be populated via RegisterCodecs and RegisterOacs.
func newShelf[T any]() *ttiShelf[T] {
	base := driverCommon.NewShelf[T]()
	lobState := newLobSessionState()
	return &ttiShelf[T]{
		Shelf:              base,
		synchronizer:       newSessionSynchronizer(),
		cancellation:       newOperationCancellation(),
		lobState:           lobState,
		stateMu:            &sync.RWMutex{},
		codecFactory:       nil,
		_statements:        make(map[*Statement]weak.Pointer[Statement]),
		_eventService:      newEventService(),
		_validatorRegistry: internalCommon.NewRegistry[stateValidator](),
	}
}

// RegisterCodecFactory registers a codecFactory for the negotiated TTC protocol version.
// Any previously registered factory is replaced. Returns the shelf to allow call chaining.
func (s *ttiShelf[T]) RegisterCodecFactory(factory codecFactory) *ttiShelf[T] {
	s.codecFactory = factory
	return s
}

// GetCodecFactory returns the registered codecFactory, or nil if none was registered.
func (s *ttiShelf[T]) GetCodecFactory() codecFactory {
	return s.codecFactory
}

// registerProviderRegistry stores the provider registry associated with this shelf.
//
// Parameters:
//   - providerRegistry: the provider registry to store on the shelf.
func (s *ttiShelf[T]) registerProviderRegistry(providerRegistry internalCommon.Registry[providers.Provider]) {
	s._providerRegistry = providerRegistry
}

// getProviderRegistry returns the provider registry stored on this shelf.
//
// Returns:
//   - the provider registry registered on the shelf, or nil if none was registered.
func (s *ttiShelf[T]) getProviderRegistry() internalCommon.Registry[providers.Provider] {
	return s._providerRegistry
}

// GetStatements gets all opened statements
//
//	 parameters:
//	   - drain : if true, open statement list is also drained out of the shelf
//		returns a slice of statements
func (s *ttiShelf[T]) GetStatements(drain bool) []*Statement {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	ss := make([]*Statement, len(s._statements))
	var i = 0
	for v := range maps.Values(s._statements) {
		if v.Value() != nil {
			ss[i] = v.Value()
			i++
		}
	}
	if drain {
		clear(s._statements)
	}
	return ss[:i]
}

// AddStatement adds a statement to this shelf
// parameter:
//
//	statement the statement to be added
func (s *ttiShelf[T]) AddStatement(statement *Statement) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s._statements[statement] = weak.Make(statement)
}

// RemoveStatement removes a statement from this shelf
// parameter:
//
//	statement the statement to be removed
func (s *ttiShelf[T]) RemoveStatement(statement *Statement) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	delete(s._statements, statement)
}

// isInTransaction returns true if a transaction is in progress, otherwise false
func (s *ttiShelf[T]) isInTransaction() bool {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s._currentTransaction != nil
}

// registerTransaction installs t only when no transaction is currently
// registered. It makes BeginTx's check-and-register transition atomic.
//
// Parameters:
//   - t: the transaction
//
// Returns:
//   - bool: true when t was registered.
func (s *ttiShelf[T]) registerTransaction(t *transaction) bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s._currentTransaction != nil {
		return false
	}
	s._currentTransaction = t
	return true
}

// unregisterTransaction clears the transaction slot only when t
// still owns it. It prevents a completed setup/terminal path from erasing a
// newer transaction registered after an intervening failure.
func (s *ttiShelf[T]) unregisterTransaction(t *transaction) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s._currentTransaction == t {
		s._currentTransaction = nil
	}
}

// getTransaction returns the current transaction
func (s *ttiShelf[T]) getTransaction() *transaction {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s._currentTransaction
}

// isCurrentTransaction reports whether t still owns the transaction slot.
func (s *ttiShelf[T]) isCurrentTransaction(t *transaction) bool {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s._currentTransaction == t
}

// registerCancelFunction registers the cancel function used to cancel current
// statement execution
//
// Parameters:
//   - cancelExecutionFunction the cancel function
func (s *ttiShelf[T]) registerCancelExecution(cancelExecutionFunction StmtCancellationFunction) {
	s._cancelExecutionFunction = cancelExecutionFunction
}

// cancelExecution invokes the physical connection's registered out-of-band
// cancellation function. NewConnection registers the function before any
// statement can execute.
func (s *ttiShelf[T]) cancelExecution(ctx context.Context) error {
	return s._cancelExecutionFunction(ctx)
}

// registerServerTimeZoneOffset records the negotiated server offset in seconds
// for timestamp decoding by all statements and Rows on this session.
func (s *ttiShelf[T]) registerServerTimeZoneOffset(serverTimeZoneOffset int16) {
	s._serverTimeZoneOffset = serverTimeZoneOffset
}

// getServerTimeZoneOffset returns the negotiated server offset in seconds.
func (s *ttiShelf[T]) getServerTimeZoneOffset() int16 {
	return s._serverTimeZoneOffset
}

// getEventService returns the physical-session event distributor used for
// streamer-stale and connection-lifecycle notifications.
func (s *ttiShelf[T]) getEventService() *eventService {
	return s._eventService
}

// stateValidator reports whether a TTC component is still in a valid state.
// Validators are checked before an operation completes so stale or otherwise
// invalid protocol state can invalidate the operation.
type stateValidator interface {
	// isValid reports whether the associated TTC component is in a valid
	// state.
	isValid(context.Context) bool
}

// registerStateValidator adds a validator to the shelf's connection
// validation chain.
func (s *ttiShelf[T]) registerStateValidator(validator stateValidator) {
	s._validatorRegistry.Register(validator)
}

// checkCurrentState runs all validators registered on the shelf.
//
// Returns an internal error when any validator reports that the state is
// invalid; otherwise it returns nil.
func (s *ttiShelf[T]) checkCurrentState(ctx context.Context) error {
	for _, item := range s._validatorRegistry.GetAll() {
		if item != nil && !item.isValid(ctx) {
			return s.LocalizeError(internalCommon.NewOracleError(errors.InternalError, nil))
		}
	}
	return nil
}
