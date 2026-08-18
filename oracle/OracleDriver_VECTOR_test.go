package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func openVectorTestDBWithConfig(cfg *TestConfig, lobPrefetchSize string) (*sql.DB, error) {
	if cfg == nil {
		return nil, sql.ErrConnDone
	}
	dsn := cfg.GetConnectionString()
	if lobPrefetchSize != "" {
		separator := "?"
		if strings.Contains(dsn, "?") {
			separator = "&"
		}
		dsn = fmt.Sprintf("%s%soracle.go.default_lob_prefetch_size=%s", dsn, separator, lobPrefetchSize)
	}

	db, err := sql.Open(cfg.Driver.Name, dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// TestDriver_Vector_Basic exercises the VECTOR lifecycle for all supported element types.
// It validates INSERT and UPDATE vector binds, fetching the stored value after each write,
// and deleting the row that contains the vector.
func TestDriver_Vector_Basic(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	if TestingConfig.DatabaseVersion < 23 {
		t.Skip("VECTOR datatype requires Oracle Database 23c+")
	}

	ctx := context.Background()
	const largeFloat64Dimensions = 1024
	largeFloat64 := make(VectorFloat64, largeFloat64Dimensions)
	updatedLargeFloat64 := make(VectorFloat64, largeFloat64Dimensions)
	for i := range largeFloat64 {
		largeFloat64[i] = float64(i) / 4
		updatedLargeFloat64[i] = -float64(i) / 8
	}

	tests := []struct {
		name            string
		tableName       string
		vecDDL          string
		bind            any
		want            any
		updatedBind     any
		updatedWant     any
		lobPrefetchSize string
	}{
		{
			name:            "float64",
			tableName:       "vector_ins_f64",
			vecDDL:          "VECTOR(1024, FLOAT64)",
			bind:            largeFloat64,
			want:            []float64(largeFloat64),
			updatedBind:     updatedLargeFloat64,
			updatedWant:     []float64(updatedLargeFloat64),
			lobPrefetchSize: "64",
		},
		{
			name:        "float32",
			tableName:   "vector_ins_f32",
			vecDDL:      "VECTOR(3, FLOAT32)",
			bind:        VectorFloat32{4.5, -5.25, 6.75},
			want:        []float32{4.5, -5.25, 6.75},
			updatedBind: VectorFloat32{-1.5, 2.25, 9.75},
			updatedWant: []float32{-1.5, 2.25, 9.75},
		},
		{
			name:        "int8",
			tableName:   "vector_ins_i8",
			vecDDL:      "VECTOR(3, INT8)",
			bind:        VectorInt8{1, -2, 3},
			want:        []int8{1, -2, 3},
			updatedBind: VectorInt8{-8, 0, 7},
			updatedWant: []int8{-8, 0, 7},
		},
		{
			name:        "binary",
			tableName:   "vector_ins_bin",
			vecDDL:      "VECTOR(16, BINARY)",
			bind:        VectorBinary{0xAA, 0x0F},
			want:        []byte{0xAA, 0x0F},
			updatedBind: VectorBinary{0x55, 0xF0},
			updatedWant: []byte{0x55, 0xF0},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, err := openVectorTestDBWithConfig(TestingConfig, tc.lobPrefetchSize)
			if err != nil {
				t.Fatalf("failed to open test DB: %v", err)
			}
			defer db.Close()

			_ = dropTable(ctx, db, tc.tableName)

			definition := map[string]string{
				"id":  "NUMBER PRIMARY KEY",
				"vec": tc.vecDDL,
			}
			if err := createTable(ctx, db, tc.tableName, definition); err != nil {
				t.Fatalf("create table %s failed: %v", tc.tableName, err)
			}
			defer func() {
				if err := dropTable(ctx, db, tc.tableName); err != nil {
					t.Errorf("drop table %s failed: %v", tc.tableName, err)
				}
			}()

			insertSQL := fmt.Sprintf("INSERT INTO %s (id, vec) VALUES (:1, :2)", tc.tableName)
			res, err := db.ExecContext(ctx, insertSQL, 1, tc.bind)
			if err != nil {
				t.Fatalf("insert into %s failed: %v", tc.tableName, err)
			}

			rowsAffected, err := res.RowsAffected()
			if err != nil {
				t.Fatalf("rows affected for %s failed: %v", tc.tableName, err)
			}
			if rowsAffected != 1 {
				t.Fatalf("unexpected rows affected for %s: got %d want 1", tc.tableName, rowsAffected)
			}
			t.Logf("[%s] insert rows affected: %d", tc.name, rowsAffected)

			var count int
			if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tc.tableName)).Scan(&count); err != nil {
				t.Fatalf("count query for %s failed: %v", tc.tableName, err)
			}
			if count != 1 {
				t.Fatalf("unexpected row count for %s: got %d want 1", tc.tableName, count)
			}
			t.Logf("[%s] row count after insert: %d", tc.name, count)

			querySQL := fmt.Sprintf("SELECT vec FROM %s WHERE id = :1", tc.tableName)
			assertFetched := func(phase string, want any) {
				switch tc.name {
				case "float64":
					var got []float64
					if err := db.QueryRowContext(ctx, querySQL, 1).Scan(&got); err != nil {
						t.Fatalf("fetch float64 %s failed: %v", phase, err)
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("float64 %s mismatch: got %v want %v", phase, got, want)
					}
				case "float32":
					var got []float32
					if err := db.QueryRowContext(ctx, querySQL, 1).Scan(&got); err != nil {
						t.Fatalf("fetch float32 %s failed: %v", phase, err)
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("float32 %s mismatch: got %v want %v", phase, got, want)
					}
				case "int8":
					var got []int8
					if err := db.QueryRowContext(ctx, querySQL, 1).Scan(&got); err != nil {
						t.Fatalf("fetch int8 %s failed: %v", phase, err)
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("int8 %s mismatch: got %v want %v", phase, got, want)
					}
				case "binary":
					var got []byte
					if err := db.QueryRowContext(ctx, querySQL, 1).Scan(&got); err != nil {
						t.Fatalf("fetch binary %s failed: %v", phase, err)
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("binary %s mismatch: got %v want %v", phase, got, want)
					}
				}
				t.Logf("[%s] %s vector length=%d", tc.name, phase, reflect.ValueOf(want).Len())
			}

			assertFetched("after insert", tc.want)

			updateSQL := fmt.Sprintf("UPDATE %s SET vec = :1 WHERE id = :2", tc.tableName)
			res, err = db.ExecContext(ctx, updateSQL, tc.updatedBind, 1)
			if err != nil {
				t.Fatalf("update %s failed: %v", tc.tableName, err)
			}
			rowsAffected, err = res.RowsAffected()
			if err != nil {
				t.Fatalf("update rows affected for %s failed: %v", tc.tableName, err)
			}
			if rowsAffected != 1 {
				t.Fatalf("unexpected update rows affected for %s: got %d want 1", tc.tableName, rowsAffected)
			}
			assertFetched("after update", tc.updatedWant)

			deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE id = :1", tc.tableName)
			res, err = db.ExecContext(ctx, deleteSQL, 1)
			if err != nil {
				t.Fatalf("delete from %s failed: %v", tc.tableName, err)
			}
			rowsAffected, err = res.RowsAffected()
			if err != nil {
				t.Fatalf("delete rows affected for %s failed: %v", tc.tableName, err)
			}
			if rowsAffected != 1 {
				t.Fatalf("unexpected delete rows affected for %s: got %d want 1", tc.tableName, rowsAffected)
			}

			if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tc.tableName)).Scan(&count); err != nil {
				t.Fatalf("count after delete for %s failed: %v", tc.tableName, err)
			}
			if count != 0 {
				t.Fatalf("unexpected row count after delete for %s: got %d want 0", tc.tableName, count)
			}
		})
	}
}
