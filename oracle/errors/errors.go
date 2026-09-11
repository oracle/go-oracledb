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
	NamingDSNInvalid       ErrorCode = "OGD-00100"
	NamingParseFailed      ErrorCode = "OGD-00101"
	NamingEzConnectError   ErrorCode = "OGD-00102"
	NamingContextError     ErrorCode = "OGD-00103"
	NamingParsePosition    ErrorCode = "OGD-00104"
	NamingParseValue       ErrorCode = "OGD-00105"
	NamingParseValues      ErrorCode = "OGD-00106"
	NamingParsePath        ErrorCode = "OGD-00107"
	NamingParsePathSegment ErrorCode = "OGD-00108"
	NamingParseBounds      ErrorCode = "OGD-00109"

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
	UnsupportedCharacterSet              ErrorCode = "OGD-00015"
	InvalidNetworkValue                  ErrorCode = "OGD-00016"
	InvalidNetworkExpectedValue          ErrorCode = "OGD-00017"
	InvalidNetworkContextValue           ErrorCode = "OGD-00018"
	InvalidNetworkContextExpectedValue   ErrorCode = "OGD-00019"
	ConnectionRefusedDetail              ErrorCode = "OGD-00020"
	NetworkRetryLimitExceeded            ErrorCode = "OGD-00026"
	NetworkServerErrorCode               ErrorCode = "OGD-00027"
	InvalidNetworkProperty               ErrorCode = "OGD-00028"
	PKCS8ParseFailed                     ErrorCode = "OGD-00029"
	PKCS8DecryptFailed                   ErrorCode = "OGD-00030"
	PKCS7PaddingInvalid                  ErrorCode = "OGD-00031"
	TLSCertificateVerificationFailed     ErrorCode = "OGD-00032"
	TLSCertificateDNMatchFailed          ErrorCode = "OGD-00033"
	PEMBlockDecryptFailed                ErrorCode = "OGD-00034"
	WalletKeyPairLoadFailed              ErrorCode = "OGD-00035"
	ConfiguredDNInvalid                  ErrorCode = "OGD-00036"
	CertificateSubjectInvalid            ErrorCode = "OGD-00037"
	PKCS8ParametersParseFailed           ErrorCode = "OGD-00038"
	PKCS8InitializationVectorParseFailed ErrorCode = "OGD-00039"
	DNMalformedAttribute                 ErrorCode = "OGD-00040"
	DNUnsupportedAttribute               ErrorCode = "OGD-00041"
	DNAttributeValueMissing              ErrorCode = "OGD-00042"
	DNMalformedEscape                    ErrorCode = "OGD-00043"
	DNMismatchAtRDN                      ErrorCode = "OGD-00044"
	InvalidNetworkLength                 ErrorCode = "OGD-00045"
	NetworkDataReadFailed                ErrorCode = "OGD-00046"
	InvalidNetworkExpectedLength         ErrorCode = "OGD-00047"
	InvalidNetworkContextExpectedLength  ErrorCode = "OGD-00048"
	ErrConnectionInband                  ErrorCode = "OGD-00049" // server error received in-band
	DNAttributeMissingFromCertificate    ErrorCode = "OGD-00111"
	DNUnsupportedAttributeOID            ErrorCode = "OGD-00112"
	DNDuplicateAttributeOID              ErrorCode = "OGD-00113"
	DNAttributeOIDValueTypeInvalid       ErrorCode = "OGD-00114"
	DNHexValueUnsupported                ErrorCode = "OGD-00115"
	DNAttributeValueInvalidUTF8          ErrorCode = "OGD-00116"
	CertificateSubjectTrailingData       ErrorCode = "OGD-00117"
	CertificateSubjectParseFailed        ErrorCode = "OGD-00118"
	WalletCACertificatesMissing          ErrorCode = "OGD-00119"
	WalletCACertificatesParseFailed      ErrorCode = "OGD-00120"
	BreakPacketReceived                  ErrorCode = "OGD-00121"
	RefuseDataParseFailed                ErrorCode = "OGD-00122"
	RedirectAddressMissing               ErrorCode = "OGD-00123"
	TLSRenegotiationUnsupported          ErrorCode = "OGD-00124"
	UnexpectedConnectResponse            ErrorCode = "OGD-00125"
	NamingInputMissing                   ErrorCode = "OGD-00126"
	NamingTokensMissing                  ErrorCode = "OGD-00127"
	NamingUnexpectedClosingParenthesis   ErrorCode = "OGD-00128"

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
	// NetworkInternalError indicates an unexpected network-driver state.
	NetworkInternalError ErrorCode = "OGD-00163"

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
	// TokenAuthenticationError indicates that an error occurred during token
	// authentication
	TokenAuthenticationError ErrorCode = "OGD-00202"
	// ValueRetrievalError indicates that a required session or descriptor value
	// could not be retrieved or was empty.
	ValueRetrievalError ErrorCode = "OGD-00203"
	// InvalidSignedTokenPrivateKey indicates that the signed token private key is missing,
	// malformed, or not an RSA signing key.
	InvalidSignedTokenPrivateKey ErrorCode = "OGD-00204"
	// ExpiredToken indicates that the resolved access token has expired
	// according to its JWT exp claim.
	ExpiredToken ErrorCode = "OGD-00205"
	// ProviderNotFound indicates that no provider was found in the registry for
	// the wanted provider type
	ProviderNotFound ErrorCode = "OGD-00206"
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
