// Tests migrated from OraHub MRs 244 and 231.

package common

import (
	"testing"

	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
)

func TestShelf_ConnectionProperties(t *testing.T) {
	shelf := NewShelf[int]()

	props := &oracleconfig.OracleDriverProperties{StrictNullValueHandling: true, DefaultLobPrefetchSize: 1024}
	if returned := shelf.UpdateConnectionProperties(props); returned != shelf {
		t.Fatal("UpdateConnectionProperties should return the shelf")
	}

	if got := shelf.GetConnectionProperties(); got != props {
		t.Fatalf("GetConnectionProperties = %p, want %p", got, props)
	}
}

func TestUtility_B1ArrayToString(t *testing.T) {
	tests := []struct {
		name string
		in   B1Array
		want string
	}{
		{name: "nil", in: nil, want: ""},
		{name: "empty", in: B1Array{}, want: ""},
		{name: "ascii", in: B1Array("hello"), want: "hello"},
		{name: "skips trailing null", in: B1Array{'a', 0}, want: "a"},
		{name: "invalid utf8 replacement", in: B1Array{0xff}, want: "�"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := B1ArrayToString(tt.in); got != tt.want {
				t.Fatalf("B1ArrayToString(%v) = %q, want %q", []byte(tt.in), got, tt.want)
			}
			if got := tt.in.String(); got != tt.want {
				t.Fatalf("B1Array.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUtility_GenerateRandomBytes(t *testing.T) {
	buf := make([]byte, 8)
	if err := GenerateRandomBytes(buf); err != nil {
		t.Fatalf("GenerateRandomBytes returned error: %v", err)
	}
	if len(buf) != 8 {
		t.Fatalf("buffer length changed: got %d, want 8", len(buf))
	}
}
