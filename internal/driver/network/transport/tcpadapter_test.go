/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package transport

import (
	"context"
	"errors"
	"net"
	"testing"
)

// TestNormalizeDialError_PreservesDNSTimeout verifies that a timed-out DNS
// lookup is not converted into a generic connection timeout.
func TestNormalizeDialError_PreservesDNSTimeout(t *testing.T) {
	dnsErr := &net.DNSError{Err: "i/o timeout", Name: "db.example.com", IsTimeout: true}
	err := &net.OpError{Op: "dial", Net: "tcp", Err: dnsErr}

	got := normalizeDialError(context.Background(), err, Address{}, "test-id")
	var gotDNSErr *net.DNSError
	if !errors.As(got, &gotDNSErr) {
		t.Fatalf("normalized error = %T, want an error wrapping *net.DNSError", got)
	}
}
