package ttc

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"

	"github.com/oracle/go-oracledb/driver/adt"
)

// GetObjectType returns a named-type descriptor cached on this physical
// connection. The cache is deliberately not global: TOID and type-version
// metadata participate in each statement's OAC and must remain aligned with
// the connection which obtained it.
func (c *Connection) GetObjectType(ctx context.Context, typeName string) (*adt.ObjectType, error) {
	key := strings.TrimSpace(typeName)
	c.adtMu.Lock()
	defer c.adtMu.Unlock()
	if typ := c.adtTypes[key]; typ != nil {
		return typ, nil
	}
	typ, err := loadADTObjectType(ctx, connectionADTExecer{connection: c}, key)
	if err != nil {
		return nil, err
	}
	c.adtTypes[key] = typ
	c.adtTypes[typ.FullName()] = typ
	c.shelf.adtByTOID[string(typ.TOID)] = typ
	return typ, nil
}

// connectionADTExecer adapts the driver's internal driver.Conn API to the
// database/sql-shaped Execer accepted by GetObjectType. It preserves use of
// the same physical connection while the descriptor is being retrieved.
type connectionADTExecer struct{ connection *Connection }

func (e connectionADTExecer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	values := make([]driver.NamedValue, len(args))
	for i, arg := range args {
		values[i] = driver.NamedValue{Ordinal: i + 1, Value: arg}
	}
	return e.connection.ExecContext(ctx, query, values)
}
