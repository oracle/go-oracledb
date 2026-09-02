// Tests migrated from OraHub MRs 244 and 231.

package naming

import (
	"testing"
)

func TestAddressString(t *testing.T) {
	addr := Address{Host: "db.example.com", Port: 1521}
	if got, want := addr.String(), "db.example.com:1521"; got != want {
		t.Fatalf("Address.String() = %q, want %q", got, want)
	}
}
