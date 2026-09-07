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

	"maps"
	"weak"

	internalCommon "github.com/oracle/go-oracledb/v26/internal/common"
	common "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/oracle/errors"
	"github.com/oracle/go-oracledb/v26/oracle/providers"
)

// ttiShelfUser declares a dependency on a TTC shelf.
// Implementors receive the shared TTC shelf to execute their actions.
type ttiShelfUser interface {
	// SetShelf injects the TTC shelf used by the implementor to perform operations.
	SetShelf(shelf *ttiShelf[common.MessageType])
}

// StmtCancellationFunction function that cancel the current statement execution
// by triggering the break/reset protocol
type StmtCancellationFunction func(ctx context.Context) error

// ttiShelf wraps common.Shelf with TTC-specific registries.
// In addition to the base Shelf, it maintains a codecFactory that selects
// encoders/decoders/OAC makers for the negotiated TTC protocol version.
type ttiShelf[T any] struct {
	*common.Shelf[T]
	codecFactory             codecFactory
	_providerRegistry        internalCommon.Registry[providers.Provider]
	_statements              map[*Statement]weak.Pointer[Statement]
	_currentTransaction      *transaction
	_cancelExecutionFunction StmtCancellationFunction
	_serverTimeZoneOffset    int16 // server time zone in seconds
	_eventService            *eventService
	_validatorRegistry       internalCommon.Registry[stateValidator]
}

// newShelf creates a new TTC shelf wrapping a fresh common.Shelf[T].
// TTC-specific registries (codecs and OAC makers) are initialized to nil and
// can be populated via RegisterCodecs and RegisterOacs.
func newShelf[T any]() *ttiShelf[T] {
	base := common.NewShelf[T]()
	return &ttiShelf[T]{
		Shelf:              base,
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
	s._statements[statement] = weak.Make(statement)
}

// RemoveStatement removes a statement from this shelf
// parameter:
//
//	statement the statement to be removed
func (s *ttiShelf[T]) RemoveStatement(statement *Statement) {
	delete(s._statements, statement)
}

// isInTransaction returns true if a transaction is in progress, otherwise false
func (s *ttiShelf[T]) isInTransaction() bool {
	return (s._currentTransaction != nil)
}

// registerTransaction registeres the current transaction
//
// Parameters:
//   - t: the transaction
func (s *ttiShelf[T]) registerTransaction(t *transaction) {
	s._currentTransaction = t
}

// unregisterTransaction unregisters the current transaction
func (s *ttiShelf[T]) unregisterTransaction() {
	s._currentTransaction = nil
}

// getTransaction returns the current transaction
func (s *ttiShelf[T]) getTransaction() *transaction {
	return s._currentTransaction
}

// registerCancelFunction registers the cancel function used to cancel current
// statement execution
//
// Parameters:
//   - cancelExecutionFunction the cancel function
func (s *ttiShelf[T]) registerCancelExecution(cancelExecutionFunction StmtCancellationFunction) {
	s._cancelExecutionFunction = cancelExecutionFunction
}

func (s *ttiShelf[T]) cancelExecution(ctx context.Context) error {
	return s._cancelExecutionFunction(ctx)
}

func (s *ttiShelf[T]) registerServerTimeZoneOffset(serverTimeZoneOffset int16) {
	s._serverTimeZoneOffset = serverTimeZoneOffset
}

func (s *ttiShelf[T]) getServerTimeZoneOffset() int16 {
	return s._serverTimeZoneOffset
}

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
