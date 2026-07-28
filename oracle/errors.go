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

package oracle

import "github.com/oracle/go-driver/driver/common"

// SQLError is implemented by errors returned by the Oracle driver.
type SQLError = common.SQLError

// ErrorCode represents Oracle driver error codes.
type ErrorCode = common.ErrorCode

const (
	InvalidCredential      = common.InvalidCredential
	NoDataFound            = common.NoDataFound
	ConnectionLost         = common.ConnectionLost
	AliasNotFound          = common.AliasNotFound
	NoListenerAvailable    = common.NoListenerAvailable
	ConnectionClosed       = common.ConnectionClosed
	ProtocolViolation      = common.ProtocolViolation
	LobOpen                = common.LobOpen
	LobClosed              = common.LobClosed
	ChannelReadFailed      = common.ChannelReadFailed
	ChannelWriteFailed     = common.ChannelWriteFailed
	UnknownHost            = common.UnknownHost
	LogonDenied            = common.LogonDenied
	ConnectTimeout         = common.ConnectTimeout
	ListenerFailed         = common.ListenerFailed
	NoDispatcherAvailable  = common.NoDispatcherAvailable
	ConnectFailed          = common.ConnectFailed
	InvalidServiceName     = common.InvalidServiceName
	NoAvailableHandler     = common.NoAvailableHandler
	UnknownInstance        = common.UnknownInstance
	ListenerRequestTimeout = common.ListenerRequestTimeout
	UnresolvedAlias        = common.UnresolvedAlias
	InvalidSID             = common.InvalidSID
	ForeignKeyViolation    = common.ForeignKeyViolation

	TableOrViewNotFound   = common.TableOrViewNotFound
	InsufficientPrivilege = common.InsufficientPrivilege
	InvalidTableName      = common.InvalidTableName
	MissingReadPrivilege  = common.MissingReadPrivilege

	ConnCloseTimeout                     = common.ConnCloseTimeout
	CtxTimeout                           = common.CtxTimeout
	UnknownSPFFunction                   = common.UnknownSPFFunction
	SPFNotFunction                       = common.SPFNotFunction
	InvalidSPFFunction                   = common.InvalidSPFFunction
	InvalidConnectionParameter           = common.InvalidConnectionParameter
	ConflictingConnectionParameterSource = common.ConflictingConnectionParameterSource

	NamingDSNInvalid     = common.NamingDSNInvalid
	NamingParseFailed    = common.NamingParseFailed
	NamingEzConnectError = common.NamingEzConnectError
	NamingContextError   = common.NamingContextError

	FailMarshal   = common.FailMarshal
	FailUnmarshal = common.FailUnmarshal

	InvalidLOBBuffer        = common.InvalidLOBBuffer
	LobExecError            = common.LobExecError
	UnsupportedCharacterSet = common.UnsupportedCharacterSet

	ConverterEmptyInput      = common.ConverterEmptyInput
	ConverterRange           = common.ConverterRange
	ConverterExpectedFormat  = common.ConverterExpectedFormat
	RowDecodeError           = common.RowDecodeError
	RowsCloseFailed          = common.RowsCloseFailed
	StatementExecutionFailed = common.StatementExecutionFailed

	StatementExecutionFactoryFailed = common.StatementExecutionFactoryFailed
	StatementCloseFailed            = common.StatementCloseFailed
	RunQueryError                   = common.RunQueryError
	RunExecError                    = common.RunExecError
	CallbackFactoryError            = common.CallbackFactoryError

	StatementParsingDanglingColon            = common.StatementParsingDanglingColon
	StatementParsingPlaceholdersArgsMismatch = common.StatementParsingPlaceholdersArgsMismatch
	StatementParsingInvalidArgCount          = common.StatementParsingInvalidArgCount
	StatementParsingNameNotFound             = common.StatementParsingNameNotFound
	StatementParsingDuplicateArg             = common.StatementParsingDuplicateArg
	StatementParsingInvalidOrdinal           = common.StatementParsingInvalidOrdinal
	StatementParsingMissingValue             = common.StatementParsingMissingValue
	InvalidIdentifier                        = common.InvalidIdentifier

	InternalError              = common.InternalError
	MissingLocalizationService = common.MissingLocalizationService
	CancelOperationError       = common.CancelOperationError
	UnsupportedFeature         = common.UnsupportedFeature
	InvalidSqlOutParameter     = common.InvalidSqlOutParameter
	InvalidFileConfig          = common.InvalidFileConfig

	NotInTransaction           = common.NotInTransaction
	IsolationLevelNotSupported = common.IsolationLevelNotSupported
	AlreadyInTransaction       = common.AlreadyInTransaction
	ErrorInTransaction         = common.ErrorInTransaction
	ConfigureTransactionError  = common.ConfigureTransactionError

	AuthenticatorError      = common.AuthenticatorError
	NegotiatorError         = common.NegotiatorError
	MarshalEngineError      = common.MarshalEngineError
	MarshalEngineFlushError = common.MarshalEngineFlushError
	StreamerReadError       = common.StreamerReadError
	StreamerWriteError      = common.StreamerWriteError
	DatabaseMountStateError = common.DatabaseMountStateError
	EmptyUsernameError      = common.EmptyUsernameError
	NoAuthenticatorError    = common.NoAuthenticatorError
	ServerTimeZoneError     = common.ServerTimeZoneError

	ProtocolViolationLimitExceeded = common.ProtocolViolationLimitExceeded
)
