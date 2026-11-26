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
func buildVectorQuasiLocator(payloadLength int) (driverCommon.B1Array, error) {
	writer := driverCommon.NewGrowableWriteBufferWithCapacity(driverCommon.BIG_ENDIAN, kolblTempLocatorMaxLength)
	if err := writer.WriteUB2(driverCommon.UB2(kolblTempLocatorMaxLength - kolbLocatorLengthHeaderBytes)); err != nil {
		return nil, err
	}
	if err := writer.WriteUB2(quasiLocatorVersion); err != nil {
		return nil, err
	}
	if err := writer.WriteUB1(driverCommon.UB1(kolblBlobFlag | kolblValueBasedLocatorFlag | kolblAbstractLocatorFlag)); err != nil {
		return nil, err
	}
	if err := writer.WriteUB1(driverCommon.UB1(kolblInitializedFlag)); err != nil {
		return nil, err
	}
	if err := writer.WriteUB2(0); err != nil { // Remaining locator flag bytes.
		return nil, err
	}
	if err := writer.WriteUB2(1); err != nil { // Byte width for binary payloads.
		return nil, err
	}
	if err := writer.WriteUB8(driverCommon.UB8(payloadLength)); err != nil {
		return nil, err
	}
	// The remaining reserved bytes include the zero character-set ID.
	if _, err := writer.Reserve(kolblTempLocatorMaxLength - writer.Position()); err != nil {
		return nil, err
	}
	return writer.Bytes(), nil
}
