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
	"fmt"
	"reflect"
	"runtime"
	"sync"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// Factory is an interface for creating and retrieving TTC message implementations.
// It allows retrieval of messages and function implementations for different TTC types.
//
// Return values for all methods:
// - If an implementor is found and matches the request, returns (Message, nil).
// - If an implementor is found but the chosen one does not match the protocol (e.g., as fallback), returns (Message, nil).
// - If no implementor is found (type/code not registered) or if the allocation function raises an error, returns (nil, error).
type Factory interface {
	// GetMessage returns the best implementor of a given message type based on version/protocol.
	// If an implementor is found, returns (Message, nil). Otherwise, returns (nil, error).
	GetMessage(msgType driverCommon.MessageType) (driverCommon.Message[driverCommon.MessageType], error)

	// GetMessageForFunction returns the best implementor of a given message for a function code based on
	// version/protocol. If an implementor is found, returns (Message, nil). Otherwise, returns (nil, error).
	//
	// Messages of type TTIFUN, TTIONEWAYFN, TTISPF and TTIRPA can have different payload
	// depending on the function code. TTIFUN, TTIONEWAYFN are messages sent from the client to the server,
	// TTISPF is a piggyback function sent from the server to the client and TTIRPA is a response for a function.
	GetMessageForFunction(msgType driverCommon.MessageType, funcType driverCommon.FunctionType) (driverCommon.Message[driverCommon.MessageType], error)
}

// MessageCreationFunc defines the function signature for creating new messages.
// This function is used during registration to instantiate new message implementors.
type MessageCreationFunc func() driverCommon.Message[driverCommon.MessageType]

// RegisteredItem represents a registered message or function implementor.
type RegisteredItem struct {
	makeFunc              MessageCreationFunc
	minTTCProtocolVersion int8
}

func (r *RegisteredItem) String() string {
	return fmt.Sprintf("RegisteredItem {makeFunc: %v, minTTCProtocolVersion: %v}",
		runtime.FuncForPC(reflect.ValueOf(r.makeFunc).Pointer()).Name(), r.minTTCProtocolVersion)
}

// Registry is the central place for registering TTC messages and function implementations.
// In the TTC protocol, communication happens via messages.
// Some of these messages represent "functions (type: normal / piggy back/ oneway)"—a subset of messages that serve specialized protocol operations.
// Every function is itself a message type, and responses to functions are also always messages.
// This struct manages registration and lookup for all TTC messages and functions,
// tracking their supported protocol version ranges and association with function Response types.
type Registry[K comparable] struct {
	mu      sync.RWMutex
	entries map[K][]RegisteredItem
}

// functionRegistryKey this struct is used as a registry key for messages of the
// same messageType that have different payload depending on a function type
type functionRegistryKey struct {
	messageType  driverCommon.MessageType
	functionType driverCommon.FunctionType
}

// MessageRegistry Define the global registeries for message types and populated during _oSessionKeyInit().
var MessageRegistry = NewRegistry[driverCommon.MessageType]()

// FunctionRegistry Define the global registeries for messages that can have different
// payload depending on the function type
var FunctionRegistry = NewRegistry[functionRegistryKey]()

// NewRegistry creates and returns a new empty registry with the given key type.
func NewRegistry[K comparable]() *Registry[K] {
	return &Registry[K]{
		entries: make(map[K][]RegisteredItem),
	}
}

// Register adds a new implementation to the registry (for MessageType or FunctionType).
func (r *Registry[K]) Register(key K,
	minTTCProtocolVersion int8,
	f MessageCreationFunc) error {
	if validator, ok := any(key).(interface{ isValid() bool }); ok {
		if !validator.isValid() {
			common.Odl.Warn("Invalid key", "key", key)
			return common.NewOracleError(oracleErrors.InternalError, nil)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	item := RegisteredItem{f, minTTCProtocolVersion}
	// Check if a message has already been registered for that key, min and max version.
	foundIndex := -1
	for index, item := range r.entries[key] {
		if item.minTTCProtocolVersion == minTTCProtocolVersion {
			foundIndex = index
			break
		}
	}
	// if no message was found append new one otherwise replace it with the new one.
	if foundIndex == -1 {
		r.entries[key] = append(r.entries[key], item)
	} else {
		r.entries[key][foundIndex] = item
	}
	return nil
}

// getCandidates retrieves all registered candidates for a given message type.
func (r *Registry[K]) getCandidates(key K) []RegisteredItem {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if candidates, ok := r.entries[key]; ok {
		return append([]RegisteredItem(nil), candidates...)
	}
	// No candidates found for the given message type, so return nil.
	return nil
}

// SimpleFactory is a simple factory implementation that retrieves the best message or function implementor based on protocol version.
type SimpleFactory struct {
	ttcVersion   int8
	msgregistry  *Registry[driverCommon.MessageType]
	funcregistry *Registry[functionRegistryKey]
}

// NewMessageFactory creates a new factory that only considers implementor versions, returning the highest available version.
func NewMessageFactory() Factory {
	return NewMessageFactoryForProtocol(-1)
}

// NewMessageFactoryForProtocol creates a new factory for a given TTC protocol version and default registries.
func NewMessageFactoryForProtocol(protocolVersion int8) Factory {
	return &SimpleFactory{
		ttcVersion:   protocolVersion,
		msgregistry:  MessageRegistry,
		funcregistry: FunctionRegistry,
	}
}

// GetMessage retrieves the best message implementor for a given message type.
func (f *SimpleFactory) GetMessage(msgType driverCommon.MessageType) (driverCommon.Message[driverCommon.MessageType], error) {
	if !isValid(msgType) {
		common.Odl.Warn("Invalid message type", "message type", msgType)
		return nil, common.NewOracleError(oracleErrors.InternalError, nil)
	}

	common.Odl.Debug("New message requested", "type", msgType)

	candidates := f.msgregistry.getCandidates(msgType)
	if len(candidates) == 0 {
		common.Odl.Warn("Requested message type is not available", "message type", msgType)
		return nil, common.NewOracleError(oracleErrors.InternalError, nil)
	}

	bestCandidate := getBestImplementor(f.ttcVersion, candidates)
	if bestCandidate != nil {
		common.Odl.Debug("Message returned", "candidate", bestCandidate)
		return bestCandidate.makeFunc(), nil
	}
	common.Odl.Warn("No candidate message found", "message type", msgType)
	return nil, common.NewOracleError(oracleErrors.InternalError, nil)
}

// GetMessageForFunction retrieves the best message implementor for a given function type.
func (f *SimpleFactory) GetMessageForFunction(msgType driverCommon.MessageType, funcType driverCommon.FunctionType) (driverCommon.Message[driverCommon.MessageType], error) {
	common.Odl.Debug("New function requested", "code", funcType)

	key := functionRegistryKey{
		messageType:  msgType,
		functionType: funcType,
	}

	candidates := f.funcregistry.getCandidates(key)
	if len(candidates) == 0 {
		common.Odl.Warn("Requested function type is not available", "message type", msgType, "function type", funcType)
		return nil, common.NewOracleError(oracleErrors.InternalError, nil)

	}

	bestCandidate := getBestImplementor(f.ttcVersion, candidates)
	if bestCandidate != nil {
		common.Odl.Debug("Function returned", "candidate", bestCandidate)
		return bestCandidate.makeFunc(), nil
	}
	common.Odl.Warn("No candidate function found", "message type", msgType, "function type", funcType)
	return nil, common.NewOracleError(oracleErrors.InternalError, nil)
}

// getBestImplementor finds the best candidate implementation based on the TTC protocol version.
func getBestImplementor(protocolVersion int8, candidates []RegisteredItem) *RegisteredItem {
	var bestCandidate *RegisteredItem

	for _, candidate := range candidates {
		if (protocolVersion == -1 || (candidate.minTTCProtocolVersion <= protocolVersion)) &&
			(bestCandidate == nil || candidate.minTTCProtocolVersion > bestCandidate.minTTCProtocolVersion) {
			bestCandidate = &candidate
		}
	}

	return bestCandidate
}
