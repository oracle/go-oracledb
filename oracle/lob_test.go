/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package oracle

import "testing"

func TestBlobValueSharesBackingStorage(t *testing.T) {
	t.Parallel()

	input := []byte{1, 2, 3}
	blob := Blob(input)
	value := blob.OracleBlobValue()

	if len(value) != len(input) || &value[0] != &input[0] {
		t.Fatal("Blob value should expose the original byte slice without copying")
	}
}
