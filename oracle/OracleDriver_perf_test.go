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

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
	oracleProviders "github.com/oracle/go-oracledb/v26/oracle/providers"
)

// BenchmarkPoolOpen measures the cost of checking out a pooled database
// connection and returning it to the pool on each iteration.
func BenchmarkPoolOpen(b *testing.B) {
	if TestingConfig == nil {
		b.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		b.Logf("error opening db: %s", err.Error())
		b.Fail()
	} else {
		for b.Loop() {
			conn, _ := db.Conn(context.Background())
			b.StopTimer()
			conn.Close()
			b.StartTimer()
		}
	}
}

// BenchmarkFullPooledConnectionCycle measures a pooled connection checkout
// followed by a ping before the connection is returned to the pool.
func BenchmarkFullPooledConnectionCycle(b *testing.B) {
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		b.Logf("error opening db: %s", err.Error())
		b.Fail()
	} else {
		for b.Loop() {
			conn, _ := db.Conn(context.Background())
			b.StopTimer()
			conn.PingContext(context.Background())
			b.StartTimer()
			conn.Close()
		}
	}
}

// BenchmarkPoolOpenOneCnx measures pooled connection checkout when the pool
// is constrained to a single open connection.
func BenchmarkPoolOpenOneCnx(b *testing.B) {
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		b.Logf("error opening db: %s", err.Error())
		b.Fail()
	} else {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(0)
		db.SetConnMaxLifetime(0)
		db.SetConnMaxIdleTime(0)
		b.ResetTimer()
		for b.Loop() {
			conn, _ := db.Conn(context.Background())
			b.StopTimer()
			conn.Close()
			b.StartTimer()
		}
	}
}

// BenchmarkPoolOpenWithWarmOneCnx measures single-connection pool checkout
// after priming the pool with one already-created connection.
func BenchmarkPoolOpenWithWarmOneCnx(b *testing.B) {
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		b.Logf("error opening db: %s", err.Error())
		b.Fail()
	} else {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(0)
		db.SetConnMaxLifetime(0)
		db.SetConnMaxIdleTime(0)

		// make sure we create one
		c, _ := db.Conn(context.Background())
		c.Close()

		b.ResetTimer()
		for b.Loop() {
			conn, _ := db.Conn(context.Background())
			b.StopTimer()
			conn.Close()
			b.StartTimer()
		}
	}
}

// BenchmarkSimpleOpen measures direct connector connection creation and
// close without using the sql.DB pool.
func BenchmarkSimpleOpen(b *testing.B) {
	if TestingConfig == nil {
		b.Skip("No configuration available")
	}

	connector, err := openTestConnectorWithConfig(TestingConfig)
	if err != nil {
		b.Logf("error opening db: %s", err.Error())
		b.Fail()
	} else {
		b.ResetTimer()
		for b.Loop() {
			conn, _ := connector.Connect(context.Background())
			b.StopTimer()
			conn.Close()
			b.StartTimer()
		}
	}
}

// BenchmarkSimpleSelectDual measures repeated execution of SELECT 1 FROM
// DUAL on a single checked-out connection.
func BenchmarkSimpleSelectDual(b *testing.B) {
	var val int

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		b.Logf("Error opening connection: %s", err.Error())
		b.Fail()
	} else {
		defer db.Close()
		conn, _ := db.Conn(context.Background())
		b.ResetTimer()
		for b.Loop() {
			rows, _ := conn.QueryContext(context.Background(), "SELECT 1 FROM DUAL")
			rows.Next()
			_ = rows.Scan(&val)
			rows.Close()
		}
		conn.Close()
	}

}

// BenchmarkSimpleSelect measures repeated scanning of rows from a simple
// two-column table using a single checked-out connection.
func BenchmarkSimpleSelect(b *testing.B) {
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		b.Logf("Error opening connection: %s", err.Error())
		b.Fail()
	} else {
		defer db.Close()

		cols := map[string]string{
			"id":   "number",
			"name": "varchar2(100)",
		}
		createTable(context.Background(), db, "t_select", cols)
		defer func() {
			dropTable(context.Background(), db, "t_select")
		}()

		db.ExecContext(context.Background(), "INSERT INTO t_select (id, name) VALUES(42, 'answer')")
		var id int64
		var name string
		conn, _ := db.Conn(context.Background())
		b.ResetTimer()
		for b.Loop() {
			rows, _ := conn.QueryContext(context.Background(), "select id, name from t_select")
			for rows.Next() {
				rows.Scan(&id, &name)
			}
			rows.Close()
		}
		conn.Close()
	}

}

// BenchmarkInsertMultipleValues measures repeated inserts that bind several
// Oracle data types through one checked-out connection.
func BenchmarkInsertMultipleValues(b *testing.B) {
	if TestingConfig == nil {
		b.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		b.Fail()
	} else {
		defer db.Close()

		cols := map[string]string{
			"id":      "NUMBER PRIMARY KEY",
			"n_col":   "NUMBER",
			"v_col":   "VARCHAR2(100)",
			"f_col":   "BINARY_DOUBLE",
			"d_col":   "DATE",
			"ts_col":  "TIMESTAMP(6)",
			"tzt_col": "TIMESTAMP(6) WITH TIME ZONE",
			"b_col":   "BOOLEAN",
			"r_col":   "RAW(2000)",
		}

		tname := createObjectName("execctx_100k_types_test1")
		createTable(context.Background(), db, tname, cols)
		defer func() {
			dropTable(context.Background(), db, tname)
		}()

		insSQL := "INSERT INTO execctx_100k_types_test1 (id, n_col, v_col, f_col, d_col, ts_col, tzt_col, b_col, r_col) " +
			"VALUES (:id, :n, :v, :f, :d, :ts, :tzt, :b, :r)"

		baseDate := time.Date(2024, time.January, 15, 0, 0, 0, 0, time.UTC)
		baseTS := time.Date(2024, time.January, 15, 10, 20, 30, 123456000, time.UTC)
		baseTSTZ := time.Date(2024, time.January, 15, 4, 50, 30, 123456000, time.FixedZone("+05:30", 5*3600+30*60))
		raw := []byte{0xDE, 0xAD, 0xBE, 0xEF}

		var dummyInt int64 = 1

		// wait for currors leak to be fixed
		conn, _ := db.Conn(context.Background())

		b.ResetTimer()
		for b.Loop() {
			// protect ourselves form cursor starvation

			conn.ExecContext(context.Background(), insSQL,
				sql.Named("id", dummyInt),
				sql.Named("n", dummyInt*10),
				sql.Named("v", "bulk"),
				sql.Named("f", float64(dummyInt)*1.01),
				sql.Named("d", baseDate),
				sql.Named("ts", baseTS),
				sql.Named("tzt", baseTSTZ),
				sql.Named("b", (dummyInt%2) == 0),
				sql.Named("r", raw),
			)

			dummyInt++
		}
		conn.Close()
	}
}

// TestDriver_TTIInitConnectionTiming compares 30 fresh connections per setting.
func TestDriver_TTIInitConnectionTiming(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	const samples = 30
	noFastAuth := errors.New("database does not advertise fast authentication")

	newConnector := func(enabled bool) *oracleConnector {
		t.Helper()
		c, err := openTestConnectorWithConfig(TestingConfig)
		if err != nil {
			t.Fatalf("create connector (TTIINIT enabled=%t): %v", enabled, err)
		}
		connector := c.(*oracleConnector)
		connector.connectorConfig.DriverProperties.TTIInitOptimizationEnabled = enabled
		return connector
	}

	disabled, enabled := newConnector(false), newConnector(true)
	factory := enabled.connInstantiatorFactory
	enabled.connInstantiatorFactory = func(config *oracleconfig.OracleDriverConfig, ns driverCommon.NetworkSession, registry common.Registry[oracleProviders.Provider], serverProPending bool) (driverCommon.ConnectionInstantiator, error) {
		if !ns.SupportsFastAuthentication() {
			return nil, noFastAuth
		}
		if serverProPending {
			return nil, errors.New("server rejected TTIINIT and requested normal negotiation")
		}
		return factory(config, ns, registry, serverProPending)
	}

	measureConnection := func(label string, c *oracleConnector) time.Duration {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		start := time.Now()
		conn, err := c.Connect(ctx)
		elapsed := time.Since(start)
		if errors.Is(err, noFastAuth) {
			t.Skip("database does not advertise fast authentication; TTIINIT cannot be timed")
		}
		if err != nil {
			t.Fatalf("%s connect: %v", label, err)
		}
		if err := conn.Close(); err != nil {
			t.Fatalf("%s close: %v", label, err)
		}
		return elapsed
	}

	var disabledTotal, enabledTotal time.Duration
	for range samples {
		disabledTotal += measureConnection("TTIINIT disabled", disabled)
		enabledTotal += measureConnection("TTIINIT enabled", enabled)
	}

	gain := 100 * float64(disabledTotal-enabledTotal) / float64(disabledTotal)
	t.Logf("TTIINIT disabled average: %v", disabledTotal/samples)
	t.Logf("TTIINIT enabled average: %v", enabledTotal/samples)
	t.Logf("TTIINIT optimization gain: %.2f%%", gain)
}
