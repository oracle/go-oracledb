/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively, the "Software"), free of charge and under any and all copyright
** rights and under any and all patent rights owned or freely licensable by each
** licensor hereunder covering either (i) the unmodified Software as contributed
** to or provided by such licensor, or (ii) the Larger Works (as defined below),
** to deal in both the Software and any Larger Work, without restriction.
**
** This license is subject to the condition that the above copyright notice and
** this permission notice be included in all copies or substantial portions of
** the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 */

package ttc

import (
	"database/sql/driver"
	"encoding/base64"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	urowidPhysicalType    = 1 // Physical ROWID
	urowidLogicalType     = 2 // Logical (primary key) ROWID
	urowidRemoteType      = 3 // Remote (foreign) ROWID
	urowidExternalType    = 4 // external table ROWID
	urowidNotUsedType     = 5
	urowidNewExternalType = 6 // New external table ROWIDNew external table ROWID
)

// Universal ROWID component length definitions.
// layout is one type-indicator byte, then object (4 bytes), file (2), block (4), and slot (2)—13 bytes total.
const (
	urowidTypeOffset      = 0
	urowidTypeFieldLength = 1

	urowidObjectFieldOffset = urowidTypeFieldLength
	urowidObjectFieldLength = 4 // length of object number

	urowidFileFieldOffset = urowidObjectFieldOffset + urowidObjectFieldLength
	urowidFileFieldLength = 2 // length of file number

	urowidBlockFieldOffset = urowidFileFieldOffset + urowidFileFieldLength
	urowidBlockFieldLength = 4 // length of block number ;

	urowidSlotFieldOffset = urowidBlockFieldOffset + urowidBlockFieldLength
	urowidSlotFieldLength = 2 // length of slot number

	urowidComponentLength = urowidObjectFieldLength + urowidFileFieldLength + urowidBlockFieldLength + urowidSlotFieldLength + 1 // +1 for tyope indicator
)

var (
	// no prefix for physical
	urowidTypePrefix = [...]byte{'A', '*', '-', '(', ')'}
)

// DecodeUrowidColumn decodes a Oracle's internal universal rowid bytes.
//
//Description:
//
//	Converts Oracle's internal universal rowid bytes to the printable rowid representation returned by Oracle clients.
//
//Parameters:
//  - columnContext: Column metadata used for error reporting (name/index).
//  - data: Raw TTC payload bytes for the column.
//
//Returns:
//  - driver.Value: time.Time containing the decoded date/time value.
//  - error: Non-nil if decoding fails.

func DecodeUrowidColumn(columnContext columnContext, value any) (driver.Value, error) {
	data, err := rawColumnData(value)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	rowid, err := urowidToString(data)
	if err != nil {
		common.Odl.Debug("urowid column parse error", "err", err)
		return nil, rowDecodeError(columnContext, err, "UROWID")
	}
	return rowid, nil
}

/*
urowidToString converts an internal UROWID value to its printable representation.

Description:

	Uses the leading type indicator to convert physical rowid components into
	their fixed-width Base64 form. For logical and other nonphysical rowids, it
	prepends the type's printable prefix to the Base64 encoding of the remaining
	bytes. The input must contain at least the type indicator.

Parameters:
  - data: Internal UROWID bytes, beginning with the type indicator.

Returns:
  - string: Printable ROWID/UROWID value.
  - error: Non-nil if convertion failed
*/
func urowidToString(data []byte) (string, error) {
	rowidType := data[urowidTypeOffset]

	if rowidType == urowidPhysicalType {
		if len(data) != urowidComponentLength {
			return "", common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil,
				"UROWID", "Decode", driverCommon.ReasonInvalidLength, urowidComponentLength)
		}
		var rowid strings.Builder
		rowid.Grow(18)

		rowid.WriteString(base64.RawStdEncoding.EncodeToString(data[urowidObjectFieldOffset : urowidObjectFieldLength+1]))                      // object
		rowid.WriteString(base64.RawStdEncoding.EncodeToString(data[urowidFileFieldOffset : urowidFileFieldOffset+urowidFileFieldLength+1]))    // file
		rowid.WriteString(base64.RawStdEncoding.EncodeToString(data[urowidBlockFieldOffset : urowidBlockFieldOffset+urowidBlockFieldLength+1])) // block
		rowid.WriteString(base64.RawStdEncoding.EncodeToString(data[urowidSlotFieldOffset : urowidSlotFieldOffset+urowidSlotFieldLength+1]))    // slot

		return rowid.String(), nil
	}

	// all others cases go here.
	// For nonphysical UROWIDs, the format is a type-specific character prefix followed by Base64 of the remaining bytes
	body := base64.RawStdEncoding.EncodeToString(data[urowidTypeOffset+urowidTypeFieldLength:])
	return string(urowidTypePrefix[rowidType-2]) + body, nil
}
