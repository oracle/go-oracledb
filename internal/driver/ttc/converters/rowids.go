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

package converters

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"

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
	urowidBlockFieldLength = 4 // length of block number

	urowidSlotFieldOffset = urowidBlockFieldOffset + urowidBlockFieldLength
	urowidSlotFieldLength = 2 // length of slot number

	urowidComponentLength = urowidObjectFieldLength + urowidFileFieldLength + urowidBlockFieldLength + urowidSlotFieldLength + 1 // +1 for tyope indicator

	rowidDefRbaFieldOffset = 0
	rowidDefRbaFieldLength = 4

	rowidDefPartitionIdFieldOffset = rowidDefRbaFieldOffset + rowidDefRbaFieldLength
	rowidDefPartitionIdFieldLength = 2

	rowidDefTableIdFieldOffset = rowidDefPartitionIdFieldOffset + rowidDefPartitionIdFieldLength
	rowidDefTableIdFieldLength = 1

	rowidDefBlockNumberFieldOffset = rowidDefTableIdFieldOffset + rowidDefTableIdFieldLength
	rowidDefBlockNumberFieldLength = 4

	rowidDefSlotNumberFieldOffset = rowidDefBlockNumberFieldOffset + rowidDefBlockNumberFieldLength
	rowidDefSlotNumberFieldLength = 2

	rowidDefComponentLength = rowidDefRbaFieldLength + rowidDefPartitionIdFieldLength + rowidDefTableIdFieldLength + rowidDefBlockNumberFieldLength + rowidDefSlotNumberFieldLength
)

var (
	// IDS alphabet.
	rowidBase64Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

	// UROID prefix (not used for physical)
	urowidTypePrefix = [...]byte{'A', '*', '-', '(', ')'}
)

// appendKgrd42b converts an integer into a fixed-width sequence of base-64 characters and appends it to dst.
// parameters:
//   - dst: destiniation byte array
//   - value: the integer to convert
//   - digits: how many base-64 characters to append for that integer.
func appendKgrd42b(dst []byte, value uint64, digits int) []byte {
	start := len(dst)
	dst = append(dst, make([]byte, digits)...)

	for i := digits - 1; i >= 0; i-- {
		dst[start+i] = rowidBase64Alphabet[value&0x3f]
		value >>= 6
	}
	return dst
}

/*
DecodeRowidDef converts an internal ROWID value to its printable representation.

Description:

	data layout is the following:
	 [RBA 4 bytes][partitionID 2 bytes][blockNumber 4 bytes][slotNumber 2 bytes]

Parameters:
  - data: Internal ROWID bytes.

Returns:
  - string: Printable ROWID value.
  - error: Non-nil if conversion failed
*/
func DecodeRowidDef(data []byte) (string, error) {
	if len(data) != rowidDefComponentLength {
		return "", common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil,
			"ROWID", "Decode", driverCommon.ReasonInvalidLength, rowidDefComponentLength)
	}

	var rba = binary.BigEndian.Uint32(data[rowidDefRbaFieldOffset:rowidDefRbaFieldLength])
	var partitionID = binary.BigEndian.Uint16(data[rowidDefPartitionIdFieldOffset : rowidDefPartitionIdFieldOffset+rowidDefPartitionIdFieldLength])
	var blockNumber = binary.BigEndian.Uint32(data[rowidDefBlockNumberFieldOffset : rowidDefBlockNumberFieldOffset+rowidDefBlockNumberFieldLength])
	var slotNumber = binary.BigEndian.Uint16(data[rowidDefSlotNumberFieldOffset : rowidDefSlotNumberFieldLength+rowidDefSlotNumberFieldOffset])

	result := make([]byte, 0, 18)
	result = appendKgrd42b(result, uint64(rba), 6)
	result = appendKgrd42b(result, uint64(partitionID), 3)
	result = appendKgrd42b(result, uint64(blockNumber), 6)
	result = appendKgrd42b(result, uint64(slotNumber), 3)

	return string(result), nil
}

/*
DecodeUrowid converts an internal UROWID value to its printable representation.

Description:

	Uses the leading type indicator to convert physical rowid components into
	their fixed-width Base64 form. For logical and other nonphysical rowids, it
	prepends the type's printable prefix to the Base64 encoding of the remaining
	bytes. The input must contain at least the type indicator.

Parameters:
  - data: Internal UROWID bytes, beginning with the type indicator.

Returns:
  - string: Printable UROWID value.
  - error: Non-nil if conversion failed
*/
func DecodeUrowid(data []byte) (string, error) {
	rowidType := data[urowidTypeOffset]

	if rowidType == urowidPhysicalType {
		if len(data) != urowidComponentLength {
			return "", common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil,
				"UROWID", "Decode", driverCommon.ReasonInvalidLength, urowidComponentLength)
		}

		objectNumber := binary.BigEndian.Uint32(data[urowidObjectFieldOffset : urowidObjectFieldOffset+urowidObjectFieldLength])
		fileNumber := binary.BigEndian.Uint16(data[urowidFileFieldOffset : urowidFileFieldOffset+urowidFileFieldLength])
		blockNumber := binary.BigEndian.Uint32(data[urowidBlockFieldOffset : urowidBlockFieldOffset+urowidBlockFieldLength])
		slotNumber := binary.BigEndian.Uint16(data[urowidSlotFieldOffset : urowidSlotFieldOffset+urowidSlotFieldLength])

		// uses the restricted hexadecimal form when the object number is zero.
		if objectNumber == 0 {
			return fmt.Sprintf("%08X.%04X.%04X", blockNumber, slotNumber, fileNumber), nil
		}

		rowid := make([]byte, 0, 18)
		rowid = appendKgrd42b(rowid, uint64(objectNumber), 6)
		rowid = appendKgrd42b(rowid, uint64(fileNumber), 3)
		rowid = appendKgrd42b(rowid, uint64(blockNumber), 6)
		rowid = appendKgrd42b(rowid, uint64(slotNumber), 3)
		return string(rowid), nil
	}

	// For nonphysical UROWIDs, the format is a type-specific character prefix followed by Base64 of the remaining bytes.
	body := base64.RawStdEncoding.EncodeToString(data[urowidTypeOffset+urowidTypeFieldLength:])
	return string(urowidTypePrefix[rowidType-2]) + body, nil
}
