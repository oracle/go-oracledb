/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package ttc

import (
	"database/sql/driver"
	"testing"
)

type testOracleBlob []byte

func (b testOracleBlob) OracleBlobValue() []byte {
	return b
}

func TestCheckNamedValueAcceptsOracleBlob(t *testing.T) {
	t.Parallel()

	namedValue := &driver.NamedValue{Value: testOracleBlob{1, 2, 3}}

	if err := checkNamedValue(namedValue); err != nil {
		t.Fatalf("checkNamedValue returned error: %v", err)
	}
}

func TestNewTTIOacBlobBindUsesBlobType(t *testing.T) {
	t.Parallel()

	oac := newTTIOacBlobBind(40).(*tTIoac)

	if oac.requestedtype != DtyBlob || DtyType(oac.dataType) != DtyBlob {
		t.Fatalf("BLOB bind OAC types = (%d, %d), want %d", oac.requestedtype, oac.dataType, DtyBlob)
	}
}
