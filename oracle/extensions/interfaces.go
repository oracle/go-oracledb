package extensions

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
)

// ConnSessionlessTx is implemented by connections that support Oracle
// sessionless transaction lifecycle operations in addition to standard BeginTx.
type ConnSessionlessTx interface {
	// BeginSessionlessTx starts a new sessionless transaction.
	//
	// Parameters:
	//   - ctx: The context is used until the transaction is suspended, committed
	//          or rolled back. If the context is canceled, the transaction will
	//          be rolled back.
	//   - opts: Standard transaction options.
	//   - timeout: Sessionless transaction timeout in seconds.
	//
	// Returns:
	//   - SessionlessTx: Started sessionless transaction.
	//   - error: Error if the transaction cannot be started.
	BeginSessionlessTx(ctx context.Context, opts sql.TxOptions, timeout uint16) (SessionlessTx, error)
	// ResumeSessionlessTx resumes the sessionless transaction identified by its
	// global transaction ID.
	//
	// Parameters:
	//   - ctx: The context is used until the transaction is suspended, committed
	//          or rolled back. If the context is canceled, the transaction will
	//          be rolled back.
	//   - globalTransactionID: Identifier of the sessionless transaction to resume.
	//
	// Returns:
	//   - SessionlessTx: Resumed sessionless transaction.
	//   - error: Error if the transaction cannot be resumed.
	ResumeSessionlessTx(ctx context.Context, globalTransactionID GlobalTransactionID) (SessionlessTx, error)
}

// GlobalTransactionID identifies a sessionless transaction.
type GlobalTransactionID []byte

// String returns the base64 encoding of the global transaction identifier.
//
// Returns:
//   - string: Base64-encoded transaction identifier.
func (value GlobalTransactionID) String() string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}

// SessionlessTx is a transaction that can be suspended and later resumed by
// its global transaction identifier.
type SessionlessTx interface {
	driver.Tx
	// Suspend detaches the transaction from its current connection.
	//
	// Returns:
	//   - error: Error if the transaction cannot be detached.
	Suspend() error
	// GlobalTransactionID returns the identifier associated with the transaction.
	//
	// Returns:
	//   - GlobalTransactionID: Transaction identifier, or nil when unavailable.
	GlobalTransactionID() GlobalTransactionID
}
