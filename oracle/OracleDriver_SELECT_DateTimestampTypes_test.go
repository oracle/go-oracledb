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
	"testing"
	"time"
)

// TestDriver_Select_DATE
// Purpose: End-to-end SELECT for DATE. Values are scanned into time.Time and verified.
func TestDriver_Select_DATE(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("date_types_test")
	cols := map[string]string{
		"id": "NUMBER PRIMARY KEY",
		"d":  "DATE",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()
	inserts := []string{
		"INSERT INTO " + table + " (id, d) VALUES (1, DATE '2024-01-15')",
		"INSERT INTO " + table + " (id, d) VALUES (2, DATE '1999-12-31')",
		"INSERT INTO " + table + " (id, d) VALUES (3, DATE '2000-02-29')",
		"INSERT INTO " + table + " (id, d) VALUES (4, DATE '1969-07-20')",
		"INSERT INTO " + table + " (id, d) VALUES (5, DATE '2077-11-05')",
	}
	for i, sql := range inserts {
		if _, err := db.ExecContext(ctx, sql); err != nil {
			t.Fatalf("insert %d failed: %v", i+1, err)
		}
	}

	rs, err := db.QueryContext(ctx, "SELECT id, d FROM "+table+" ORDER BY id")
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	type rowExp struct {
		id   int
		date time.Time
	}

	loc := time.UTC
	exp := []rowExp{
		{id: 1, date: time.Date(2024, time.January, 15, 0, 0, 0, 0, loc)},
		{id: 2, date: time.Date(1999, time.December, 31, 0, 0, 0, 0, loc)},
		{id: 3, date: time.Date(2000, time.February, 29, 0, 0, 0, 0, loc)},
		{id: 4, date: time.Date(1969, time.July, 20, 0, 0, 0, 0, loc)},
		{id: 5, date: time.Date(2077, time.November, 5, 0, 0, 0, 0, loc)},
	}

	idx := 0
	for rs.Next() {
		var (
			id int
			d  time.Time
		)
		if err := rs.Scan(&id, &d); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		t.Logf("DATE row %d: id=%d DATE=%s", idx, id, d.Format(time.RFC3339Nano))

		if idx >= len(exp) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		e := exp[idx]
		if id != e.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, e.id)
		}
		assertSameYMDHMSNanos(t, idx, "DATE", d, e.date)
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(exp) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(exp))
	}
}

// TestDriver_Select_TIMESTAMP
// Purpose: End-to-end SELECT for TIMESTAMP. Values are scanned into time.Time and verified.
func TestDriver_Select_TIMESTAMP(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("ts_types_test")
	cols := map[string]string{
		"id": "NUMBER PRIMARY KEY",
		"ts": "TIMESTAMP",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	inserts := []string{
		"INSERT INTO " + table + " (id, ts) VALUES (1, TIMESTAMP '2024-01-15 10:20:30.123456')",
		"INSERT INTO " + table + " (id, ts) VALUES (2, TIMESTAMP '1999-12-31 23:59:59.000000')",
		"INSERT INTO " + table + " (id, ts) VALUES (3, TIMESTAMP '2000-02-29 00:00:00.654321')",
		"INSERT INTO " + table + " (id, ts) VALUES (4, TIMESTAMP '1969-07-20 20:17:40.500000')",
		"INSERT INTO " + table + " (id, ts) VALUES (5, TIMESTAMP '2077-11-05 07:25:00.999999')",
		"INSERT INTO " + table + " (id, ts) VALUES (6, TIMESTAMP '2000-01-01 00:00:00.999999')",
		"INSERT INTO " + table + " (id, ts) VALUES (7, TIMESTAMP '0001-01-01 00:00:00.000000')",
		"INSERT INTO " + table + " (id, ts) VALUES (8, TIMESTAMP '9999-12-31 23:59:59.999999')",
	}
	for i, sql := range inserts {
		if _, err := db.ExecContext(ctx, sql); err != nil {
			t.Fatalf("insert %d failed: %v", i+1, err)
		}
	}

	rs, err := db.QueryContext(ctx, "SELECT id, ts FROM "+table+" ORDER BY id")
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	type rowExp struct {
		id int
		ts time.Time
	}

	loc := time.UTC
	exp := []rowExp{
		{id: 1, ts: time.Date(2024, time.January, 15, 10, 20, 30, 123456000, loc)},
		{id: 2, ts: time.Date(1999, time.December, 31, 23, 59, 59, 0, loc)},
		{id: 3, ts: time.Date(2000, time.February, 29, 0, 0, 0, 654321000, loc)},
		{id: 4, ts: time.Date(1969, time.July, 20, 20, 17, 40, 500000000, loc)},
		{id: 5, ts: time.Date(2077, time.November, 5, 7, 25, 0, 999999000, loc)},
		{id: 6, ts: time.Date(2000, time.January, 1, 0, 0, 0, 999999000, loc)},
		{id: 7, ts: time.Date(1, time.January, 1, 0, 0, 0, 0, loc)},
		{id: 8, ts: time.Date(9999, time.December, 31, 23, 59, 59, 999999000, loc)},
	}

	idx := 0
	for rs.Next() {
		var (
			id int
			ts time.Time
		)
		if err := rs.Scan(&id, &ts); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		t.Logf("TIMESTAMP row %d: id=%d TS=%s", idx, id, ts.Format(time.RFC3339Nano))

		if idx >= len(exp) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		e := exp[idx]
		if id != e.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, e.id)
		}
		assertSameYMDHMSNanos(t, idx, "TIMESTAMP", ts, e.ts)
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(exp) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(exp))
	}
}

// TestDriver_Select_TimestampWithTimeZone
// Purpose: End-to-end SELECT for TIMESTAMP WITH TIME ZONE. Validates components and offset seconds.
func TestDriver_Select_TimestampWithTimeZone(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("tstz_types_test")
	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"tstz": "TIMESTAMP(9) WITH TIME ZONE",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	inserts := []string{
		"INSERT INTO " + table + " (id, tstz) VALUES (1, TO_TIMESTAMP_TZ('2024-01-15 04:50:30.123456 +05:30','YYYY-MM-DD HH24:MI:SS.FF TZH:TZM'))",
		"INSERT INTO " + table + " (id, tstz) VALUES (2, TO_TIMESTAMP_TZ('1999-12-31 16:00:00.000000 -08:00','YYYY-MM-DD HH24:MI:SS.FF TZH:TZM'))",
		"INSERT INTO " + table + " (id, tstz) VALUES (3, TO_TIMESTAMP_TZ('2024-12-31 23:59:59.999999 +00:00','YYYY-MM-DD HH24:MI:SS.FF TZH:TZM'))",
		"INSERT INTO " + table + " (id, tstz) VALUES (4, TO_TIMESTAMP_TZ('2023-07-01 12:34:56.123456789 +09:30','YYYY-MM-DD HH24:MI:SS.FF9 TZH:TZM'))",
		"INSERT INTO " + table + " (id, tstz) VALUES (5, TO_TIMESTAMP_TZ('2022-02-02 08:00:00.0 +14:00','YYYY-MM-DD HH24:MI:SS.FF TZH:TZM'))",
		"INSERT INTO " + table + " (id, tstz) VALUES (6, TO_TIMESTAMP_TZ('1970-01-01 00:00:00.000000000 -12:00','YYYY-MM-DD HH24:MI:SS.FF9 TZH:TZM'))",
		"INSERT INTO " + table + " (id, tstz) VALUES (7, TO_TIMESTAMP_TZ('1985-03-01 05:15:45.987654321 -03:30','YYYY-MM-DD HH24:MI:SS.FF9 TZH:TZM'))",
		"INSERT INTO " + table + " (id, tstz) VALUES (8, TO_TIMESTAMP_TZ('2099-06-30 23:59:59.123456789 +12:45','YYYY-MM-DD HH24:MI:SS.FF9 TZH:TZM'))",
		"INSERT INTO " + table + " (id, tstz) VALUES (9, TO_TIMESTAMP_TZ('2020-12-31 23:59:59.999999999 +00:00','YYYY-MM-DD HH24:MI:SS.FF9 TZH:TZM'))",
		"INSERT INTO " + table + " (id, tstz) VALUES (10, TO_TIMESTAMP_TZ('2019-04-01 12:00:00.000000 +01:00','YYYY-MM-DD HH24:MI:SS.FF TZH:TZM'))",
	}
	for i, sql := range inserts {
		if _, err := db.ExecContext(ctx, sql); err != nil {
			t.Fatalf("insert %d failed: %v", i+1, err)
		}
	}

	rs, err := db.QueryContext(ctx, "SELECT id, tstz FROM "+table+" ORDER BY id")
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	type rowExp struct {
		id      int
		tstz    time.Time
		stzOffS int
	}

	z0530 := time.FixedZone("+05:30", 5*3600+30*60)
	zm0800 := time.FixedZone("-08:00", -(8 * 3600))
	z0000 := time.FixedZone("+00:00", 0)
	z0930 := time.FixedZone("+09:30", 9*3600+30*60)
	z1400 := time.FixedZone("+14:00", 14*3600)

	zm1200 := time.FixedZone("-12:00", -(12 * 3600))
	zm0330 := time.FixedZone("-03:30", -(3*3600 + 30*60))
	z1245 := time.FixedZone("+12:45", 12*3600+45*60)
	zPlus0100 := time.FixedZone("+01:00", 3600)

	exp := []rowExp{
		{id: 1, tstz: time.Date(2024, time.January, 15, 4, 50, 30, 123456000, z0530), stzOffS: 5*3600 + 30*60},
		{id: 2, tstz: time.Date(1999, time.December, 31, 16, 0, 0, 0, zm0800), stzOffS: -(8 * 3600)},
		{id: 3, tstz: time.Date(2024, time.December, 31, 23, 59, 59, 999999000, z0000), stzOffS: 0},
		{id: 4, tstz: time.Date(2023, time.July, 1, 12, 34, 56, 123456789, z0930), stzOffS: 9*3600 + 30*60},
		{id: 5, tstz: time.Date(2022, time.February, 2, 8, 0, 0, 0, z1400), stzOffS: 14 * 3600},
		{id: 6, tstz: time.Date(1970, time.January, 1, 0, 0, 0, 0, zm1200), stzOffS: -(12 * 3600)},
		{id: 7, tstz: time.Date(1985, time.March, 1, 5, 15, 45, 987654321, zm0330), stzOffS: -(3*3600 + 30*60)},
		{id: 8, tstz: time.Date(2099, time.June, 30, 23, 59, 59, 123456789, z1245), stzOffS: 12*3600 + 45*60},
		{id: 9, tstz: time.Date(2020, time.December, 31, 23, 59, 59, 999999999, z0000), stzOffS: 0},
		{id: 10, tstz: time.Date(2019, time.April, 1, 12, 0, 0, 0, zPlus0100), stzOffS: 3600},
	}

	idx := 0
	for rs.Next() {
		var (
			id   int
			tstz time.Time
		)
		if err := rs.Scan(&id, &tstz); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		_, off := tstz.Zone()
		t.Logf("TSTZ row %d: id=%d TSTZ=%s off=%d", idx, id, tstz.Format(time.RFC3339Nano), off)

		if idx >= len(exp) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		e := exp[idx]
		if id != e.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, e.id)
		}
		assertSameYMDHMSNanos(t, idx, "TSTZ", tstz, e.tstz)
		if _, off := tstz.Zone(); off != e.stzOffS {
			t.Fatalf("TSTZ offset mismatch at row %d: got %d want %d", idx, off, e.stzOffS)
		}
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(exp) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(exp))
	}
}

// TestDriver_Select_TimestampWithLocalTimeZone
// Purpose: End-to-end SELECT for TIMESTAMP WITH LOCAL TIME ZONE. Validates local clock components.
func TestDriver_Select_TimestampWithLocalTimeZone(t *testing.T) {
	//t.Parallel()
	// TODO : this test use hard coded object names
	//        fix setupPlsqlInOutObjects() to use random names
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("tsltz_types_test")
	cols := map[string]string{
		"id":    "NUMBER PRIMARY KEY",
		"tsltz": "TIMESTAMP(9) WITH LOCAL TIME ZONE",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	type insertCase struct {
		id     int
		sql    string
		expect time.Time
	}

	sessionTimeZoneOffsetH, sessionTimeZoneOffsetM, err := getRemoteDbAndSessionTimeZoneOffset(context.Background(), db)
	if err != nil {
		t.Fatalf("cannot get remote server TZ offset: %v", err)
	}

	localFromOffset := func(year int, month time.Month, day, hour, minute, second int, nanos int, offsetSeconds int) time.Time {
		loc := time.FixedZone("", offsetSeconds)
		return time.Date(year, month, day, hour, minute, second, nanos, loc).In(time.Local)
	}

	cases := []insertCase{
		{
			id:     1,
			sql:    "INSERT INTO " + table + " (id, tsltz) VALUES (1, TIMESTAMP '2024-01-15 10:20:30.123456')",
			expect: time.Date(2024, time.January, 15, 10, 20, 30, 123456000, time.Local),
		},
		{
			id:     2,
			sql:    "INSERT INTO " + table + " (id, tsltz) VALUES (2, TIMESTAMP '1999-12-31 23:59:59.0')",
			expect: time.Date(1999, time.December, 31, 23, 59, 59, 0, time.Local),
		},
		{
			id:     3,
			sql:    "INSERT INTO " + table + " (id, tsltz) VALUES (3, CAST(TO_TIMESTAMP_TZ('2024-07-01 12:34:56.987654321 -07:00','YYYY-MM-DD HH24:MI:SS.FF9 TZH:TZM') AS TIMESTAMP(9) WITH LOCAL TIME ZONE))",
			expect: localFromOffset(2024, time.July, 1, 12, 34, 56, 987654321, -(7 * 3600)),
		},
		{
			id:     4,
			sql:    "INSERT INTO " + table + " (id, tsltz) VALUES (4, CAST(TO_TIMESTAMP_TZ('1996-02-29 23:59:59.999999999 +14:00','YYYY-MM-DD HH24:MI:SS.FF9 TZH:TZM') AS TIMESTAMP(9) WITH LOCAL TIME ZONE))",
			expect: localFromOffset(1996, time.February, 29, 23, 59, 59, 999999999, 14*3600),
		},
		{
			id:     5,
			sql:    "INSERT INTO " + table + " (id, tsltz) VALUES (5, CAST(TO_TIMESTAMP_TZ('1970-01-01 00:00:00.000000000 -12:00','YYYY-MM-DD HH24:MI:SS.FF9 TZH:TZM') AS TIMESTAMP(9) WITH LOCAL TIME ZONE))",
			expect: localFromOffset(1970, time.January, 1, 0, 0, 0, 0, -(12 * 3600)),
		},
		{
			id:     6,
			sql:    "INSERT INTO " + table + " (id, tsltz) VALUES (6, CAST(TO_TIMESTAMP_TZ('2000-02-29 08:15:45.123456 +05:45','YYYY-MM-DD HH24:MI:SS.FF TZH:TZM') AS TIMESTAMP(9) WITH LOCAL TIME ZONE))",
			expect: localFromOffset(2000, time.February, 29, 8, 15, 45, 123456000, 5*3600+45*60),
		},
		{
			id:     7,
			sql:    "INSERT INTO " + table + " (id, tsltz) VALUES (7, CAST(TO_TIMESTAMP_TZ('2024-03-10 01:30:00.250000 -04:00','YYYY-MM-DD HH24:MI:SS.FF TZH:TZM') AS TIMESTAMP(9) WITH LOCAL TIME ZONE))",
			expect: localFromOffset(2024, time.March, 10, 1, 30, 0, 250000000, -(4 * 3600)),
		},
		{
			id:     8,
			sql:    "INSERT INTO " + table + " (id, tsltz) VALUES (8, CAST(TO_TIMESTAMP_TZ('2024-11-03 01:30:00.750000 -05:00','YYYY-MM-DD HH24:MI:SS.FF TZH:TZM') AS TIMESTAMP(9) WITH LOCAL TIME ZONE))",
			expect: localFromOffset(2024, time.November, 3, 1, 30, 0, 750000000, -(5 * 3600)),
		},
		{
			id:     9,
			sql:    "INSERT INTO " + table + " (id, tsltz) VALUES (9, CAST(TO_TIMESTAMP_TZ('0001-01-01 00:00:00.000000 +00:00','YYYY-MM-DD HH24:MI:SS.FF TZH:TZM') AS TIMESTAMP(9) WITH LOCAL TIME ZONE))",
			expect: localFromOffset(1, time.January, 1, 0, 0, 0, 0, 0),
		},
		{
			id:     10,
			sql:    "INSERT INTO " + table + " (id, tsltz) VALUES (10, CAST(TO_TIMESTAMP_TZ('2400-12-31 23:59:59.999999 +00:00','YYYY-MM-DD HH24:MI:SS.FF TZH:TZM') AS TIMESTAMP(9) WITH LOCAL TIME ZONE))",
			expect: localFromOffset(2400, time.December, 31, 23, 59, 59, 999999000, 0),
		},
	}

	for i, c := range cases {
		if _, err := db.ExecContext(ctx, c.sql); err != nil {
			t.Fatalf("insert %d failed: %v", i+1, err)
		}
	}

	rs, err := db.QueryContext(ctx, "SELECT id, tsltz FROM "+table+" ORDER BY id")
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	exp := cases

	idx := 0
	for rs.Next() {
		var (
			id    int
			tsltz time.Time
		)
		if err := rs.Scan(&id, &tsltz); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		t.Logf("TSLTZ row %d: id=%d TSLTZ=%s", idx, id, tsltz.Format(time.RFC3339Nano))

		if idx >= len(exp) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		e := exp[idx]
		if id != e.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, e.id)
		}
		// The first two rows are inserted using TIMESTAMP, the SESSION TIMEZONE has
		// to be taken into account
		if id == 1 || id == 2 {
			sessionOffset := sessionTimeZoneOffsetH*3600 + sessionTimeZoneOffsetM*60
			if _, dateOffset := tsltz.Zone(); dateOffset != sessionOffset {
				offset := dateOffset - sessionOffset
				tsltz = tsltz.Add(time.Second * time.Duration(-1*offset))
				t.Logf("Offset: %d, sessionOffset: %d, dateOffset: %d _n", offset, sessionOffset, dateOffset)
			}
		}
		assertSameYMDHMSNanos(t, idx, "TSLTZ", tsltz, e.expect)
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(exp) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(exp))
	}
}

func assertSameYMDHMSNanos(t *testing.T, idx int, label string, got, want time.Time) {
	if got.Year() != want.Year() || got.Month() != want.Month() || got.Day() != want.Day() ||
		got.Hour() != want.Hour() || got.Minute() != want.Minute() || got.Second() != want.Second() ||
		got.Nanosecond() != want.Nanosecond() {
		t.Errorf("%s mismatch at row %d:\n got: %s\nwant: %s", label, idx, got.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

// Prepared statement variants mirroring the above literal SQL tests.
// They use named binds and the Query + Next + Scan pattern throughout.

// TestDriver_Select_DATE_Prepared_Named
func TestDriver_Select_DATE_Prepared_Named(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("date_types_ps_test")
	cols := map[string]string{
		"id": "NUMBER PRIMARY KEY",
		"d":  "DATE",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	loc := time.UTC
	type rowExp struct {
		id   int
		date time.Time
	}
	exp := []rowExp{
		{id: 1, date: time.Date(2024, time.January, 15, 0, 0, 0, 0, loc)},
		{id: 2, date: time.Date(1999, time.December, 31, 0, 0, 0, 0, loc)},
		{id: 3, date: time.Date(2000, time.February, 29, 0, 0, 0, 0, loc)},
		{id: 4, date: time.Date(1969, time.July, 20, 0, 0, 0, 0, loc)},
		{id: 5, date: time.Date(2077, time.November, 5, 0, 0, 0, 0, loc)},
	}

	// Prepared INSERT (named binds)
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	ins := "INSERT INTO " + table + " (id, d) VALUES (:id, :d)"
	insStmt, err := db.PrepareContext(ctx, ins)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	t.Cleanup(func() { _ = insStmt.Close() })

	for _, r := range exp {
		if _, err := insStmt.ExecContext(ctx,
			sql.Named("id", r.id),
			sql.Named("d", r.date),
		); err != nil {
			t.Fatalf("insert id=%d failed: %v", r.id, err)
		}
	}

	// Prepared SELECT (no binds)
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	sel := "SELECT id, d FROM " + table + " ORDER BY id"
	selStmt, err := db.PrepareContext(ctx, sel)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	t.Cleanup(func() { _ = selStmt.Close() })

	rs, err := selStmt.QueryContext(ctx)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	idx := 0
	for rs.Next() {
		var id int
		var d time.Time
		if err := rs.Scan(&id, &d); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if idx >= len(exp) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		e := exp[idx]
		if id != e.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, e.id)
		}
		assertSameYMDHMSNanos(t, idx, "DATE", d, e.date)
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(exp) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(exp))
	}
}

// TestDriver_Select_TIMESTAMP_Prepared_Named
func TestDriver_Select_TIMESTAMP_Prepared_Named(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("ts_types_ps_test")
	cols := map[string]string{
		"id": "NUMBER PRIMARY KEY",
		"ts": "TIMESTAMP",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	loc := time.UTC
	type rowExp struct {
		id int
		ts time.Time
	}
	exp := []rowExp{
		{id: 1, ts: time.Date(2024, time.January, 15, 10, 20, 30, 123456000, loc)},
		{id: 2, ts: time.Date(1999, time.December, 31, 23, 59, 59, 0, loc)},
		{id: 3, ts: time.Date(2000, time.February, 29, 0, 0, 0, 654321000, loc)},
		{id: 4, ts: time.Date(1969, time.July, 20, 20, 17, 40, 500000000, loc)},
		{id: 5, ts: time.Date(2077, time.November, 5, 7, 25, 0, 999999000, loc)},
		{id: 6, ts: time.Date(2000, time.January, 1, 0, 0, 0, 999999000, loc)},
		{id: 7, ts: time.Date(1, time.January, 1, 0, 0, 0, 0, loc)},
		{id: 8, ts: time.Date(9999, time.December, 31, 23, 59, 59, 999999000, loc)},
	}

	// Prepared INSERT
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	ins := "INSERT INTO " + table + " (id, ts) VALUES (:id, :ts)"
	insStmt, err := db.PrepareContext(ctx, ins)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	t.Cleanup(func() { _ = insStmt.Close() })
	for _, r := range exp {
		if _, err := insStmt.ExecContext(ctx,
			sql.Named("id", r.id),
			sql.Named("ts", r.ts),
		); err != nil {
			t.Fatalf("insert id=%d failed: %v", r.id, err)
		}
	}

	// Prepared SELECT
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	sel := "SELECT id, ts FROM " + table + " ORDER BY id"
	selStmt, err := db.PrepareContext(ctx, sel)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	t.Cleanup(func() { _ = selStmt.Close() })

	rs, err := selStmt.QueryContext(ctx)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	idx := 0
	for rs.Next() {
		var id int
		var ts time.Time
		if err := rs.Scan(&id, &ts); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if idx >= len(exp) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		e := exp[idx]
		if id != e.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, e.id)
		}
		assertSameYMDHMSNanos(t, idx, "TIMESTAMP", ts, e.ts)
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(exp) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(exp))
	}
}

// TestDriver_Select_TimestampWithTimeZone_Prepared_Named
func TestDriver_Select_TimestampWithTimeZone_Prepared_Named(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("tstz_types_ps_test")
	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"tstz": "TIMESTAMP(9) WITH TIME ZONE",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	z0530 := time.FixedZone("+05:30", 5*3600+30*60)
	zm0800 := time.FixedZone("-08:00", -(8 * 3600))
	z0000 := time.FixedZone("+00:00", 0)
	z0930 := time.FixedZone("+09:30", 9*3600+30*60)
	z1400 := time.FixedZone("+14:00", 14*3600)
	zm1200 := time.FixedZone("-12:00", -(12 * 3600))
	zm0330 := time.FixedZone("-03:30", -(3*3600 + 30*60))
	z1245 := time.FixedZone("+12:45", 12*3600+45*60)
	zPlus0100 := time.FixedZone("+01:00", 3600)

	type rowExp struct {
		id      int
		tstz    time.Time
		stzOffS int
	}
	exp := []rowExp{
		{id: 1, tstz: time.Date(2024, time.January, 15, 4, 50, 30, 123456000, z0530), stzOffS: 5*3600 + 30*60},
		{id: 2, tstz: time.Date(1999, time.December, 31, 16, 0, 0, 0, zm0800), stzOffS: -(8 * 3600)},
		{id: 3, tstz: time.Date(2024, time.December, 31, 23, 59, 59, 999999000, z0000), stzOffS: 0},
		{id: 4, tstz: time.Date(2023, time.July, 1, 12, 34, 56, 123456789, z0930), stzOffS: 9*3600 + 30*60},
		{id: 5, tstz: time.Date(2022, time.February, 2, 8, 0, 0, 0, z1400), stzOffS: 14 * 3600},
		{id: 6, tstz: time.Date(1970, time.January, 1, 0, 0, 0, 0, zm1200), stzOffS: -(12 * 3600)},
		{id: 7, tstz: time.Date(1985, time.March, 1, 5, 15, 45, 987654321, zm0330), stzOffS: -(3*3600 + 30*60)},
		{id: 8, tstz: time.Date(2099, time.June, 30, 23, 59, 59, 123456789, z1245), stzOffS: 12*3600 + 45*60},
		{id: 9, tstz: time.Date(2020, time.December, 31, 23, 59, 59, 999999999, z0000), stzOffS: 0},
		{id: 10, tstz: time.Date(2019, time.April, 1, 12, 0, 0, 0, zPlus0100), stzOffS: 3600},
	}

	// Prepared INSERT
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	ins := "INSERT INTO " + table + " (id, tstz) VALUES (:id, :tstz)"
	insStmt, err := db.PrepareContext(ctx, ins)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	t.Cleanup(func() { _ = insStmt.Close() })
	for _, r := range exp {
		if _, err := insStmt.ExecContext(ctx,
			sql.Named("id", r.id),
			sql.Named("tstz", r.tstz),
		); err != nil {
			t.Fatalf("insert id=%d failed: %v", r.id, err)
		}
	}

	// Prepared SELECT
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	sel := "SELECT id, tstz FROM " + table + " ORDER BY id"
	selStmt, err := db.PrepareContext(ctx, sel)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	t.Cleanup(func() { _ = selStmt.Close() })

	rs, err := selStmt.QueryContext(ctx)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	idx := 0
	for rs.Next() {
		var id int
		var got time.Time
		if err := rs.Scan(&id, &got); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if idx >= len(exp) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		e := exp[idx]
		if id != e.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, e.id)
		}
		assertSameYMDHMSNanos(t, idx, "TSTZ", got, e.tstz)
		if _, off := got.Zone(); off != e.stzOffS {
			t.Fatalf("TSTZ offset mismatch at row %d: got %d want %d", idx, off, e.stzOffS)
		}
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(exp) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(exp))
	}
}

// TestDriver_Select_TimestampWithLocalTimeZone_Prepared_Named
func TestDriver_Select_TimestampWithLocalTimeZone_Prepared_Named(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("tsltz_types_ps_test")
	cols := map[string]string{
		"id":    "NUMBER PRIMARY KEY",
		"tsltz": "TIMESTAMP(9) WITH LOCAL TIME ZONE",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	type insertCase struct {
		id     int
		val    time.Time
		expect time.Time
	}
	localFromOffset := func(year int, month time.Month, day, hour, minute, second int, nanos int, offsetSeconds int) time.Time {
		loc := time.FixedZone("", offsetSeconds)
		return time.Date(year, month, day, hour, minute, second, nanos, loc).In(time.Local)
	}

	cases := []insertCase{
		{1, time.Date(2024, time.January, 15, 10, 20, 30, 123456000, time.Local), time.Date(2024, time.January, 15, 10, 20, 30, 123456000, time.Local)},
		{2, time.Date(1999, time.December, 31, 23, 59, 59, 0, time.Local), time.Date(1999, time.December, 31, 23, 59, 59, 0, time.Local)},
		{3, time.Date(2024, time.July, 1, 12, 34, 56, 987654321, time.FixedZone("-07:00", -(7*3600))), localFromOffset(2024, time.July, 1, 12, 34, 56, 987654321, -(7 * 3600))},
		{4, time.Date(1996, time.February, 29, 23, 59, 59, 999999999, time.FixedZone("+14:00", 14*3600)), localFromOffset(1996, time.February, 29, 23, 59, 59, 999999999, 14*3600)},
		{5, time.Date(1970, time.January, 1, 0, 0, 0, 0, time.FixedZone("-12:00", -(12*3600))), localFromOffset(1970, time.January, 1, 0, 0, 0, 0, -(12 * 3600))},
		{6, time.Date(2000, time.February, 29, 8, 15, 45, 123456000, time.FixedZone("+05:45", 5*3600+45*60)), localFromOffset(2000, time.February, 29, 8, 15, 45, 123456000, 5*3600+45*60)},
		{7, time.Date(2024, time.March, 10, 1, 30, 0, 250000000, time.FixedZone("-04:00", -(4*3600))), localFromOffset(2024, time.March, 10, 1, 30, 0, 250000000, -(4 * 3600))},
		{8, time.Date(2024, time.November, 3, 1, 30, 0, 750000000, time.FixedZone("-05:00", -(5*3600))), localFromOffset(2024, time.November, 3, 1, 30, 0, 750000000, -(5 * 3600))},
		{9, time.Date(1, time.January, 1, 0, 0, 0, 0, time.FixedZone("+00:00", 0)), localFromOffset(1, time.January, 1, 0, 0, 0, 0, 0)},
		{10, time.Date(2400, time.December, 31, 23, 59, 59, 999999000, time.FixedZone("+00:00", 0)), localFromOffset(2400, time.December, 31, 23, 59, 59, 999999000, 0)},
	}

	// Prepared INSERT
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	ins := "INSERT INTO " + table + " (id, tsltz) VALUES (:id, :tsltz)"
	insStmt, err := db.PrepareContext(ctx, ins)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	t.Cleanup(func() { _ = insStmt.Close() })
	for _, c := range cases {
		if _, err := insStmt.ExecContext(ctx,
			sql.Named("id", c.id),
			sql.Named("tsltz", c.val),
		); err != nil {
			t.Fatalf("insert id=%d failed: %v", c.id, err)
		}
	}

	// Prepared SELECT
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	sel := "SELECT id, tsltz FROM " + table + " ORDER BY id"
	selStmt, err := db.PrepareContext(ctx, sel)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	t.Cleanup(func() { _ = selStmt.Close() })

	rs, err := selStmt.QueryContext(ctx)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	idx := 0
	for rs.Next() {
		var id int
		var got time.Time
		if err := rs.Scan(&id, &got); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if idx >= len(cases) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		e := cases[idx]
		if id != e.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, e.id)
		}
		assertSameYMDHMSNanos(t, idx, "TSLTZ", got, e.expect)
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(cases) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(cases))
	}
}

// TestDriver_Select_TimestampWithTimeZone_Prepared_LoadLocation_And_Offset
// Purpose: Prepared statement with TIMESTAMP WITH TIME ZONE using:
//   - One input built with time.LoadLocation (region name, e.g. Asia/Kolkata)
//   - One input built with numeric offset (FixedZone)
//
// Verifies round-trip clock components and zone offset seconds.
func TestDriver_Select_TimestampWithTimeZone_Prepared_LoadLocation_And_Offset(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("tstz_ps_loc_off_test")
	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"tstz": "TIMESTAMP(9) WITH TIME ZONE",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	// Build inputs:
	// Case 1: Region-based location (stable offset zone to avoid DST ambiguity)
	locKolkata, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Skipf("cannot load location Asia/Kolkata: %v", err)
	}
	val1 := time.Date(2024, time.January, 15, 4, 50, 30, 123456789, locKolkata)
	_, off1 := val1.Zone()
	exp1 := time.Date(2024, time.January, 15, 4, 50, 30, 123456789, time.FixedZone("", off1))

	// Case 2: Numeric offset location
	off2 := -(8 * 3600) // -08:00
	val2 := time.Date(1999, time.December, 31, 16, 0, 0, 0, time.FixedZone("-08:00", off2))
	exp2 := time.Date(1999, time.December, 31, 16, 0, 0, 0, time.FixedZone("", off2))

	type rowCase struct {
		id     int
		in     time.Time
		expect time.Time
		expOff int
	}
	cases := []rowCase{
		{1, val1, exp1, off1},
		{2, val2, exp2, off2},
	}

	// Prepared INSERT
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	ins := "INSERT INTO " + table + " (id, tstz) VALUES (:id, :tstz)"
	insStmt, err := db.PrepareContext(ctx, ins)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	t.Cleanup(func() { _ = insStmt.Close() })
	for _, c := range cases {
		if _, err := insStmt.ExecContext(ctx,
			sql.Named("id", c.id),
			sql.Named("tstz", c.in),
		); err != nil {
			t.Fatalf("insert id=%d failed: %v", c.id, err)
		}
	}

	// Prepared SELECT
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	sel := "SELECT id, tstz FROM " + table + " ORDER BY id"
	selStmt, err := db.PrepareContext(ctx, sel)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	t.Cleanup(func() { _ = selStmt.Close() })

	rs, err := selStmt.QueryContext(ctx)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	idx := 0
	for rs.Next() {
		var id int
		var got time.Time
		if err := rs.Scan(&id, &got); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if idx >= len(cases) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		e := cases[idx]
		if id != e.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, e.id)
		}
		// Verify clock components and nanoseconds
		assertSameYMDHMSNanos(t, idx, "TSTZ", got, e.expect)
		// Verify zone offset seconds preserved
		if _, off := got.Zone(); off != e.expOff {
			t.Fatalf("TSTZ offset mismatch at row %d: got %d want %d", idx, off, e.expOff)
		}
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(cases) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(cases))
	}
}
