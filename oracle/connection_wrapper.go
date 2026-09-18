package oracle

import (
	"context"
	"database/sql"

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"github.com/oracle/go-oracledb/v26/oracle/extensions"
)

// connectionWrapper provides Oracle specific operations for a dedicated
// database/sql connection.
//
// The wrapped connection must be a connection returned by this driver.
type connectionWrapper struct {
	connection *sql.Conn
}

// NewConnectionWrapper validates and wraps a dedicated database/sql connection
// for Oracle specific operations.
//
// Parameters:
//   - connection: Dedicated database/sql connection to wrap.
//
// Returns:
//   - *connectionWrapper: Wrapper for the supplied connection.
//   - error: Error if the underlying driver connection type is not supported.
func NewConnectionWrapper(connection *sql.Conn) (*connectionWrapper, error) {
	var wrapper *connectionWrapper
	err := connection.Raw(func(c any) error {
		// Include here all functions/interfaces we want a connection to implement in
		// order to be wrapped by this wrapper
		type canBeWrapped interface {
			extensions.ConnSessionlessTx
		}
		_, ok := c.(canBeWrapped)
		if !ok {
			return common.NewOracleError(oracleErrors.UnsupportedFeature, nil, "Sessionless Transactions")
		}
		wrapper = &connectionWrapper{connection: connection}
		return nil
	})
	return wrapper, err
}

// BeginSessionlessTx starts a sessionless transaction on the wrapped connection.
//
// Parameters:
//   - ctx: Context used for the transaction start operation.
//   - opts: Standard transaction options.
//   - timeout: Sessionless transaction timeout in seconds.
//
// Returns:
//   - extensions.SessionlessTx: Started sessionless transaction.
//   - error: Error if the transaction cannot be started.
func (wrapper *connectionWrapper) BeginSessionlessTx(ctx context.Context, opts sql.TxOptions, timeout uint16) (extensions.SessionlessTx, error) {
	var publicSessionlessTransaction extensions.SessionlessTx
	err := wrapper.connection.Raw(func(c any) error {
		var err error
		publicSessionlessTransaction, err = c.(extensions.ConnSessionlessTx).BeginSessionlessTx(ctx, opts, timeout)
		return err
	})
	return publicSessionlessTransaction, err
}

// ResumeSessionlessTx resumes a sessionless transaction on connection.
//
// Parameters:
//   - ctx: Context used for the transaction resume operation.
//   - globalTransactionID: Identifier of the sessionless transaction to resume.
//
// Returns:
//   - extensions.SessionlessTx: Resumed sessionless transaction.
//   - error: Error if the transaction cannot be resumed.
func (wrapper *connectionWrapper) ResumeSessionlessTx(ctx context.Context, globalTransactionID extensions.GlobalTransactionID) (extensions.SessionlessTx, error) {
	var publicSessionlessTransaction extensions.SessionlessTx
	err := wrapper.connection.Raw(func(c any) error {
		var err error
		publicSessionlessTransaction, err = c.(extensions.ConnSessionlessTx).ResumeSessionlessTx(ctx, globalTransactionID)
		return err
	})
	return publicSessionlessTransaction, err
}
