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

package common

import (
	"errors"
	"fmt"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var messageLanguages = []language.Tag{
	language.English, // en: First language will be the default if match fails
	language.French,  // fr
}

var defaultPrinter = message.NewPrinter(language.English)

// localizationService is the concrete implementation of LocalizationService.
type localizationService struct {
	printer *message.Printer
}

// LocalizationService is a sealed interface that formats Oracle errors and attaches
// localized printers to errors created or wrapped by the driver.
type LocalizationService interface {
	// format renders a localized message for the given error code and arguments.
	format(code ErrorCode, args ...interface{}) string
	// LocalizeError attaches the service printer to an existing error chain.
	LocalizeError(err error) error
}

// NewLocalizationService returns a localization service for the provided user
// language tag. When the tag does not match a supported language, English is
// used as the fallback.
func NewLocalizationService(userLanguage language.Tag) LocalizationService {
	matcher := language.NewMatcher(messageLanguages)
	matchedLanguage, _, confidence := matcher.Match(userLanguage)
	Odl.Debug(fmt.Sprintf("Matched language %s with confidence %v", matchedLanguage, confidence))
	return &localizationService{
		printer: message.NewPrinter(matchedLanguage),
	}
}

// format renders a localized message for the given error code and arguments.
func (ms *localizationService) format(code ErrorCode, args ...interface{}) string {
	return ms.printer.Sprintf(string(code), args...)
}

// LocalizeError attaches the service printer to an existing error chain.
func (ms *localizationService) LocalizeError(err error) error {
	deepSetLocalizationService(err, ms)
	return err
}

// deepSetLocalizationService walks an error chain and attaches the provided
// localization service to each OracleError it finds.
func deepSetLocalizationService(err error, ms *localizationService) {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if oErr, ok := e.(OracleError); ok {
			oErr.setLocalizationService(ms)
		}
	}
}

// SQLError errors returned by the driver should implement this interface.
//
// Deprecated: use oracle.SQLError for application code.
type SQLError interface {
	// ErrorCode returns the error code (ORA-XXXXX)
	ErrorCode() string
	// Error returns the error message (implements the Go error interface)
	Error() string
	// Unwrap returns wrapped error if exists, otherwise returns nil
	Unwrap() error
}

type OracleError interface {
	// setLocalizationService stores the printer used to render the error text.
	setLocalizationService(LocalizationService)
}

// oracleError is the concrete SQLError implementation used for driver-side
// failures.
type oracleError struct {
	code                ErrorCode
	cause               error
	args                []any
	localizationService LocalizationService
}

// setLocalizationService stores the printer used to render the error text.
func (e *oracleError) setLocalizationService(localizationService LocalizationService) {
	e.localizationService = localizationService
}

// NewOracleError returns a new instance of the error with the given error code, cause and arguments.
//
// Parameters:
//   - code: the error code
//   - cause: the error that caused this error or nil if there is no cause
//   - args: the arguments to add to the error code
//
// Returns: a new instance of OracleError
func NewOracleError(code ErrorCode, cause error, args ...interface{}) SQLError {
	return &oracleError{
		code:  code,
		cause: cause,
		args:  args,
	}
}

// ErrorCode returns the error code
func (e *oracleError) ErrorCode() string {
	return string(e.code)
}

// Error returns an error message prefixed with error code.
func (e *oracleError) Error() string {
	msg := defaultPrinter.Sprintf(string(e.code), e.args...)
	if e.localizationService != nil {
		msg = e.localizationService.format(e.code, e.args...)
	}
	if len(msg) != 0 && msg != string(e.code) {
		if e.cause != nil {
			return fmt.Sprintf("%s - %s: %v", e.code, msg, e.cause.Error())
		}
		return fmt.Sprintf("%s - %s", e.code, msg)
	}
	return fmt.Sprintf("%s - Unknown error code, message not found", string(e.code))
}

// Unwrap unwraps the cause of this error
func (e *oracleError) Unwrap() error {
	return e.cause
}

// oerMessageError this struct should only be used by errors returned by the
// database on the OER message
type oerMessageError struct {
	code    string
	message string
}

// NewOERMessageError create a new oerOracleError. This function should only be
// used for errors returned by the database
func NewOERMessageError(code string, message string) SQLError {
	return &oerMessageError{
		code:    code,
		message: message,
	}
}

// ErrorCode returns the error code
func (e *oerMessageError) ErrorCode() string {
	return e.code
}

// Error returns an error message prefixed with error code.
func (e *oerMessageError) Error() string {
	return e.message
}

// Unwrap Database errors do not wrap any other error, returns nil
func (e *oerMessageError) Unwrap() error {
	return nil
}

// init loads the dictionary of error messages
func init() {
	initMessagesEn()
	initMessagesFr()
}

type CtxTimeoutCauseError interface {
	error
	GetSource() string
	GetValue() uint
	GetEmitterID() string
}
type _ctxTimeoutCauseError struct {
	source         string
	timeoutValueMS uint
	connectionID   string
}

func NewCtxTimeoutCauseError(timeoutSource string, timeoutValue uint, id string) CtxTimeoutCauseError {
	return &_ctxTimeoutCauseError{
		source:         timeoutSource,
		timeoutValueMS: timeoutValue,
		connectionID:   id,
	}
}

func (e _ctxTimeoutCauseError) GetEmitterID() string {
	return e.connectionID
}

func (e _ctxTimeoutCauseError) GetSource() string {
	return e.source
}
func (e _ctxTimeoutCauseError) GetValue() uint {
	return e.timeoutValueMS
}
func (e _ctxTimeoutCauseError) Error() string {
	return fmt.Sprintf("timeout of %d set by %s", e.timeoutValueMS, e.source)
}

// ErrorCode represents Oracle error codes.
//
// Deprecated: use oracle.ErrorCode for application code.
type ErrorCode string

// A constant must be declared for each error code used in the application. This
// constants can be used to check which error was received and react
// accordingly.
const (
	// ORA-XXXXX errors
	InvalidCredential      ErrorCode = "ORA-01017"
	NoDataFound            ErrorCode = "ORA-01403"
	ConnectionLost         ErrorCode = "ORA-03113"
	AliasNotFound          ErrorCode = "ORA-12154"
	NoListenerAvailable    ErrorCode = "ORA-12541"
	ConnectionClosed       ErrorCode = "ORA-17008"
	ProtocolViolation      ErrorCode = "ORA-17401"
	LobOpen                ErrorCode = "ORA-17445"
	LobClosed              ErrorCode = "ORA-17446"
	ChannelReadFailed      ErrorCode = "ORA-17800"
	ChannelWriteFailed     ErrorCode = "ORA-17831"
	UnknownHost            ErrorCode = "ORA-17868"
	LogonDenied            ErrorCode = "ORA-01017"
	ConnectTimeout         ErrorCode = "ORA-12170"
	ListenerFailed         ErrorCode = "ORA-12500"
	NoDispatcherAvailable  ErrorCode = "ORA-12516"
	ConnectFailed          ErrorCode = "ORA-17820"
	InvalidServiceName     ErrorCode = "ORA-12514"
	NoAvailableHandler     ErrorCode = "ORA-12520"
	UnknownInstance        ErrorCode = "ORA-12521"
	ListenerRequestTimeout ErrorCode = "ORA-12525"
	UnresolvedAlias        ErrorCode = "ORA-12261"
	InvalidSID             ErrorCode = "ORA-12505"
	ForeignKeyViolation    ErrorCode = "ORA-02291"

	// Table/Object access errors
	TableOrViewNotFound   ErrorCode = "ORA-00942"
	InsufficientPrivilege ErrorCode = "ORA-01031"
	InvalidTableName      ErrorCode = "ORA-00903"
	MissingReadPrivilege  ErrorCode = "ORA-41900"

	// Oracle GO Driver errors
	ConnCloseTimeout                     ErrorCode = "OGD-00001"
	CtxTimeout                           ErrorCode = "OGD-00002"
	UnknownSPFFunction                   ErrorCode = "OGD-00005"
	SPFNotFunction                       ErrorCode = "OGD-00006"
	InvalidSPFFunction                   ErrorCode = "OGD-00007"
	InvalidConnectionParameter           ErrorCode = "OGD-00008"
	ConflictingConnectionParameterSource ErrorCode = "OGD-00009"

	// Naming / Data Source Name parsing errors (driver-facing)
	// Raised by driver/network/naming package for Data Source Name / descriptor parsing and validation.
	NamingDSNInvalid     ErrorCode = "OGD-00100"
	NamingParseFailed    ErrorCode = "OGD-00101"
	NamingEzConnectError ErrorCode = "OGD-00102"
	NamingContextError   ErrorCode = "OGD-00103"

	// Message marshal Error

	// FailMarshal Message marshal Error
	FailMarshal ErrorCode = "OGD-00011"
	// FailUnmarshal Message Unmarshal Error
	FailUnmarshal ErrorCode = "OGD-00012"

	// InvalidLOBBuffer Invalid buffer configuration for LOB operations
	InvalidLOBBuffer ErrorCode = "OGD-00013"
	// LobExecError represents failures while performing LOB operations using OLOBOPS flows.
	LobExecError ErrorCode = "OGD-00014"
	// UnsupportedCharacterSet surfaces when LOB operations encounter a character set that
	// the Go driver does not yet support.
	UnsupportedCharacterSet ErrorCode = "OGD-00015"

	// Converter Errors
	// Empty input Error
	ConverterEmptyInput ErrorCode = "OGD-00021"
	// Structured converter expectation codes
	// Range with two placeholders for min/max, plus converter type and operation
	ConverterRange ErrorCode = "OGD-00022"
	// Expected format/type/value with a single expected placeholder, plus converter type and operation
	ConverterExpectedFormat ErrorCode = "OGD-00023"
	// Row decoding/scanning errors
	RowDecodeError  ErrorCode = "OGD-00024"
	RowsCloseFailed ErrorCode = "OGD-00025"

	// Statement execution preparation/validation failures
	StatementExecutionFailed ErrorCode = "OGD-00050"
	// Statement execution factory failures (invalid executor type for SQL kind/op)
	StatementExecutionFactoryFailed ErrorCode = "OGD-00051"
	StatementCloseFailed            ErrorCode = "OGD-00052"

	// runQuery errors
	RunQueryError ErrorCode = "OGD-00053"

	// runExec errors
	RunExecError ErrorCode = "OGD-00054"

	// Callback/factory errors
	CallbackFactoryError ErrorCode = "OGD-00055"

	// Specific parsing failures
	StatementParsingDanglingColon            ErrorCode = "OGD-00155"
	StatementParsingPlaceholdersArgsMismatch ErrorCode = "OGD-00156"
	StatementParsingInvalidArgCount          ErrorCode = "OGD-00157"
	StatementParsingNameNotFound             ErrorCode = "OGD-00158"
	StatementParsingDuplicateArg             ErrorCode = "OGD-00159"
	StatementParsingInvalidOrdinal           ErrorCode = "OGD-00160"
	StatementParsingMissingValue             ErrorCode = "OGD-00161"
	// InvalidIdentifier indicates a provided SQL identifier is invalid.
	InvalidIdentifier ErrorCode = "OGD-00162"
	// InvalidGTRIDValue indicates a provided global transaction identifier is empty
	// or exceeds the server-supported size limit.
	InvalidGTRIDValue ErrorCode = "OGD-00163"

	// Driver Internal Error
	InternalError ErrorCode = "OGD-00062"
	// MissingLocalizationService indicates the shelf was used before a localization
	// service was registered.
	MissingLocalizationService ErrorCode = "OGD-00067"

	// CancelOperationError error occurred while requesting the server to cancel
	// the current operation
	CancelOperationError ErrorCode = "OGD-00063"

	// UnsupportedFeature indicates a requested driver capability or API is not supported yet.
	UnsupportedFeature ErrorCode = "OGD-00064"
	// InvalidSqlOutParameter indicates sql.Out destination validation failed.
	InvalidSqlOutParameter ErrorCode = "OGD-00065"
	// InvalidFileConfig Can't load configuration from file.
	InvalidFileConfig ErrorCode = "OGD-00066"

	// TRANSACTION PROCESSING

	// Not in transaction error
	NotInTransaction ErrorCode = "OGD-00080"
	// Isolation level not supported error
	IsolationLevelNotSupported ErrorCode = "OGD-00081"
	// Already in a transaction error
	AlreadyInTransaction ErrorCode = "OGD-00082"
	// Error in Transaction operation
	ErrorInTransaction ErrorCode = "OGD-00083"
	// Error when creating a transaction, ALTER SESSION to set isolation level
	ConfigureTransactionError ErrorCode = "OGD-00084"

	AuthenticatorError      ErrorCode = "OGD-00091"
	NegotiatorError         ErrorCode = "OGD-00092"
	MarshalEngineError      ErrorCode = "OGD-00093"
	MarshalEngineFlushError ErrorCode = "OGD-00094"
	StreamerReadError       ErrorCode = "OGD-00095"
	StreamerWriteError      ErrorCode = "OGD-00096"
	DatabaseMountStateError ErrorCode = "OGD-00097"
	EmptyUsernameError      ErrorCode = "OGD-00098"
	NoAuthenticatorError    ErrorCode = "OGD-00099"
	ServerTimeZoneError     ErrorCode = "OGD-00110"

	ProtocolViolationLimitExceeded ErrorCode = "OGD-00200"
)

// OracleRefuseErrorCodes maps ORA error numbers to the driver error
// identifiers used with NewOracleError.
var OracleRefuseErrorCodes = map[string]ErrorCode{
	"12500": ListenerFailed,
	"12505": InvalidSID,
	"12514": InvalidServiceName,
	"12516": NoDispatcherAvailable,
	"12520": NoAvailableHandler,
	"12521": UnknownInstance,
	"12525": ListenerRequestTimeout,
}
