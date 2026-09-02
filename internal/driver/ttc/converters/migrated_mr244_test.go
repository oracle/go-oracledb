// Tests migrated from OraHub MRs 244 and 231.

package converters

import (
	"bytes"
	"testing"
)

func TestEncodeNull(t *testing.T) {
	got, err := EncodeNull(nil)
	if err != nil {
		t.Fatalf("EncodeNull returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("EncodeNull = %v, want nil", got)
	}
}

func TestEncodeBooleanAsNumber(t *testing.T) {
	tests := []struct {
		name string
		in   bool
		want []byte
	}{
		{name: "false", in: false, want: []byte{0x80}},
		{name: "true", in: true, want: []byte{0xC1, 0x02}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EncodeBooleanAsNumber(tt.in)
			if err != nil {
				t.Fatalf("EncodeBooleanAsNumber returned error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("EncodeBooleanAsNumber(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
