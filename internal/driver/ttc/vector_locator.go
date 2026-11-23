/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and any and all patent rights owned or freely
** licensable by each licensor hereunder covering either (i) the unmodified
** Software as contributed to or provided by such licensor, or (ii) the Larger
** Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed to or provided by such licensors),
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software and make,
** use, sell, offer for sale, import, export, have made, and have sold the
** Software and the Larger Work(s), and to sublicense the foregoing rights on
** either these or other terms.
**
** This license is subject to the following condition:
** The above copyright notice and either this complete permission notice or at
** a minimum a reference to the UPL must be included in all copies or
** substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

package ttc

import driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"

// buildVectorQuasiLocator builds a value-based BLOB quasi-locator for a VECTOR bind.
// payloadLength is the complete encoded VECTOR envelope length.
func buildVectorQuasiLocator(payloadLength int) driverCommon.B1Array {
	locatorSize := kolblTempLocatorMaxLength
	locator := make(driverCommon.B1Array, locatorSize)

	length := locatorSize - kolbLocatorLengthHeaderBytes
	locator[kolbLocatorLengthOffset] = byte(length >> 8)
	locator[kolbLocatorLengthOffset+1] = byte(length)

	locator[kolbVersionOffset] = 0
	locator[kolbVersionOffset+1] = quasiLocatorVersion

	locator[koll1FlagOffset] = (kolblBlobFlag | kolblValueBasedLocatorFlag) | kolblAbstractLocatorFlag
	locator[koll2FlagOffset] = kolblInitializedFlag
	locator[koll3FlagOffset] = 0
	locator[koll4FlagOffset] = 0

	// Byte width (BYTL) for binary payloads.
	locator[8] = 0
	locator[9] = 1

	lobLen := driverCommon.UB8(payloadLength)
	locator[10] = byte(lobLen >> 56)
	locator[11] = byte(lobLen >> 48)
	locator[12] = byte(lobLen >> 40)
	locator[13] = byte(lobLen >> 32)
	locator[14] = byte(lobLen >> 24)
	locator[15] = byte(lobLen >> 16)
	locator[16] = byte(lobLen >> 8)
	locator[17] = byte(lobLen)

	// Character set id remains zero for binary locators.
	locator[20] = 0
	locator[21] = 0

	return locator
}
