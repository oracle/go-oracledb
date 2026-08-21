package errors

// SQLError errors returned by the driver should implement this interface.
type SQLError interface {
	error
	// ErrorCode returns the error code (ORA-XXXXX)
	ErrorCode() string
}

// ErrorCode represents Oracle error codes.
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

	// EmptyTokenError indicates that the configured token value is empty after
	// resolution from AccessToken or TokenLocation.
	EmptyTokenError ErrorCode = "OGD-00201"
	// MissingTokenLocationError indicates that token-based authentication
	// requires a token location, but none was provided.
	MissingTokenLocationError ErrorCode = "OGD-00202"
	// ValueRetrievalError indicates that a required session or descriptor value
	// could not be retrieved or was empty.
	ValueRetrievalError ErrorCode = "OGD-00203"
	// InvalidPrivateKey indicates that the OCI private key is missing,
	// malformed, or not an RSA signing key.
	InvalidPrivateKey ErrorCode = "OGD-00204"
	// ExpiredToken indicates that the resolved access token has expired
	// according to its JWT exp claim.
	ExpiredToken ErrorCode = "OGD-00205"
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
