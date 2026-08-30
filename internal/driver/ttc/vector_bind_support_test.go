package ttc

import (
	"database/sql/driver"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// Named VECTOR types must bypass database/sql's default conversion so their
// concrete type reaches the encoder registry unchanged.
func TestCheckVectorNamedValue(t *testing.T) {
	cases := []struct {
		name  string
		value any
		ok    bool
	}{
		{"float64", driverCommon.VectorFloat64{1, 2}, true},
		{"float32", driverCommon.VectorFloat32{1, 2}, true},
		{"int8", driverCommon.VectorInt8{1, 2}, true},
		{"binary", driverCommon.VectorBinary{0xAA}, true},
		{"plain float64", []float64{1, 2}, false},
		{"raw bytes", []byte{1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkVectorNamedValue(&driver.NamedValue{Ordinal: 1, Value: tc.value})
			if tc.ok && err != nil {
				t.Fatalf("expected accepted VECTOR type, got %v", err)
			}
			if !tc.ok && err != driver.ErrSkip {
				t.Fatalf("expected driver.ErrSkip, got %v", err)
			}
		})
	}
}
