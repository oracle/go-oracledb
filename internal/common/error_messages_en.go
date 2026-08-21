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
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// initMessagesEn Initialises error messages for English language.
func initMessagesEn() {
	// Document: No
	// Cause:    N/A
	// Action:   N/A
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.InvalidCredential), "invalid credential or not authorized: %s")
	// Document: No
	// Cause:    N/A
	// Action:   N/A
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.ConnectionLost), "database connection closed by peer")
	// Document: No
	// Cause:    N/A
	// Action:   N/A
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.AliasNotFound), "cannot connect to database. Could not find alias %s.")
	// Document: No
	// Cause:    N/A
	// Action:   N/A
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.NoListenerAvailable), "cannot connect. No listener at %s")
	// Document: No
	// Cause:    N/A
	// Action:   N/A
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.ProtocolViolation), "protocol violation")
	// Document: No
	// Cause:    N/A
	// Action:   N/A
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.ProtocolViolationLimitExceeded), "protocol violation, limit of %s exceeded: limit [%d] value [%d]")
	// Document: No
	// Cause:    N/A
	// Action:   N/A
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.LobClosed), "LOB already closed in the same transaction.")
	// Document: No
	// Cause:    N/A
	// Action:   N/A
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.LobOpen), "LOB already opened in the same transaction.")

	// Document : Yes
	// Cause    : The remote server has abruptly closed the connection.
	// Action   : Check the logs of the database instance to identify
	//            the cause and retry the operation.
	// Comment  : N/A
	message.SetString(language.English, string(oracleErrors.ChannelReadFailed), "End-of-stream detected. The remote peer has closed the connection.")
	// Document : Yes
	// Cause    : Write operation on the communication channel with the remote server
	//          : has failed.
	// Action   : Check the logs to identify the cause and retry the operation.
	// Comment  : N/A
	message.SetString(language.English, string(oracleErrors.ChannelWriteFailed), "Write to channel failed.")
	// Document: No
	// Cause:    N/A
	// Action:   N/A
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.UnknownHost), "unknown host specified.")
	// Document: No
	// Cause:    N/A
	// Action:   N/A
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.LogonDenied), "invalid credential or not authorized")
	// Document: Yes
	// Cause:    The service name specified in the connect descriptor is not registered with the listener.
	// Action:   Verify the service name in the connect string and ensure it's correctly configured in the listener.
	// Comment:  Common in multi-node setups or when the database service hasn't started.
	message.SetString(language.English, string(oracleErrors.InvalidServiceName), "Cannot connect to database. Service %s is not registered with the listener at host %s port %d. (CONNECTION_ID=%s)")
	// Document: Yes
	// Cause:    No available processes or handlers for the requested server type (e.g., shared vs dedicated).
	// Action:   Check database process limits or switch to a different server type in the connect string.
	// Comment:  Often occurs under high load or misconfiguration of shared servers.
	message.SetString(language.English, string(oracleErrors.NoAvailableHandler), "Cannot connect to database. No available %s handler for requested service %s at host %s port %d. (CONNECTION_ID=%s)")
	// Document: Yes
	// Cause:    The instance name specified in the connection string is not registered with the listener.
	// Action:   Verify the instance name and service name in the connect descriptor. Ensure the listener is configured correctly for the instance.
	// Comment:  This error occurs when trying to connect to a non-existent database instance.
	message.SetString(language.English, string(oracleErrors.UnknownInstance), "Cannot connect to database. Instance %s for service %s not known by listener at host %s port %d. (CONNECTION_ID=%s)")
	// Document: Yes
	// Cause:    The connection alias (e.g., from tnsnames.ora or Easy Connect string) could not be resolved due to syntax errors or invalid format.
	// Action:   Check the syntax of the Easy Connect string or tnsnames.ora entry for correctness.
	// Comment:  This is specific to alias resolution failures in naming methods.
	message.SetString(language.English, string(oracleErrors.UnresolvedAlias), "Cannot resolve connection alias due to errors in Easy Connect syntax %s")
	// Document: Yes
	// Cause:    The SID specified in the connect descriptor is not recognized by the listener.
	// Action:   Verify the SID in the connect string matches a valid database SID registered with the listener.
	// Comment:  Common when using legacy SID-based connections instead of service names.
	message.SetString(language.English, string(oracleErrors.InvalidSID), "Cannot connect to database. SID %s is not registered with the listener at host %s port %d. (CONNECTION_ID=%s)")
	// Document: Yes
	// Cause:    The listener did not receive the client's request within the allowed time.
	// Action:   Check for network delays or increase the listener timeout.
	// Comment:  Often due to slow networks or client-side delays.
	message.SetString(language.English, string(oracleErrors.ListenerRequestTimeout), `Listener has not received client's request in time allowed`)
	// Document: No
	// Cause:    N/A
	// Action:   N/A
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.ConnectionClosed), "closed connection")
	// Document: No
	// Cause:    The context deadline was exceeded during an operation such as send or receive.
	// Action:   Increase the context timeout, check for network latency, or optimize the operation.
	// Comment:  This is a custom driver error (OGD-00002) for handling context timeouts in network operations.
	message.SetString(language.English, string(oracleErrors.CtxTimeout), "context timed out during %s for %s.(CONNECTION_ID=%s) ")
	// Document: Yes
	// Cause:    A timeout occurred while closing the connection. A timeout is used
	//           to prevent the close operation from blocking indefinetely. This
	//           timeout was triggered before the connection could be closed.
	// Action:   Verify the network connectivity, ensure that the database
	//           server is reachable.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.ConnCloseTimeout), "a timeout occurred while closing the network session")
	// Document: Yes
	// Cause:    The client received a server to client function that it does not
	//           know.
	// Action:   Update the client version.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.UnknownSPFFunction), "unknown server to client function code received: %d")
	// Document: No
	// Cause:    The SPF message does not implement the Function interface.
	// Action:   This error should never happen in production.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.SPFNotFunction), "message does not implement the Function interface")
	// Document: No
	// Cause:    Ths SPF message could not be casted to the expected message type.
	// Action:   This error should never happen in production.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.InvalidSPFFunction), "message is not of the expected type: [%s]")

	// Document: No
	// Cause:    A parameter set in the connection string is invalid
	// Action:   Check connection string parameter
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.InvalidConnectionParameter), "invalid connection parameter value [%s] for [%s], accepted values [%s]")

	// Document: No
	// Cause:    A parameter set in more than one source is not allowed.
	// Action:   Check connection string parameter
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.ConflictingConnectionParameterSource), "parameter [%s] Can't be specified in more than one source")

	// Document: Yes
	// Cause:    DSN string is syntactically invalid.
	// Action:   Fix the DSN format.
	// Comment:  Arg[0]=component
	message.SetString(language.English, string(oracleErrors.NamingDSNInvalid), "Can't parse %s from DSN")

	// Document: Yes
	// Cause:    Connection string syntax is invalid.
	// Action:   Fix the connect descriptor syntax.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.NamingParseFailed), "invalid connection string")

	// Document: Yes
	// Cause:    The connection string has an invalid value for a field.
	// Action:   Fix the connection string.
	// Comment:  Arg[0]=value; Arg[1]=field
	message.SetString(language.English, string(oracleErrors.NamingEzConnectError), "invalid value %q for %s in connection string")

	// Document: Yes
	// Cause:    The connect descriptor contains an invalid value for a field.
	// Action:   Fix the connect descriptor.
	// Comment:  Arg[0]=value; Arg[1]=field
	message.SetString(language.English, string(oracleErrors.NamingContextError), "invalid value %q for %s in connect descriptor")

	// Document: No
	// Cause:    A failure occurred while marshalling TTC Message.
	// Action:   Inspect the cause and message name in args[0].
	// Comment:  Arg[0]: operation ()
	message.SetString(language.English, string(oracleErrors.FailMarshal), "Failed to marshal message: %s")

	// Document: No
	// Cause:    A failure occurred while unmarshalling TTC Message.
	// Action:   Inspect the cause and message name/description in args[0].
	// Comment:  Arg[0]: operation (TTIOER)
	message.SetString(language.English, string(oracleErrors.FailUnmarshal), "Failed to unmarshal message: %s")

	// Document: No
	// Cause:    LOB buffer configuration is invalid for the requested operation.
	// Action:   Ensure offset and length are within buffer bounds and greater than zero.
	// Comment:  Arg[0]: operation (marshal/unmarshal); Arg[1]: message name; Arg[2]: reason keyword
	message.SetString(language.English, string(oracleErrors.InvalidLOBBuffer), "Invalid LOB buffer configuration during %s (%s): %s")

	// Document: No
	// Cause:    A failure occurred while executing an OLOBOPS LOB operation.
	// Action:   Inspect the cause and operation name in args[0].
	// Comment:  Arg[0]: operation (read|write|trim|get-length|get-chunk-size|open|close|is-open)
	message.SetString(language.English, string(oracleErrors.LobExecError), "LOB operation failed during %s.")

	// Document: No
	// Cause:    The negotiated character set is not supported by the Go driver.
	// Action:   Reconnect using AL32UTF8 or AL16UTF16 character sets.
	// Comment:  Arg[0]: unsupported character set identifier.
	message.SetString(language.English, string(oracleErrors.UnsupportedCharacterSet), "Unsupported character set: %d")

	message.SetString(language.English, string(oracleErrors.ConnectTimeout), "%s Timeout of %d for %s.(CONNECTION_ID=%s)")

	// Document: Yes
	// Cause:    The listener was unable to start a dedicated server process for the connection request.
	// Action:   Increase the PROCESSES parameter in the database initialization file or check for resource constraints on the server.
	// Comment:  Often due to insufficient processes or memory on the database server.
	message.SetString(language.English, string(oracleErrors.ListenerFailed), "ORA-12500: TNS:listener failed to start a dedicated server process")

	// Document: Yes
	// Cause:    The listener could not find an available handler (dispatcher) for the requested protocol stack in shared server mode.
	// Action:   Increase dispatchers or check for high load on the database.
	// Comment:  Common in shared server configurations with insufficient resources.
	message.SetString(language.English, string(oracleErrors.NoDispatcherAvailable), "ORA-12516: TNS:listener could not find available handler with matching protocol stack")

	message.SetString(language.English, string(oracleErrors.ConnectFailed), "Cannot connect")

	// Table/Object access errors
	// Document: Yes
	// Cause:    The specified table or view does not exist in the database.
	// Action:   Verify the table or view name and ensure it exists in the current schema.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.TableOrViewNotFound), "table or view does not exist")

	// Document: Yes
	// Cause:    The user does not have the required privilege to perform the requested operation.
	// Action:   Contact the database administrator to grant the necessary privileges.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.InsufficientPrivilege), "insufficient privileges")

	// Document: Yes
	// Cause:    The table name specified in the SQL statement has invalid syntax.
	// Action:   Verify the table name syntax and ensure it follows Oracle naming conventions.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.InvalidTableName), "invalid table name")

	// Document: Yes
	// Cause:    The user does not have READ privilege on the specified object.
	// Action:   Contact the database administrator to grant the necessary READ privileges.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.MissingReadPrivilege), "missing READ privilege")

	// Document: No
	// Cause:    Empty input encountered in a converter operation.
	// Action:   Provide a non-empty value or handle NULL semantics.
	// Comment:  Arg[0]: converter type (e.g., CHAR, VARCHAR2, INTERVAL YEAR TO MONTH); Arg[1]: operation ("encode" or "decode")
	message.SetString(language.English, string(oracleErrors.ConverterEmptyInput), "Converter %s %s: empty input")

	// Document: No
	// Cause:    A converter value was outside the allowed range.
	// Action:   Inspect the reason in args[2] and correct the input bounds.
	// Comment:  Arg[0]: converter type (e.g., INTERVAL YEAR TO MONTH); Arg[1]: operation ("Encode" or "Decode"); Arg[2]: reason; Arg[3]: min; Arg[4]: max
	message.SetString(language.English, string(oracleErrors.ConverterRange), "Converter %s %s: %s; expected=%v..%v")

	// Document: No
	// Cause:    The value did not match the expected format/type/value.
	// Action:   Inspect the reason in args[2] and the expected value/format in args[3].
	// Comment:  Arg[0]: converter type (e.g., INTERVAL DAY TO SECOND); Arg[1]: operation ("Encode" or "Decode"); Arg[2]: reason; Arg[3]: expected
	message.SetString(language.English, string(oracleErrors.ConverterExpectedFormat), "Converter %s %s: %s; expected=%v")

	// Document: No
	// Cause:    A failure occurred while decoding a column value during row scan.
	// Action:   Inspect the cause (converter error) and the column name/index.
	// Comment:  Arg[0]: database type name (e.g., NUMBER, FLOAT, VARCHAR2); Arg[1]: column name; Arg[2]: column index
	message.SetString(language.English, string(oracleErrors.RowDecodeError), "Decode %s failed for column %s (index %d)")

	// Document: No
	// Cause:    Closing statement rows has failed
	// Action:
	// Comment:
	message.SetString(language.English, string(oracleErrors.RowsCloseFailed), "Closing the statement's rows failed: %s")

	// Document: No
	// Cause:    Failed while preparing or validating a statement for a given SQL kind.
	// Action:   Inspect the cause for more details.
	// Comment:  Arg[0]: SQL kind (SELECT|DML|PLSQL|OTHER); Arg[1]: cause/detail
	message.SetString(language.English, string(oracleErrors.StatementExecutionFailed), "failed to prepare %s statement: %s.")

	// Document: No
	// Cause:    Factory could not provide an executor for the SQL kind and operation.
	// Action:   Verify classification logic and supported kinds for the operation.
	// Comment:  Arg[0]: SQL kind (SELECT|DML|PLSQL|OTHER); Arg[1]: operation (Query/QueryContext|Exec/ExecContext)
	message.SetString(language.English, string(oracleErrors.StatementExecutionFactoryFailed), "failed to get Executor from factory, invalid type: %s for %s")

	// Document: No
	// Cause:    Factory could not provide an executor for the SQL kind and operation.
	// Action:   Verify classification logic and supported kinds for the operation.
	// Comment:  Arg[0]: SQL kind (SELECT|DML|PLSQL|OTHER); Arg[1]: operation (Query/QueryContext|Exec/ExecContext)
	message.SetString(language.English, string(oracleErrors.StatementCloseFailed), "failed to close statement: %s")

	// Document: No
	// Cause:    A failure occurred in runQuery.
	// Action:   Inspect the cause and operation name in args[0].
	// Comment:  Arg[0]: operation (push|flush|pull|column-meta|ttioer|invalid-dcb)
	message.SetString(language.English, string(oracleErrors.RunQueryError), "runQuery failed: %s.")

	// Document: No
	// Cause:    A failure occurred in runExec.
	// Action:   Inspect the cause and operation name in args[0].
	// Comment:  Arg[0]: operation (push|flush|pull|ttioer|unmarshal-dml-rows)
	message.SetString(language.English, string(oracleErrors.RunExecError), "runExec failed: %s.")

	// Document: No
	// Cause:    A failure occurred while obtaining a message instance from the factory.
	// Action:   Inspect the cause and operation name in args[0].
	// Comment:  Arg[0]: operation (get-rxd|get-bvc|get-oallrpa)
	message.SetString(language.English, string(oracleErrors.CallbackFactoryError), "message factory failed: %s.")

	// Document: No
	// Cause:    SQL ended with a ':' without a following identifier.
	// Action:   Ensure ':' is followed by a valid bind identifier or remove the trailing ':'.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.StatementParsingDanglingColon), "dangling colon at end of SQL")
	// Document: No
	// Cause:    Placeholder and argument counts mismatch for SQL bind processing.
	// Action:   Provide arguments matching the number of placeholders in the SQL.
	// Comment:  Arg[0]: number of placeholders found (%d); Arg[1]: provided argument count (%d)
	message.SetString(language.English, string(oracleErrors.StatementParsingPlaceholdersArgsMismatch), "%d placeholder(s) found, but %d arguments provided")
	// Document: No
	// Cause:    The number of provided NamedValue arguments does not match the count of unique bind names.
	// Action:   Supply exactly one argument for each unique bind name present in the SQL.
	// Comment:  Arg[0]: provided NamedValue count (%d); Arg[1]: expected unique bind count (%d)
	message.SetString(language.English, string(oracleErrors.StatementParsingInvalidArgCount), "invalid argument count: provided %d, expected %d unique placeholder(s)")
	// Document: No
	// Cause:    A NamedValue was provided with a Name that does not correspond to any placeholder in the SQL.
	// Action:   Ensure NamedValue names match the placeholder names used in the SQL.
	// Comment:  Arg[0]: missing placeholder name (%q)
	message.SetString(language.English, string(oracleErrors.StatementParsingNameNotFound), "placeholder name %q not found in SQL")
	// Document: No
	// Cause:    The same placeholder occurrence was assigned more than once (duplicate argument).
	// Action:   Provide a single value per placeholder occurrence; remove duplicates.
	// Comment:  Arg[0]: placeholder name (%q); Arg[1]: 1-based occurrence index (%d)
	message.SetString(language.English, string(oracleErrors.StatementParsingDuplicateArg), "duplicate value for placeholder %q at occurrence index %d")
	// Document: No
	// Cause:    An ordinal was provided that is out of range (<= 0 or > number of placeholders).
	// Action:   Use ordinals within the range [1..N], where N is the number of placeholder occurrences.
	// Comment:  Arg[0]: provided ordinal (%d); Arg[1]: total placeholder occurrences (%d)
	message.SetString(language.English, string(oracleErrors.StatementParsingInvalidOrdinal), "invalid ordinal provided %d, number of placeholders found %d")
	// Document: No
	// Cause:    After processing inputs, at least one placeholder occurrence was left without a value.
	// Action:   Provide a value for each placeholder occurrence in the SQL.
	// Comment:  Arg[0]: placeholder name (%q); Arg[1]: 1-based occurrence index (%d)
	message.SetString(language.English, string(oracleErrors.StatementParsingMissingValue), "missing value for placeholder %q at occurrence index %d")
	// Document: No
	// Cause:    The provided SQL identifier is invalid.
	// Action:   Use an identifier with a supported length and characters.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.InvalidIdentifier), "invalid SQL identifier")

	// Document: No
	message.SetString(language.English, string(oracleErrors.CancelOperationError), "cancel operation failed")

	// Document: No
	message.SetString(language.English, string(oracleErrors.InternalError), "internal error occurred.")
	// Document: No
	// Cause:    The shelf was used before a localization service was registered.
	// Action:   Ensure the connection instantiator registers a localization service before returning the shelf.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.MissingLocalizationService), "missing localization service on shelf")

	// Document: No
	// Cause:    A requested feature is not implemented by the driver.
	// Action:   Avoid using the unsupported feature or upgrade to a version that supports it.
	// Comment:  Arg[0]: feature name
	message.SetString(language.English, string(oracleErrors.UnsupportedFeature), "%s feature is not supported")

	// Document: No
	// Cause:    The destination provided in sql.Out is invalid (nil, non-pointer,
	//           nil pointer, or pointer-to-pointer).
	// Action:   Provide a non-nil pointer to a concrete value in sql.Out.Dest.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.InvalidSqlOutParameter), "invalid sql.Out parameter : %s")

	// Document: No
	// Cause:    The destination provided in sql.Out is invalid (nil, non-pointer,
	//           nil pointer, or pointer-to-pointer).
	// Action:   Provide a non-nil pointer to a concrete value in sql.Out.Dest.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.InvalidFileConfig), "failed to load configuration")

	// Document: No
	// Cause:    A call was made to commit or rollback and there is not active
	//           transaction.
	// Action:   Start a transaction befor calling commit or rollback.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.NotInTransaction), "Not in transaction")
	// Document: No
	// Cause:    The provided isolation level is not supported.
	// Action:   Use one of the following isolation levels: LevelSerializable or
	//           LevelReadCommitted.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.IsolationLevelNotSupported), "Isolation level not supported")
	// Document: No
	// Cause:    An attempt was made to begin a transaction while a transaction is
	//           already started.
	// Action:   Commit or rollback the current transaction before starting a new
	//           one.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.AlreadyInTransaction), "Isolation level not supported")
	// Document: No
	// Cause:    An error occurred while committing or rolling back the transaction.
	// Action:   Try agian.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.ErrorInTransaction), "An error occurred while performing transaction operation %s")
	// Document: No
	// Cause:    An error occurred while executing the ALTER SESSION statement to
	//           set the transaction ISOLATION LEVEL.
	// Action:   Verify the cause of the error, and try again.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.ConfigureTransactionError), "Failed to configure transaction")
	// Document: No
	// Cause:    An error occurred while authenticating.
	// Action:   Check the credentials and try again.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.AuthenticatorError), "An error occurred during authentication")
	// Document: No
	// Cause:    An error occurred while negotiating the connection.
	// Action:   Try again.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.NegotiatorError), "An error occurred while negotiating the connection")
	// Document: No
	// Cause:    An error occurred while marshalling or unmarshalling a value.
	// Action:   Try again.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.MarshalEngineError), "An error occurred while marshalling or unmarshalling a value of type %s")
	// Document: No
	// Cause:    An error occurred while flushing data.
	// Action:   Try again.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.MarshalEngineFlushError), "An error occurred while flushing data")
	// Document: No
	// Cause:    An error occurred while reading a message.
	// Action:   Check network connection and try again.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.StreamerReadError), "An error occurred while reading a message")
	// Document: No
	// Cause:    An error occurred while writing a message.
	// Action:   Check network connection and try again.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.StreamerWriteError), "An error occurred while writing a message")
	// Document: No
	// Cause:    Database is in NOMOUNT state
	// Action:   ?
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.DatabaseMountStateError), "database is in NOMOUNT state")
	// Document: Yes
	// Cause:    No username was provided
	// Action:   Provide a username and try agian.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.EmptyUsernameError), "empty username not supported")
	// Document: Yes
	// Cause:    No authenticator available for given parameters.
	// Action:   Only username and password are supported.
	// Comment:  N/A
	message.SetString(language.English, string(oracleErrors.NoAuthenticatorError), "no authenticator available for given parameters")
	// Document: No
	// Cause:    The server timezone could not be queried, retrieved, or parsed during connection initialization.
	// Action:   Verify the database connection and the DBTIMEZONE value returned by the server.
	// Comment:  Arg[0]: operation (query|retrieve|parse)
	message.SetString(language.English, string(oracleErrors.ServerTimeZoneError), "Failed to %s server timezone")

	// Document: No
	// Cause:    Token-based authentication resolved an empty token value from AccessToken or TokenLocation.
	// Action:   Provide a non-empty token directly with AccessToken or ensure the token file contains a valid token.
	message.SetString(language.English, string(oracleErrors.EmptyTokenError), "empty token for %s")

	// Document: No
	// Cause:    Token-based authentication requires a token location, but no token location was configured.
	// Action:   Set TokenLocation or provide AccessToken directly.
	message.SetString(language.English, string(oracleErrors.MissingTokenLocationError), "missing token location")

	// Document: No
	// Cause:    A required value could not be retrieved or was empty.
	// Action:   Verify that the required value is populated before token authentication uses it.
	// Comment:  Arg[0]: required property key
	message.SetString(language.English, string(oracleErrors.ValueRetrievalError), "failed to retrieve value %s")

	// Document: No
	// Cause:    The OCI private key is missing, malformed, or not a supported RSA signing key.
	// Action:   Verify that oci_db_key.pem exists, is valid PEM/PKCS8, and contains an RSA private key.
	message.SetString(language.English, string(oracleErrors.InvalidPrivateKey), "invalid OCI private key")

	// Document: No
	// Cause:    The configured access token has expired according to its JWT exp claim.
	// Action:   Generate or retrieve a fresh access token and try the connection again.
	message.SetString(language.English, string(oracleErrors.ExpiredToken), "configured access token has expired")

}
