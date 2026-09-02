// Tests migrated from OraHub MR 244.

package common

import "testing"

func TestNewCtxTimeoutCauseError(t *testing.T) {
	err := NewCtxTimeoutCauseError("connect", 250, "cid-1")

	if err.GetSource() != "connect" {
		t.Fatalf("source = %q, want connect", err.GetSource())
	}
	if err.GetValue() != 250 {
		t.Fatalf("value = %d, want 250", err.GetValue())
	}
	if err.GetEmitterID() != "cid-1" {
		t.Fatalf("emitter = %q, want cid-1", err.GetEmitterID())
	}
	if got, want := err.Error(), "timeout of 250 set by connect"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}
