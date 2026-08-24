/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package oracle

// Blob marks a byte slice for Oracle BLOB binding. Unlike a plain []byte RAW
// bind, Blob supports payloads larger than Oracle's scalar RAW limit and can
// appear before other bind variables in a statement.
type Blob []byte

// OracleBlobValue exposes the payload to the driver's internal LOB binder.
// The returned slice shares Blob's backing storage.
func (b Blob) OracleBlobValue() []byte {
	return b
}
