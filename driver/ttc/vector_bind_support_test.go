package ttc

import (
	"database/sql/driver"
	"testing"

	"github.com/oracle/go-driver/driver/common"
)

// TestCheckVectorNamedValue verifies that only the named VECTOR slice types
// bypass database/sql's default bind conversion; plain slices remain unchanged.
func TestCheckVectorNamedValue(t *testing.T) {
	cases := []struct {
		name  string
		value any
		ok    bool
	}{
		{name: "vector float64", value: common.VectorFloat64{1, 2}, ok: true},
		{name: "vector float32", value: common.VectorFloat32{1, 2}, ok: true},
		{name: "vector int8", value: common.VectorInt8{1, 2}, ok: true},
		{name: "binary", value: common.VectorBinary{0xAA}, ok: true},
		{name: "plain float64", value: []float64{1, 2}, ok: false},
		{name: "plain float32", value: []float32{1, 2}, ok: false},
		{name: "plain int8", value: []int8{1, 2}, ok: false},
		{name: "raw bytes", value: []byte{0x01}, ok: false},
		{name: "string", value: "x", ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nv := driver.NamedValue{Ordinal: 1, Value: tc.value}
			err := checkVectorNamedValue(&nv)
			if tc.ok {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}
			if err != driver.ErrSkip {
				t.Fatalf("expected driver.ErrSkip, got %v", err)
			}
		})
	}
}
