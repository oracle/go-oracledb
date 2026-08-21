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
** is contributed by such licensors),
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

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// Tests that a marshal engine can be created and that it implements the
// Marshaller interface.
func TestNewMarshalEngine(t *testing.T) {
	t.Parallel()
	dataBuffer := NewArrayDataBuffer(1) // Assuming DataBuffer is properly initialized
	byteOrder := common.BIG_ENDIAN
	engine := NewNativeMarshalEngine(dataBuffer, byteOrder)
	if engine == nil {
		t.Errorf("NewMarshalEngine returned nil")
	}
	// Check that the interface is implemented
	marshaller := common.Marshaller(engine)
	if marshaller == nil {
		t.Errorf(("Marshaller is null"))
	}
}

// Tests that a UB1 is written correctly for all values of UB1.
func TestMarshalUB1(t *testing.T) {
	t.Parallel()
	dataBuffer := NewArrayDataBuffer(1024)
	byteOrder := common.BIG_ENDIAN
	engine := NewMarshalEngine(dataBuffer, byteOrder, [5]byte{Native, Native, Native, Native, Native})
	for i := 0; i < 256; i++ {
		err := engine.MarshalUB1(context.Background(), common.UB1(i))
		if err != nil {
			t.Errorf("MarshalUB1 failed: %v", err)
		}
		if dataBuffer.bytes[dataBuffer.currentWritePosition-1] != byte(i) {
			t.Errorf("Invalid value was: %d but should be: %d", dataBuffer.bytes[dataBuffer.currentWritePosition-1], i)
		}
	}
}

// Tests that different values of UB2, containing different number of bytes, are
// written correctly in both BIG_ENDIAN and LITTLE_ENDIAN byte order and NATIVE
// and universal encoding
func TestMarshalUB2(t *testing.T) {
	t.Parallel()
	fmt.Printf("--------- Testing NATIVE and BIG_ENDIAN \n")
	values := map[common.UB2][]byte{
		0x0000: {0x00, 0x00},
		0x0001: {0x00, 0x01},
		0x00F4: {0x00, 0xF4},
		0x01F4: {0x01, 0xF4},
		0xF3F4: {0xF3, 0xF4},
	}
	dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Native, 150000)
	runMarshalTest(dataBuffer, values, engine.MarshalUB2, t)

	fmt.Printf("--------- Testing NATIVE and LITTLE_ENDIAN \n")
	values = map[common.UB2][]byte{
		0x0000: {0x00, 0x00},
		0x0001: {0x01, 0x00},
		0x00F4: {0xF4, 0x00},
		0x01F4: {0xF4, 0x01},
		0xF3F4: {0xF4, 0xF3},
	}
	dataBuffer, engine = newMarshalEngine(common.LITTLE_ENDIAN, B2, Native, 150000)
	runMarshalTest(dataBuffer, values, engine.MarshalUB2, t)

	fmt.Printf("--------- Testing UNIVERSAL and BIG_ENDIAN \n")
	values = map[common.UB2][]byte{
		0x0000: {0x00},
		0x0001: {0x01, 0x01},
		0x00F4: {0x01, 0xF4},
		0x01F4: {0x02, 0x01, 0xF4},
		0xF3F4: {0x02, 0xF3, 0xF4},
	}
	dataBuffer, engine = newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 150000)
	runMarshalTest(dataBuffer, values, engine.MarshalUB2, t)

	fmt.Printf("--------- Testing UNIVERSAL and LITTLE_ENDIAN \n")
	// UNIVERSAL representation is always BIG_ENDIAN
	dataBuffer, engine = newMarshalEngine(common.LITTLE_ENDIAN, B2, Universal, 150000)
	runMarshalTest(dataBuffer, values, engine.MarshalUB2, t)
}

// Tests that different values of SB4, containing different number of bytes, are
// written correctly in both BIG_ENDIAN and LITTLE_ENDIAN byte order and NATIVE
// and universal encoding
func TestMarshalSB4(t *testing.T) {
	t.Parallel()
	fmt.Printf("--------- Testing SB4 NATIVE and BIG_ENDIAN \n")
	values := map[common.SB4][]byte{
		0x00000000: {0x00, 0x00, 0x00, 0x00},
		0x00000001: {0x00, 0x00, 0x00, 0x01},
		0x000000F4: {0x00, 0x00, 0x00, 0xF4},
		0x000001F4: {0x00, 0x00, 0x01, 0xF4},
		0x0000F3F4: {0x00, 0x00, 0xF3, 0xF4},
		0x00E3F3F4: {0x00, 0xE3, 0xF3, 0xF4},
		0x5544F3F4: {0x55, 0x44, 0xF3, 0xF4},
		-12258316:  {0xFF, 0x44, 0xF3, 0xF4}, //0xFF44F3F4
	}
	dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, B4, Native, 150000)
	runMarshalTest(dataBuffer, values, engine.MarshalSB4, t)

	fmt.Printf("--------- Testing SB4 NATIVE and LITTLE_ENDIAN \n")
	values = map[common.SB4][]byte{
		0x00000000: {0x00, 0x00, 0x00, 0x00},
		0x00000001: {0x01, 0x00, 0x00, 0x00},
		0x000000F4: {0xF4, 0x00, 0x00, 0x00},
		0x000001F4: {0xF4, 0x01, 0x00, 0x00},
		0x0000F3F4: {0xF4, 0xF3, 0x00, 0x00},
		0x00E3F3F4: {0xF4, 0xF3, 0xE3, 0x00},
		0x5544F3F4: {0xF4, 0xF3, 0x44, 0x55},
		-12258316:  {0xF4, 0xF3, 0x44, 0xFF}, //0xFF44F3F4
	}
	dataBuffer, engine = newMarshalEngine(common.LITTLE_ENDIAN, B4, Native, 150000)
	runMarshalTest(dataBuffer, values, engine.MarshalSB4, t)

	fmt.Printf("--------- Testing SB4 UNIVERSAL and BIG_ENDIAN \n")
	values = map[common.SB4][]byte{
		0x00000000:  {0x00},
		0x00000001:  {0x01, 0x01},
		0x000000F4:  {0x01, 0xF4},
		0x000001F4:  {0x02, 0x01, 0xF4},
		0x0000F3F4:  {0x02, 0xF3, 0xF4},
		0x00E3F3F4:  {0x03, 0xE3, 0xF3, 0xF4},
		0x5544F3F4:  {0x04, 0x55, 0x44, 0xF3, 0xF4},
		-1:          {0x81, 0x01},
		-244:        {0x81, 0xF4},
		-500:        {0x82, 0x01, 0xF4},
		-62452:      {0x82, 0xF3, 0xF4},
		-14939124:   {0x83, 0xE3, 0xF3, 0xF4},
		-1430582260: {0x84, 0x55, 0x44, 0xF3, 0xF4},
	}
	dataBuffer, engine = newMarshalEngine(common.BIG_ENDIAN, B4, Universal, 150000)
	runMarshalTest(dataBuffer, values, engine.MarshalSB4, t)

	fmt.Printf("--------- Testing SB4 UNIVERSAL and LITTLE_ENDIAN \n")
	// UNIVERSAL representation is always BIG_ENDIAN
	dataBuffer, engine = newMarshalEngine(common.LITTLE_ENDIAN, B4, Universal, 150000)
	runMarshalTest(dataBuffer, values, engine.MarshalSB4, t)
}

// Tests that different values of UB4, containing different number of bytes, are
// written correctly in both BIG_ENDIAN and LITTLE_ENDIAN byte order and NATIVE
// and universal encoding
func TestMarshalUB4(t *testing.T) {
	t.Parallel()
	fmt.Printf("--------- Testing UB4 NATIVE and BIG_ENDIAN \n")
	values := map[common.UB4][]byte{
		0x00000000: {0x00, 0x00, 0x00, 0x00},
		0x00000001: {0x00, 0x00, 0x00, 0x01},
		0x000000F4: {0x00, 0x00, 0x00, 0xF4},
		0x000001F4: {0x00, 0x00, 0x01, 0xF4},
		0x0000F3F4: {0x00, 0x00, 0xF3, 0xF4},
		0x00E3F3F4: {0x00, 0xE3, 0xF3, 0xF4},
		0x5544F3F4: {0x55, 0x44, 0xF3, 0xF4},
		0xFF44F3F4: {0xFF, 0x44, 0xF3, 0xF4},
	}
	dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, B4, Native, 150000)
	runMarshalTest(dataBuffer, values, engine.MarshalUB4, t)

	fmt.Printf("--------- Testing UB4 NATIVE and LITTLE_ENDIAN \n")
	values = map[common.UB4][]byte{
		0x00000000: {0x00, 0x00, 0x00, 0x00},
		0x00000001: {0x01, 0x00, 0x00, 0x00},
		0x000000F4: {0xF4, 0x00, 0x00, 0x00},
		0x000001F4: {0xF4, 0x01, 0x00, 0x00},
		0x0000F3F4: {0xF4, 0xF3, 0x00, 0x00},
		0x00E3F3F4: {0xF4, 0xF3, 0xE3, 0x00},
		0x5544F3F4: {0xF4, 0xF3, 0x44, 0x55},
		0xFF44F3F4: {0xF4, 0xF3, 0x44, 0xFF},
	}
	dataBuffer, engine = newMarshalEngine(common.LITTLE_ENDIAN, B4, Native, 150000)
	runMarshalTest(dataBuffer, values, engine.MarshalUB4, t)

	fmt.Printf("--------- Testing UB4 UNIVERSAL and BIG_ENDIAN \n")
	values = map[common.UB4][]byte{
		0x00000000: {0x00},
		0x00000001: {0x01, 0x01},
		0x000000F4: {0x01, 0xF4},
		0x000001F4: {0x02, 0x01, 0xF4},
		0x0000F3F4: {0x02, 0xF3, 0xF4},
		0x00E3F3F4: {0x03, 0xE3, 0xF3, 0xF4},
		0x5544F3F4: {0x04, 0x55, 0x44, 0xF3, 0xF4},
		0xFF44F3F4: {0x04, 0xFF, 0x44, 0xF3, 0xF4},
	}
	dataBuffer, engine = newMarshalEngine(common.BIG_ENDIAN, B4, Universal, 150000)
	runMarshalTest(dataBuffer, values, engine.MarshalUB4, t)

	fmt.Printf("--------- Testing UB4 UNIVERSAL and LITTLE_ENDIAN \n")
	// UNIVERSAL representation is always BIG_ENDIAN
	dataBuffer, engine = newMarshalEngine(common.LITTLE_ENDIAN, B4, Universal, 150000)
	runMarshalTest(dataBuffer, values, engine.MarshalUB4, t)
}

// Tests that different values of UB4, containing different number of bytes, are
// written correctly in both BIG_ENDIAN and LITTLE_ENDIAN byte order and NATIVE
// and universal encoding
func TestMarshalUB8(t *testing.T) {
	t.Parallel()
	fmt.Printf("--------- Testing UB4 NATIVE and BIG_ENDIAN \n")
	values := map[common.UB8][]byte{
		0x0000000000000000: {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		0x0000000000000001: {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
		0x00000000000000F4: {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF4},
		0x00000000000001F4: {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0xF4},
		0x000000000000F3F4: {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF3, 0xF4},
		0x0000000000E3F3F4: {0x00, 0x00, 0x00, 0x00, 0x00, 0xE3, 0xF3, 0xF4},
		0x000000005544F3F4: {0x00, 0x00, 0x00, 0x00, 0x55, 0x44, 0xF3, 0xF4},
		0x00000000FF44F3F4: {0x00, 0x00, 0x00, 0x00, 0xFF, 0x44, 0xF3, 0xF4},
		0x00000025FF44F3F4: {0x00, 0x00, 0x00, 0x25, 0xFF, 0x44, 0xF3, 0xF4},
		0x00000525FF44F3F4: {0x00, 0x00, 0x05, 0x25, 0xFF, 0x44, 0xF3, 0xF4},
		0x00100525FF44F3F4: {0x00, 0x10, 0x05, 0x25, 0xFF, 0x44, 0xF3, 0xF4},
		0xD2100525FF44F3F4: {0xD2, 0x10, 0x05, 0x25, 0xFF, 0x44, 0xF3, 0xF4},
	}
	dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, B4, Native, 150000)
	runMarshalTest(dataBuffer, values, engine.MarshalUB8, t)

	fmt.Printf("--------- Testing UB4 NATIVE and LITTLE_ENDIAN \n")
	values = map[common.UB8][]byte{
		0x0000000000000000: {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		0x0000000000000001: {0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		0x00000000000000F4: {0xF4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		0x00000000000001F4: {0xF4, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		0x000000000000F3F4: {0xF4, 0xF3, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		0x0000000000E3F3F4: {0xF4, 0xF3, 0xE3, 0x00, 0x00, 0x00, 0x00, 0x00},
		0x000000005544F3F4: {0xF4, 0xF3, 0x44, 0x55, 0x00, 0x00, 0x00, 0x00},
		0x00000000FF44F3F4: {0xF4, 0xF3, 0x44, 0xFF, 0x00, 0x00, 0x00, 0x00},
		0x00000025FF44F3F4: {0xF4, 0xF3, 0x44, 0xFF, 0x25, 0x00, 0x00, 0x00},
		0x00000525FF44F3F4: {0xF4, 0xF3, 0x44, 0xFF, 0x25, 0x05, 0x00, 0x00},
		0x00100525FF44F3F4: {0xF4, 0xF3, 0x44, 0xFF, 0x25, 0x05, 0x10, 0x00},
		0xD2100525FF44F3F4: {0xF4, 0xF3, 0x44, 0xFF, 0x25, 0x05, 0x10, 0xD2},
	}
	dataBuffer, engine = newMarshalEngine(common.LITTLE_ENDIAN, B4, Native, 150000)
	runMarshalTest(dataBuffer, values, engine.MarshalUB8, t)

	fmt.Printf("--------- Testing UB4 UNIVERSAL and BIG_ENDIAN \n")
	values = map[common.UB8][]byte{
		0x0000000000000000: {0x00},
		0x0000000000000001: {0x01, 0x01},
		0x00000000000000F4: {0x01, 0xF4},
		0x00000000000001F4: {0x02, 0x01, 0xF4},
		0x000000000000F3F4: {0x02, 0xF3, 0xF4},
		0x0000000000E3F3F4: {0x03, 0xE3, 0xF3, 0xF4},
		0x000000005544F3F4: {0x04, 0x55, 0x44, 0xF3, 0xF4},
		0x00000000FF44F3F4: {0x04, 0xFF, 0x44, 0xF3, 0xF4},
		0x00000025FF44F3F4: {0x05, 0x25, 0xFF, 0x44, 0xF3, 0xF4},
		0x00000525FF44F3F4: {0x06, 0x05, 0x25, 0xFF, 0x44, 0xF3, 0xF4},
		0x00100525FF44F3F4: {0x07, 0x10, 0x05, 0x25, 0xFF, 0x44, 0xF3, 0xF4},
		0xD2100525FF44F3F4: {0x08, 0xD2, 0x10, 0x05, 0x25, 0xFF, 0x44, 0xF3, 0xF4},
	}
	dataBuffer, engine = newMarshalEngine(common.BIG_ENDIAN, B4, Universal, 150000)
	runMarshalTest(dataBuffer, values, engine.MarshalUB8, t)

	fmt.Printf("--------- Testing UB4 UNIVERSAL and LITTLE_ENDIAN \n")
	// UNIVERSAL representation is always BIG_ENDIAN
	dataBuffer, engine = newMarshalEngine(common.LITTLE_ENDIAN, B4, Universal, 150000)
	runMarshalTest(dataBuffer, values, engine.MarshalUB8, t)
}

// Tests that B1Array is marshalled correctly
func TestMarshalByteArray(t *testing.T) {
	t.Parallel()
	var values = []common.B1Array{
		{0x00},
		{0x02, 0xF3, 0xF4},
		{0x04, 0x55, 0x44, 0xF3, 0xF4},
		{0x06, 0x55, 0x44, 0xF3, 0xF4, 0x01, 0xF4},
		{0x08, 0x85, 0x44, 0xF3, 0xF4, 0x01, 0xF4, 0x62, 0xD3},
	}
	dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, B8, Universal, 150000)
	for _, value := range values {
		engine.MarshalB1Array(context.Background(), value)
		length := len(value)
		for j := 0; j < length; j++ {
			if dataBuffer.bytes[dataBuffer.currentWritePosition-(length-j)] != value[j] {
				t.Errorf("Invalid value at byte %d was: %d but should be: %d", j, dataBuffer.bytes[dataBuffer.currentWritePosition-(length-j)], value[j])
			}
		}
	}
}

// Tests that Char is marshalled correctly
func TestMarshalChar(t *testing.T) {
	t.Parallel()
	var values = []common.B1Array{
		{0x00},
		{0x02, 0xF3, 0xF4},
		{0x04, 0x55, 0x44, 0xF3, 0xF4},
		{0x06, 0x55, 0x44, 0xF3, 0xF4, 0x01, 0xF4},
		{0x08, 0x85, 0x44, 0xF3, 0xF4, 0x01, 0xF4, 0x62, 0xD3},
	}
	dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, B8, Universal, 150000)
	for _, value := range values {
		engine.MarshalChar(context.Background(), value)
		length := len(value)
		for j := 0; j < length; j++ {
			if dataBuffer.bytes[dataBuffer.currentWritePosition-(length-j)] != value[j] {
				t.Errorf("Invalid value at byte %d was: %d but should be: %d", j, dataBuffer.bytes[dataBuffer.currentWritePosition-(length-j)], value[j])
			}
		}
	}
}

// Tests that PTR is correctly encoded in both UNIVERSAL and NATIVE encoding.
func TestMarshalPTR(t *testing.T) {
	t.Parallel()
	_, engine := newMarshalEngine(common.BIG_ENDIAN, PTR, Universal, 150000)
	engine.MarshalPTR(context.Background())
	value, err := engine.UnmarshalUB1(context.Background())
	if err != nil {
		t.Errorf("Error unmarshalling pointer %v", err)
	}
	if value != 0x01 {
		t.Errorf("Wrong pointer value")
	}
	_, engine = newMarshalEngine(common.BIG_ENDIAN, PTR, Native, 150000)
	engine.MarshalPTR(context.Background())
	b1Array, err2 := engine.UnmarshalB1Array(context.Background(), 4)
	if err2 != nil {
		t.Errorf("Error unmarshalling pointer %v", err2)
	}
	for _, value := range b1Array {
		if value != 0x7F {
			t.Errorf("Wrong pointer value expected 0x7F but was %d", value)
		}
	}
}

// Tests that null PTR is correctly encoded in both UNIVERSAL and NATIVE
// encoding.
func TestMarshalNullPTR(t *testing.T) {
	t.Parallel()
	dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, PTR, Universal, 150000)
	engine.MarshalNullPTR(context.Background())
	dataBuffer.currentReadPosition = 0
	value, err := engine.UnmarshalUB1(context.Background())
	if err != nil {
		t.Errorf("Error unmarshalling null pointer %v", err)
	}
	if value != 0x00 {
		t.Errorf("Wrong null pointer value")
	}
	_, engine = newMarshalEngine(common.BIG_ENDIAN, PTR, Native, 150000)
	engine.MarshalNullPTR(context.Background())
	b1Array, err2 := engine.UnmarshalB1Array(context.Background(), 4)
	if err2 != nil {
		t.Errorf("Error unmarshalling pointer %v", err2)
	}
	for _, value := range b1Array {
		if value != 0x00 {
			t.Errorf("Wrong pointer value expected 0x00 but was %d", value)
		}
	}
}

// Tests that CLR is correct encoded with values smaller or larger than
// _maximumShortValueLength
func TestMarshalCLR(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		length int
		offset int
	}{
		{"short length", 10, 0},
		{"length equal to TTCC_MXIN", _checkSize, 0},
		{"length greater than TTCC_MXIN", _checkSize + 1, 0},
		{"with offset", 10, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataBuffer, engine := newMarshalEngine(common.LITTLE_ENDIAN, B4, Native, 150000)
			value := make(common.B1Array, tt.length)
			for i := range value {
				value[i] = byte(i % 256) // Fill with some data
			}

			err := engine.MarshalCLR(context.Background(), value, tt.offset, tt.length-tt.offset)
			if err != nil {
				t.Errorf("MarshalCLR failed: %v", err)
			}
			if tt.length <= int(_maximumShortValueLength) {
				expectedLength := (tt.length - tt.offset) + 1
				if dataBuffer.currentWritePosition != expectedLength {
					t.Errorf("currentPosition mismatch: got %d, want %d", dataBuffer.currentWritePosition, expectedLength)
				}
				// Verify the length byte
				if dataBuffer.bytes[0] != byte(tt.length-tt.offset) {
					t.Errorf("length byte mismatch: got %d, want %d", dataBuffer.bytes[0], tt.length)
				}
				// Verify the data
				for i := 0; i < tt.length-tt.offset; i++ {
					if dataBuffer.bytes[i+1] != value[(tt.offset+i)] {
						t.Errorf("data mismatch at index %d: got %d, want %d", i, dataBuffer.bytes[i+1], value[tt.offset+i])
					}
				}
			} else {
				// For large data, verify the escape byte and chunking
				if dataBuffer.bytes[0] != _longLengthIndicator {
					t.Errorf("escape byte mismatch: got %d, want %d", dataBuffer.bytes[0], _longLengthIndicator)
				}
				// More complex verification for chunked data
				pos := 1
				dataRead := 0
				remaining := tt.length - tt.offset
				for remaining > 0 {
					chunkSize := remaining
					if chunkSize > _checkSize {
						chunkSize = _checkSize
					}
					// Verify the 4-byte chunk length
					chunkLen := binary.LittleEndian.Uint32(dataBuffer.bytes[pos : pos+4])
					if int(chunkLen) != chunkSize {
						t.Errorf("chunk length mismatch: got %d, want %d", chunkLen, chunkSize)
					}
					pos += 4
					// Verify the chunk data
					for i := 0; i < chunkSize; i++ {
						if dataBuffer.bytes[pos+i] != value[(tt.offset+dataRead+i)] {
							t.Errorf("chunk data mismatch at index %d: got %d, want %d", i, dataBuffer.bytes[pos+i], value[(tt.offset+dataRead+i)])
						}
					}
					pos += chunkSize
					dataRead += chunkSize
					remaining -= chunkSize
				}
				// Verify the terminating zero byte
				if dataBuffer.bytes[pos] != 0 {
					t.Errorf("terminating zero byte mismatch: got %d, want 0", dataBuffer.bytes[pos])
				}
			}
		})
	}
}

// Tests that data buffer flush method is called when marshaller flush method
// is called
func TestFlush(t *testing.T) {
	t.Parallel()
	dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 10)
	dataBuffer.Flush(context.Background())
	err := engine.Flush(context.Background())
	if !dataBuffer.hasBeenFlushed {
		t.Errorf("DataBuffer Flush() was not called")
	}
	if err != nil {
		t.Errorf("Should not have failed [%v]", err)
	}
	dataBuffer.returnFlushError = true
	err = engine.Flush(context.Background())
	if err == nil {
		t.Errorf("Should have failed")
	}
	if sqlError, ok := err.(oracleErrors.SQLError); ok {
		if sqlError.ErrorCode() != string(oracleErrors.MarshalEngineFlushError) {
			t.Fatalf("Wrong error code, should be %s but was %s", oracleErrors.MarshalEngineFlushError, sqlError.ErrorCode())
		}
	} else {
		t.Fatalf("Error should be instance of SQLError")
	}
}

// Marshalls and unmarshalls UB1 to check that unmarshalled values correspond to
// marshalled values
func TestUnmarshalUB1(t *testing.T) {
	t.Parallel()
	fmt.Printf("--------- Testing BIG_ENDIAN \n")
	dataBuffer := NewArrayDataBuffer(1024)
	byteOrder := common.BIG_ENDIAN
	engine := NewMarshalEngine(dataBuffer, byteOrder, [5]byte{Native, Native, Native, Native, Native})
	for i := 0; i < 256; i++ {
		err := engine.MarshalUB1(context.Background(), common.UB1(i))
		if err != nil {
			t.Errorf("MarshalUB1 failed: %v", err)
		}
	}
	dataBuffer.currentReadPosition = 0
	for i := 0; i < 256; i++ {
		value, err := engine.UnmarshalUB1(context.Background())
		if err != nil {
			t.Errorf("MarshalUB1 failed: %v", err)
		}
		if i != int(value) {
			t.Errorf("Wrong value expected %d, but was %d", i, value)
		}
	}
}

// Marshalls and unmarshalls UB2 to check that unmarshalled values correspond to
// marshalled values
func TestUnmarshalUB2(t *testing.T) {
	t.Parallel()
	fmt.Printf("--------- Testing NATIVE and BIG_ENDIAN \n")
	values := []common.UB2{0x0000, 0x0001, 0x00F4, 0x01F4, 0xF3F4}

	tests := []struct {
		name           string
		byteOrder      common.ByteOrder
		valueType      byte
		representation byte
		size           int
	}{
		{
			name:           "Testing NATIVE and BIG_ENDIAN",
			byteOrder:      common.BIG_ENDIAN,
			valueType:      B2,
			representation: Native,
			size:           150000,
		},
		{
			name:           "Testing NATIVE and LITTLE_ENDIAN",
			byteOrder:      common.LITTLE_ENDIAN,
			valueType:      B2,
			representation: Native,
			size:           150000,
		},
		{
			name:           "Testing UNIVERSAL and BIG_ENDIAN",
			byteOrder:      common.BIG_ENDIAN,
			valueType:      B2,
			representation: Universal,
			size:           150000,
		},
		{
			name:           "Testing UNIVERSAL and LITTLE_ENDIAN",
			byteOrder:      common.LITTLE_ENDIAN,
			valueType:      B2,
			representation: Universal,
			size:           150000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fmt.Printf("--------- %s", tt.name)
			dataBuffer, engine := newMarshalEngine(tt.byteOrder, tt.valueType, tt.representation, tt.size)

			for _, value := range values {
				engine.MarshalUB2(context.Background(), value)
			}
			dataBuffer.currentReadPosition = 0
			for _, value := range values {
				readValue, err := engine.UnmarshalUB2(context.Background())
				if err != nil {
					t.Errorf("UnmarshalUB2 failed: %v", err)
				}
				if readValue != value {
					t.Errorf("Wrong value expected %d, but was %d", value, readValue)
				}
			}
		})
	}
}

// Marshalls and unmarshalls SB4 to check that unmarshalled values correspond to
// marshalled values
func TestUnmarshalSB4(t *testing.T) {
	t.Parallel()
	fmt.Printf("--------- Testing NATIVE and BIG_ENDIAN \n")
	values := []common.SB4{0x00000000, 0x00000001, 0x000000F4, 0x000001F4,
		0x0000F3F4, 0x00E3F3F4, 0x5544F3F4, -12258316}

	tests := []struct {
		name           string
		byteOrder      common.ByteOrder
		valueType      byte
		representation byte
		size           int
	}{
		{
			name:           "Testing NATIVE and BIG_ENDIAN",
			byteOrder:      common.BIG_ENDIAN,
			valueType:      B4,
			representation: Native,
			size:           150000,
		},
		{
			name:           "Testing NATIVE and LITTLE_ENDIAN",
			byteOrder:      common.LITTLE_ENDIAN,
			valueType:      B4,
			representation: Native,
			size:           150000,
		},
		{
			name:           "Testing UNIVERSAL and BIG_ENDIAN",
			byteOrder:      common.BIG_ENDIAN,
			valueType:      B4,
			representation: Universal,
			size:           150000,
		},
		{
			name:           "Testing UNIVERSAL and LITTLE_ENDIAN",
			byteOrder:      common.LITTLE_ENDIAN,
			valueType:      B4,
			representation: Universal,
			size:           150000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fmt.Printf("--------- %s", tt.name)
			dataBuffer, engine := newMarshalEngine(tt.byteOrder, tt.valueType, tt.representation, tt.size)

			for _, value := range values {
				engine.MarshalSB4(context.Background(), value)
			}
			dataBuffer.currentReadPosition = 0
			for _, value := range values {
				readValue, err := engine.UnmarshalSB4(context.Background())
				if err != nil {
					t.Errorf("UnmarshalUB4 failed: %v", err)
				}
				if readValue != value {
					t.Errorf("Wrong value expected %d, but was %d", value, readValue)
				}
			}
		})
	}
}

func TestUnmarshalSB1(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		byteOrder      common.ByteOrder
		valueType      byte
		representation byte
		wire           common.B1Array
		want           common.SB1
	}{
		{
			name:           "native big endian wraps from SB2",
			byteOrder:      common.BIG_ENDIAN,
			valueType:      B2,
			representation: Native,
			wire:           common.B1Array{0x00, 0xFF},
			want:           -1,
		},
		{
			name:           "native little endian wraps from SB2",
			byteOrder:      common.LITTLE_ENDIAN,
			valueType:      B2,
			representation: Native,
			wire:           common.B1Array{0xFF, 0x00},
			want:           -1,
		},
		{
			name:           "universal casts from signed path",
			byteOrder:      common.BIG_ENDIAN,
			valueType:      B2,
			representation: Universal,
			wire:           common.B1Array{0x01, 0xFF},
			want:           -1,
		},
		{
			name:           "universal negative flag",
			byteOrder:      common.BIG_ENDIAN,
			valueType:      B2,
			representation: Universal,
			wire:           common.B1Array{0x81, 0x01},
			want:           -1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, engine := newMarshalEngine(test.byteOrder, test.valueType, test.representation, 1000)
			if err := engine.MarshalB1Array(context.Background(), test.wire); err != nil {
				t.Fatalf("MarshalB1Array failed: %v", err)
			}

			readValue, err := engine.UnmarshalSB1(context.Background())
			if err != nil {
				t.Fatalf("UnmarshalSB1 failed: %v", err)
			}
			if readValue != test.want {
				t.Errorf("Wrong value expected %d, but was %d", test.want, readValue)
			}
		})
	}
}

func TestUnmarshalSB2(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		byteOrder      common.ByteOrder
		valueType      byte
		representation byte
		wire           common.B1Array
		want           common.SB2
	}{
		{
			name:           "native big endian",
			byteOrder:      common.BIG_ENDIAN,
			valueType:      B2,
			representation: Native,
			wire:           common.B1Array{0xFF, 0xFE},
			want:           -2,
		},
		{
			name:           "native little endian",
			byteOrder:      common.LITTLE_ENDIAN,
			valueType:      B2,
			representation: Native,
			wire:           common.B1Array{0xFE, 0xFF},
			want:           -2,
		},
		{
			name:           "universal negative flag",
			byteOrder:      common.BIG_ENDIAN,
			valueType:      B2,
			representation: Universal,
			wire:           common.B1Array{0x81, 0x02},
			want:           -2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, engine := newMarshalEngine(test.byteOrder, test.valueType, test.representation, 1000)
			if err := engine.MarshalB1Array(context.Background(), test.wire); err != nil {
				t.Fatalf("MarshalB1Array failed: %v", err)
			}

			readValue, err := engine.UnmarshalSB2(context.Background())
			if err != nil {
				t.Fatalf("UnmarshalSB2 failed: %v", err)
			}
			if readValue != test.want {
				t.Errorf("Wrong value expected %d, but was %d", test.want, readValue)
			}
		})
	}
}

func TestUnmarshalSB2UniversalRangeValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tests := []struct {
		name    string
		wire    common.B1Array
		want    common.SB2
		wantErr bool
	}{
		{
			name:    "positive overflow",
			wire:    common.B1Array{0x02, 0x80, 0x00},
			wantErr: true,
		},
		{
			name:    "negative underflow",
			wire:    common.B1Array{0x82, 0x80, 0x01},
			wantErr: true,
		},
		{
			name: "min int16",
			wire: common.B1Array{0x82, 0x80, 0x00},
			want: common.SB2(math.MinInt16),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 1000)
			if err := engine.MarshalB1Array(ctx, test.wire); err != nil {
				t.Fatalf("MarshalB1Array failed: %v", err)
			}

			readValue, err := engine.UnmarshalSB2(ctx)
			if test.wantErr {
				if err == nil {
					t.Fatalf("UnmarshalSB2 should have failed")
				}
				checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "SB2")
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalSB2 failed: %v", err)
			}
			if readValue != test.want {
				t.Errorf("Unexpected value. Expected: %d. Value: %d", test.want, readValue)
			}
		})
	}
}

func TestUnmarshalUnsignedUniversalRejectsNegativeFlag(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tests := []struct {
		name      string
		valueType byte
		decode    func(context.Context, *MarshalEngine) (any, error)
	}{
		{
			name:      "UB2",
			valueType: B2,
			decode: func(ctx context.Context, engine *MarshalEngine) (any, error) {
				return engine.UnmarshalUB2(ctx)
			},
		},
		{
			name:      "UB4",
			valueType: B4,
			decode: func(ctx context.Context, engine *MarshalEngine) (any, error) {
				return engine.UnmarshalUB4(ctx)
			},
		},
		{
			name:      "UB8",
			valueType: B8,
			decode: func(ctx context.Context, engine *MarshalEngine) (any, error) {
				return engine.UnmarshalUB8(ctx)
			},
		},
	}

	wire := common.B1Array{0x81, 0x01}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, engine := newMarshalEngine(common.BIG_ENDIAN, test.valueType, Universal, 1000)
			if err := engine.MarshalB1Array(ctx, wire); err != nil {
				t.Fatalf("MarshalB1Array failed: %v", err)
			}

			_, err := test.decode(ctx, engine)
			if err == nil {
				t.Fatalf("expected %s universal decode to fail for negative flag", test.name)
			}
			checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), test.name)
		})
	}
}

// Marshalls and unmarshalls UB4 to check that unmarshalled values correspond to
// marshalled values
func TestUnmarshalUB4(t *testing.T) {
	t.Parallel()
	fmt.Printf("--------- Testing NATIVE and BIG_ENDIAN \n")
	values := []common.UB4{0x00000000, 0x00000001, 0x000000F4, 0x000001F4,
		0x0000F3F4, 0x00E3F3F4, 0x5544F3F4, 0xFF44F3F4}

	tests := []struct {
		name           string
		byteOrder      common.ByteOrder
		valueType      byte
		representation byte
		size           int
	}{
		{
			name:           "Testing NATIVE and BIG_ENDIAN",
			byteOrder:      common.BIG_ENDIAN,
			valueType:      B4,
			representation: Native,
			size:           150000,
		},
		{
			name:           "Testing NATIVE and LITTLE_ENDIAN",
			byteOrder:      common.LITTLE_ENDIAN,
			valueType:      B4,
			representation: Native,
			size:           150000,
		},
		{
			name:           "Testing UNIVERSAL and BIG_ENDIAN",
			byteOrder:      common.BIG_ENDIAN,
			valueType:      B4,
			representation: Universal,
			size:           150000,
		},
		{
			name:           "Testing UNIVERSAL and LITTLE_ENDIAN",
			byteOrder:      common.LITTLE_ENDIAN,
			valueType:      B4,
			representation: Universal,
			size:           150000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fmt.Printf("--------- %s", tt.name)
			dataBuffer, engine := newMarshalEngine(tt.byteOrder, tt.valueType, tt.representation, tt.size)

			for _, value := range values {
				engine.MarshalUB4(context.Background(), value)
			}
			dataBuffer.currentReadPosition = 0
			for _, value := range values {
				readValue, err := engine.UnmarshalUB4(context.Background())
				if err != nil {
					t.Errorf("UnmarshalUB4 failed: %v", err)
				}
				if readValue != value {
					t.Errorf("Wrong value expected %d, but was %d", value, readValue)
				}
			}
		})
	}
}

// Marshalls and unmarshalls UB8 to check that unmarshalled values correspond to
// marshalled values
func TestUnmarshalUB8(t *testing.T) {
	t.Parallel()
	fmt.Printf("--------- Testing NATIVE and BIG_ENDIAN \n")
	values := []common.UB8{
		0x0000000000000000, 0x0000000000000001, 0x00000000000000F4,
		0x00000000000001F4, 0x000000000000F3F4, 0x0000000000E3F3F4,
		0x000000005544F3F4, 0x00000000FF44F3F4, 0x00000025FF44F3F4,
		0x00000525FF44F3F4, 0x00100525FF44F3F4, 0xD2100525FF44F3F4,
	}

	tests := []struct {
		name           string
		byteOrder      common.ByteOrder
		valueType      byte
		representation byte
		size           int
	}{
		{
			name:           "Testing NATIVE and BIG_ENDIAN",
			byteOrder:      common.BIG_ENDIAN,
			valueType:      B8,
			representation: Native,
			size:           150000,
		},
		{
			name:           "Testing NATIVE and LITTLE_ENDIAN",
			byteOrder:      common.LITTLE_ENDIAN,
			valueType:      B8,
			representation: Native,
			size:           150000,
		},
		{
			name:           "Testing UNIVERSAL and BIG_ENDIAN",
			byteOrder:      common.BIG_ENDIAN,
			valueType:      B8,
			representation: Universal,
			size:           150000,
		},
		{
			name:           "Testing UNIVERSAL and LITTLE_ENDIAN",
			byteOrder:      common.LITTLE_ENDIAN,
			valueType:      B8,
			representation: Universal,
			size:           150000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fmt.Printf("--------- %s", tt.name)
			dataBuffer, engine := newMarshalEngine(tt.byteOrder, tt.valueType, tt.representation, tt.size)

			for _, value := range values {
				engine.MarshalUB8(context.Background(), value)
			}
			dataBuffer.currentReadPosition = 0
			for _, value := range values {
				readValue, err := engine.UnmarshalUB8(context.Background())
				if err != nil {
					t.Errorf("UnmarshalUB4 failed: %v", err)
				}
				if readValue != value {
					t.Errorf("Wrong value expected %d, but was %d", value, readValue)
				}
			}
		})
	}
}

// Marshalls and unmarshalls CLR to check that unmarshalled values correspond to
// marshalled values
// TODO: check the correct size of length in case of NATIVE encoding
func TestUnmarshalCLR(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		length int
		offset int
	}{
		{"short length", 10, 0},
		{"length equal to TTCC_MXIN", _checkSize, 0},
		{"length greater than TTCC_MXIN", _checkSize + 1, 0},
		{"with offset", 10, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataBuffer, engine := newMarshalEngine(common.LITTLE_ENDIAN, B4, Native, 150000)
			value := make([]byte, tt.length)
			for i := range value {
				value[i] = byte(i % 256) // Fill with some data
			}

			err := engine.MarshalCLR(context.Background(), (common.B1Array)(value), tt.offset, tt.length-tt.offset)
			if err != nil {
				t.Errorf("MarshalCLR failed: %v", err)
			}

			dataBuffer.currentReadPosition = 0
			readValue := make([]byte, tt.length-tt.offset)
			readLength, err := engine.UnmarshalCLR(context.Background(), (common.B1Array)(readValue), tt.length-tt.offset)
			if err != nil {
				t.Errorf("UnmarshalCLR failed: %v", err)
			}
			if readLength != (tt.length - tt.offset) {
				t.Errorf("Invalid length read, expected %d, but was %d", (tt.length - tt.offset), readLength)
			}
			for i := 0; i < (tt.length - tt.offset); i++ {
				if readValue[i] != value[tt.offset+i] {
					t.Errorf("Invalid value read, expected %d, but was %d", value[tt.offset+i], readValue[i])
				}
			}
		})
	}
}

func TestUnmarshalCLRColumnData(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		length int
		offset int
	}{
		{"short length", 10, 0},
		{"length equal to TTCC_MXIN", _checkSize, 0},
		{"length greater than TTCC_MXIN", _checkSize + 1, 0},
		{"with offset", 10, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataBuffer, engine := newMarshalEngine(common.LITTLE_ENDIAN, B4, Native, 150000)
			value := make([]byte, tt.length)
			for i := range value {
				value[i] = byte(i % 256) // Fill with some data
			}

			err := engine.MarshalCLR(context.Background(), (common.B1Array)(value), tt.offset, tt.length-tt.offset)
			if err != nil {
				t.Errorf("MarshalCLR failed: %v", err)
			}

			dataBuffer.currentReadPosition = 0
			readValue, readLength, err := engine.UnmarshalCLRColumnData(context.Background())
			if err != nil {
				t.Errorf("UnmarshalCLR failed: %v", err)
			}
			if readLength != (tt.length - tt.offset) {
				t.Errorf("Invalid length read, expected %d, but was %d", (tt.length - tt.offset), readLength)
			}
			for i := 0; i < (tt.length - tt.offset); i++ {
				if readValue[i] != value[tt.offset+i] {
					t.Errorf("Invalid value read, expected %d, but was %d", value[tt.offset+i], readValue[i])
				}
			}
		})
	}
}

func TestUnmarshalCLRColumnDataRejectsInvalidLongChunkLengths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		length common.SB4
	}{
		{"negative chunk length", -1},
		{"chunk length exceeds maximum", common.SB4(_maximumLongCLRChunkLength + 1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, B4, Native, 16)
			if err := engine.MarshalUB1(context.Background(), common.UB1(_longLengthIndicator)); err != nil {
				t.Fatalf("MarshalUB1 failed: %v", err)
			}
			if err := engine.MarshalSB4(context.Background(), tt.length); err != nil {
				t.Fatalf("MarshalSB4 failed: %v", err)
			}

			dataBuffer.currentReadPosition = 0
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("UnmarshalCLRColumnData panicked for %s: %v", tt.name, r)
				}
			}()

			_, _, err := engine.UnmarshalCLRColumnData(context.Background())
			if err == nil {
				t.Fatalf("UnmarshalCLRColumnData should have rejected %s", tt.name)
			}
			checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "CLR")
		})
	}
}

func TestUnmarshalCLRColumnDataRejectsAggregateLengthAboveMaximum(t *testing.T) {
	t.Parallel()
	fullChunks := _maximumLongCLRAggregateLength / _maximumLongCLRChunkLength
	finalLength := (_maximumLongCLRAggregateLength % _maximumLongCLRChunkLength) + 1
	dataBuffer := &aggregateLimitDataBuffer{
		chunkLength: _maximumLongCLRChunkLength,
		fullChunks:  fullChunks,
		finalLength: finalLength,
	}
	engine := NewMarshalEngine(dataBuffer, common.BIG_ENDIAN, [5]byte{Native, Native, Native, Native, Native})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("UnmarshalCLRColumnData panicked for aggregate length above maximum: %v", r)
		}
	}()

	_, _, err := engine.UnmarshalCLRColumnData(context.Background())
	if err == nil {
		t.Fatal("UnmarshalCLRColumnData should have rejected aggregate length above maximum")
	}
	checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "CLR")
}

type aggregateLimitDataBuffer struct {
	chunkLength      int
	fullChunks       int
	finalLength      int
	readInitial      bool
	pendingChunkData bool
	currentChunkLen  int
	chunksSent       int
}

func (b *aggregateLimitDataBuffer) WriteByteWithContext(context.Context, byte) error {
	return nil
}

func (b *aggregateLimitDataBuffer) WriteBytesWithContext(context.Context, []byte) error {
	return nil
}

func (b *aggregateLimitDataBuffer) ReadByteWithContext(context.Context) (byte, error) {
	if b.readInitial {
		return 0, fmt.Errorf("unexpected byte read")
	}
	b.readInitial = true
	return _longLengthIndicator, nil
}

func (b *aggregateLimitDataBuffer) ReadBytesWithContext(_ context.Context, length int32) (*[]byte, error) {
	if b.pendingChunkData {
		if int(length) != b.currentChunkLen {
			return nil, fmt.Errorf("unexpected chunk data length %d, want %d", length, b.currentChunkLen)
		}
		b.pendingChunkData = false
		return new(make([]byte, int(length))), nil
	}

	if length != 4 {
		return nil, fmt.Errorf("unexpected chunk length field size %d", length)
	}

	if b.chunksSent < b.fullChunks {
		b.currentChunkLen = b.chunkLength
	} else {
		b.currentChunkLen = b.finalLength
	}
	b.chunksSent++
	b.pendingChunkData = true

	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, uint32(b.currentChunkLen))
	return &data, nil
}

func (b *aggregateLimitDataBuffer) Flush(context.Context) error {
	return nil
}

// Marshalls and unmarshalls B1Array to check that unmarshalled values
// correspond to marshalled values
func TestUnmarshalByteArray(t *testing.T) {
	t.Parallel()
	var values = []common.B1Array{
		{0x00},
		{0x02, 0xF3, 0xF4},
		{0x04, 0x55, 0x44, 0xF3, 0xF4},
		{0x06, 0x55, 0x44, 0xF3, 0xF4, 0x01, 0xF4},
		{0x08, 0x85, 0x44, 0xF3, 0xF4, 0x01, 0xF4, 0x62, 0xD3},
	}
	// write and read one by one
	for _, value := range values {
		dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, B8, Universal, 150000)
		engine.MarshalB1Array(context.Background(), value)
		length := len(value)
		dataBuffer.currentReadPosition = 0
		readValue, err := engine.UnmarshalB1Array(context.Background(), length)
		if err != nil {
			t.Errorf("An error occurred while reading values %v", err)
		}

		for j := 0; j < length; j++ {
			if value[j] != (readValue)[j] {
				t.Errorf("Wrong value read, expected %d, but was %d", value[j], (readValue)[j])
			}
		}
	}

	// write all values
	dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, B8, Universal, 150000)
	for _, value := range values {
		engine.MarshalB1Array(context.Background(), value)
	}

	// read all values
	dataBuffer.currentReadPosition = 0
	for _, value := range values {
		length := len(value)
		readValue, err := engine.UnmarshalB1Array(context.Background(), length)
		if err != nil {
			t.Errorf("An error occurred while reading values %v", err)
		}

		for j := 0; j < length; j++ {
			if value[j] != (readValue)[j] {
				t.Errorf("Wrong value read, expected %d, but was %d", value[j], (readValue)[j])
			}
		}
	}
}

// Unmarshalls Text and checks that the unmarshalled values are correct
func TestUnmarshalText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    []byte
		length   int
		expected string
	}{
		{
			name:     "null-terminated text",
			input:    []byte{'H', 'e', 'l', 'l', 'o', 0, 'w', 'o', 'r', 'l', 'd'},
			length:   6,
			expected: "Hello",
		},
		{
			name:     "text without null terminator",
			input:    []byte{'H', 'e', 'l', 'l', 'o'},
			length:   5,
			expected: "Hello",
		},
		{
			name:     "empty input",
			input:    []byte{},
			length:   0,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataBuffer := NewArrayDataBuffer(len(tt.input) + 1)
			err := dataBuffer.WriteBytesWithContext(context.Background(), tt.input)
			if err != nil {
				t.Errorf("WriteBytes failed: %v", err)
			}
			dataBuffer.currentReadPosition = 0
			byteOrder := common.BIG_ENDIAN
			engine := NewMarshalEngine(dataBuffer, byteOrder, [5]byte{Native, Native, Native, Native, Native})

			result, err := engine.UnmarshalText(context.Background(), tt.length)
			if err != nil {
				t.Errorf("UnmarshalText failed: %v", err)
			}
			if err == nil && string(result) != tt.expected {
				t.Errorf("UnmarshalText returned %q, want %q", string(result), tt.expected)
			}
		})
	}
}

// Marshalls and unmarshalls key-value pairs to check that unmarshalled values
// correspond to marshalled values
func TestUnmarshalKeyValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		keyValuePairs *keyValueList
	}{
		{
			name:          "valid key-value pairs",
			keyValuePairs: _newKeyValPairs(),
		},
		{
			name:          "empty keys and values",
			keyValuePairs: _newEmptyKeyValPairs(),
		},
		{
			name:          "empty keys and values",
			keyValuePairs: _newNilKeyValPairs(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, 0, 0, 150000)

			err := ((common.Marshallable)(tt.keyValuePairs)).MarshalTo(context.Background(), engine)
			if err != nil {
				t.Errorf("MarshalKeyValue failed: %v", err)
			}

			dataBuffer.currentReadPosition = 0
			returnedKeyValuePairs := newPreallocatedKeyValueList(tt.keyValuePairs.Len())
			err = ((common.UnMarshallable)(returnedKeyValuePairs)).UnMarshalFrom(context.Background(), engine)
			if err != nil {
				t.Errorf("Error unmarshalling key value %v", err)
			}
			if returnedKeyValuePairs.Len() != tt.keyValuePairs.Len() {
				t.Errorf("Wrong number of flags, expected %d, but was %d", tt.keyValuePairs.Len(), returnedKeyValuePairs.Len())
			}
			returnedKv := returnedKeyValuePairs.Front()
			for kv := tt.keyValuePairs.Front(); kv != nil; kv = kv.Next() {
				if returnedKv.Value.(*common.KeyValue).Flag != kv.Value.(*common.KeyValue).Flag {
					t.Errorf("Wrong flags, expected %d, but was %d", kv.Value.(*common.KeyValue).Flag, returnedKv.Value.(*common.KeyValue).Flag)
				}
				_rkv := returnedKv.Value.(*common.KeyValue)
				_kv := kv.Value.(*common.KeyValue)
				if !_rkv.Equals(_kv) {
					t.Errorf("Wrong keyValue, expected %d, but was %d", returnedKv.Value.(*common.KeyValue), kv.Value.(*common.KeyValue))
				}
				returnedKv = returnedKv.Next()
			}
		})
	}
}

// Error case: writes a UB1 on a buffer that is not big enough and expects error
func TestMarshalUB1NoSpace(t *testing.T) {
	t.Parallel()
	fmt.Printf("--------- Testing BIG_ENDIAN \n")
	dataBuffer := NewArrayDataBuffer(0)
	byteOrder := common.BIG_ENDIAN
	engine := NewMarshalEngine(dataBuffer, byteOrder, [5]byte{Native, Native, Native, Native, Native})
	for i := 0; i < 256; i++ {
		err := engine.MarshalUB1(context.Background(), common.UB1(i))
		if err == nil {
			t.Errorf("MarshalUB1 should have failed")
		}
		checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "UB1")
	}
}

// Error case: writes a CLR on a buffer that is not big enough and expects error
func TestMarshalCLRNoSpace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		length       int
		offset       int
		bufferLength int
	}{
		{"short length buffer size 0", 10, 0, 0},
		{"short length buffer size 1", 10, 0, 1},
		{"short length buffer size length", 10, 0, 10},
		{"length equal to TTCC_MXIN size 0", _checkSize, 0, 0},
		{"length equal to TTCC_MXIN size 1", _checkSize, 0, 1},
		{"length equal to TTCC_MXIN size 3 (cannot read length)", _checkSize, 0, 3},
		{"length equal to TTCC_MXIN size length", _checkSize, 0, _checkSize},
		{"length greater than TTCC_MXIN size 0", _checkSize + 1, 0, 0},
		{"length greater than TTCC_MXIN size 1", _checkSize + 1, 0, 1},
		{"length greater than TTCC_MXIN size length", _checkSize + 1, 0, _checkSize},
		{"length greater than TTCC_MXIN size length + 11", _checkSize + 1, 0, _checkSize + 11},
		{"length greater than TTCC_MXIN size length + 13", _checkSize + 1, 0, _checkSize + 13},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataBuffer, engine := newMarshalEngine(common.LITTLE_ENDIAN, B4, Native, tt.bufferLength)
			value := make(common.B1Array, tt.length)
			for i := range value {
				value[i] = byte(i % 256) // Fill with some data
			}

			dataBuffer.currentWritePosition = 0
			dataBuffer.currentReadPosition = 0
			err := engine.MarshalCLR(context.Background(), value, tt.offset, tt.length-tt.offset)
			if err == nil {
				t.Fatalf("MarshalCLR should have failed")
			}
			if sqlError, ok := err.(oracleErrors.SQLError); ok {
				if sqlError.ErrorCode() != string(oracleErrors.MarshalEngineError) {
					t.Fatalf("Wrong error code, should be %s but was %s", oracleErrors.MarshalEngineError, sqlError.ErrorCode())
				}
				if !strings.Contains(sqlError.Error(), "CLR") {
					t.Fatalf("Error message should contain type CLR but was: %s", sqlError.Error())
				}
			} else {
				t.Fatalf("Error should be instance of SQLError")
			}
		})
	}
}

// Error case: writes a key-value on a buffer that is not big enough and expects
// error
func TestMarshalKeyValueNoSpace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		keyValuePairs *keyValueList
		numOfPairs    int
		bufferSize    int
	}{
		{
			name:          "valid key-value pairs size 0",
			keyValuePairs: _newKeyValPairs(),
			numOfPairs:    2,
			bufferSize:    0,
		},
		{
			name:          "valid key-value pairs size 1",
			keyValuePairs: _newKeyValPairs(),
			numOfPairs:    2,
			bufferSize:    1,
		},
		{
			name:          "valid key-value pairs size 3",
			keyValuePairs: _newKeyValPairs(),
			numOfPairs:    2,
			bufferSize:    3,
		},
		{
			name:          "valid key-value pairs size 7",
			keyValuePairs: _newKeyValPairs(),
			numOfPairs:    2,
			bufferSize:    7,
		},
		{
			name:          "valid key-value pairs size 9",
			keyValuePairs: _newKeyValPairs(),
			numOfPairs:    2,
			bufferSize:    9,
		},
		{
			name:          "valid key-value pairs size 16",
			keyValuePairs: _newKeyValPairs(),
			numOfPairs:    2,
			bufferSize:    16,
		},
		{
			name:          "empty keys and values",
			keyValuePairs: _newEmptyKeyValPairs(),
			numOfPairs:    2,
			bufferSize:    0,
		},
		{
			name:          "empty keys and values",
			keyValuePairs: _newEmptyKeyValPairs(),
			numOfPairs:    2,
			bufferSize:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, 0, 0, tt.bufferSize)
			err := ((common.Marshallable)(tt.keyValuePairs)).MarshalTo(context.Background(), engine)

			dataBuffer.currentWritePosition = 0
			dataBuffer.currentReadPosition = 0
			if err == nil {
				t.Fatalf("MarshalKeyValue should have failed")
			}
			if sqlError, ok := err.(oracleErrors.SQLError); ok {
				if sqlError.ErrorCode() != string(oracleErrors.MarshalEngineError) {
					t.Fatalf("Wrong error code, should be %s but was %s", oracleErrors.MarshalEngineError, sqlError.ErrorCode())
				}
				if !strings.Contains(sqlError.Error(), "Key/Value") {
					t.Fatalf("Error message should contain type Key/Value but was: %s", sqlError.Error())
				}
			} else {
				t.Fatalf("Error should be instance of SQLError")
			}
		})
	}
}

// Error case: unmarshalls a key-value on a buffer that only contains part of
// the data and expects error
func TestUnmarshalKeyValueNoSpace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		keyValuePairs *keyValueList
		numOfPairs    int
		writePosition int
	}{
		{
			name:          "valid key-value pairs size 0",
			keyValuePairs: _newKeyValPairs(),
			numOfPairs:    2,
			writePosition: 0,
		},
		{
			name:          "valid key-value pairs size 1",
			keyValuePairs: _newKeyValPairs(),
			numOfPairs:    2,
			writePosition: 1,
		},
		{
			name:          "valid key-value pairs size 3",
			keyValuePairs: _newKeyValPairs(),
			numOfPairs:    2,
			writePosition: 3,
		},
		{
			name:          "valid key-value pairs size 7",
			keyValuePairs: _newKeyValPairs(),
			numOfPairs:    2,
			writePosition: 7,
		},
		{
			name:          "valid key-value pairs size 9",
			keyValuePairs: _newKeyValPairs(),
			numOfPairs:    2,
			writePosition: 9,
		},
		{
			name:          "valid key-value pairs size 16",
			keyValuePairs: _newKeyValPairs(),
			numOfPairs:    2,
			writePosition: 16,
		},
		{
			name:          "empty keys and values",
			keyValuePairs: _newEmptyKeyValPairs(),
			numOfPairs:    2,
			writePosition: 0,
		},
		{
			name:          "empty keys and values",
			keyValuePairs: _newEmptyKeyValPairs(),
			numOfPairs:    2,
			writePosition: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, 0, 0, 1000)
			err := ((common.Marshallable)(tt.keyValuePairs)).MarshalTo(context.Background(), engine)
			if err != nil {
				t.Errorf("Failed to marshal")
			}
			dataBuffer.currentWritePosition = tt.writePosition
			dataBuffer.currentReadPosition = 0
			keyvalFlags := newPreallocatedKeyValueList(tt.keyValuePairs.Len())
			err = ((common.UnMarshallable)(keyvalFlags)).UnMarshalFrom(context.Background(), engine)
			if err == nil {
				t.Errorf("UnmarshalKeyValue should have failed")
			}
			if sqlError, ok := err.(oracleErrors.SQLError); ok {
				if sqlError.ErrorCode() != string(oracleErrors.MarshalEngineError) {
					t.Fatalf("Wrong error code, should be %s but was %s", oracleErrors.MarshalEngineError, sqlError.ErrorCode())
				}
				if !strings.Contains(sqlError.Error(), "Key/Value") {
					t.Fatalf("Error message should contain type Key/Value but was: %s", sqlError.Error())
				}
			} else {
				t.Fatalf("Error should be instance of SQLError")
			}
		})
	}
}

// Error case: unmarshalls an UB2 on a buffer that only contains part of
// the data and expects error
func TestUnmarshalUB2NoSpace(t *testing.T) {
	t.Parallel()
	var value common.UB2 = 1056
	dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 1000)
	engine.MarshalUB2(context.Background(), value)
	dataBuffer.currentWritePosition = 1
	_, err := engine.UnmarshalUB2(context.Background())
	if err == nil {
		t.Errorf("UnmarshalUB2 should have failed")
	}
	checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "UB2")
	dataBuffer, engine = newMarshalEngine(common.BIG_ENDIAN, B2, Native, 1000)
	engine.MarshalUB2(context.Background(), value)
	dataBuffer.currentWritePosition = 1
	_, err = engine.UnmarshalUB2(context.Background())
	if err == nil {
		t.Errorf("UnmarshalUB2 should have failed")
	}
	checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "UB2")
}

// Error case: unmarshalls an UB4 on a buffer that only contains part of
// the data and expects error
func TestUnmarshalUB4NoSpace(t *testing.T) {
	t.Parallel()
	var value common.UB4 = 1056
	dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, B4, Universal, 1000)
	engine.MarshalUB4(context.Background(), value)
	dataBuffer.currentWritePosition = 2
	_, err := engine.UnmarshalUB4(context.Background())
	if err == nil {
		t.Errorf("UnmarshalUB4 should have failed")
	}
	checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "UB4")
	dataBuffer, engine = newMarshalEngine(common.BIG_ENDIAN, B4, Native, 1000)
	engine.MarshalUB4(context.Background(), value)
	dataBuffer.currentWritePosition = 2
	_, err = engine.UnmarshalUB4(context.Background())
	if err == nil {
		t.Errorf("UnmarshalUB4 should have failed")
	}
	checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "UB4")
}

// Error case: data contain negative zero value which is not a possible value
func TestUnmarshalUB4WithNegativeZeroLength(t *testing.T) {
	t.Parallel()
	value := common.B1Array{0x80, 0x00, 0x01, 0x02}
	_, engine := newMarshalEngine(common.BIG_ENDIAN, B4, Universal, 1000)
	engine.MarshalB1Array(context.Background(), value)
	_, err := engine.UnmarshalUB4(context.Background())
	if err == nil {
		t.Errorf("UnmarshalUB4 should have failed")
	}
	checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "UB4")
}

func TestUnmarshalUniversalRejectsByteCountsOverTargetWidth(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tests := []struct {
		name    string
		typ     byte
		value   common.B1Array
		errType string
		decode  func(context.Context, *MarshalEngine) error
	}{
		{
			name:    "UB2",
			typ:     B2,
			value:   common.B1Array{0x03, 0x00, 0x00, 0x00},
			errType: "UB2",
			decode: func(ctx context.Context, engine *MarshalEngine) error {
				_, err := engine.UnmarshalUB2(ctx)
				return err
			},
		},
		{
			name:    "UB4",
			typ:     B4,
			value:   common.B1Array{0x05, 0x00, 0x00, 0x00, 0x00, 0x00},
			errType: "UB4",
			decode: func(ctx context.Context, engine *MarshalEngine) error {
				_, err := engine.UnmarshalUB4(ctx)
				return err
			},
		},
		{
			name:    "SB4",
			typ:     B4,
			value:   common.B1Array{0x05, 0x00, 0x00, 0x00, 0x00, 0x00},
			errType: "SB4",
			decode: func(ctx context.Context, engine *MarshalEngine) error {
				_, err := engine.UnmarshalSB4(ctx)
				return err
			},
		},
		{
			name:    "UB8",
			typ:     B8,
			value:   common.B1Array{0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			errType: "UB8",
			decode: func(ctx context.Context, engine *MarshalEngine) error {
				_, err := engine.UnmarshalUB8(ctx)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, engine := newMarshalEngine(common.BIG_ENDIAN, test.typ, Universal, 1000)
			if err := engine.MarshalB1Array(ctx, test.value); err != nil {
				t.Fatalf("MarshalB1Array failed: %v", err)
			}

			err := test.decode(ctx, engine)
			if err == nil {
				t.Fatalf("Unmarshal%s should have failed", test.name)
			}
			checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), test.errType)
		})
	}
}

func TestUnmarshalUniversalUnsignedRejectsNegativeEncodings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	value := common.B1Array{0x81, 0x01}
	tests := []struct {
		name    string
		typ     byte
		errType string
		decode  func(context.Context, *MarshalEngine) error
	}{
		{
			name:    "UB2",
			typ:     B2,
			errType: "UB2",
			decode: func(ctx context.Context, engine *MarshalEngine) error {
				_, err := engine.UnmarshalUB2(ctx)
				return err
			},
		},
		{
			name:    "UB4",
			typ:     B4,
			errType: "UB4",
			decode: func(ctx context.Context, engine *MarshalEngine) error {
				_, err := engine.UnmarshalUB4(ctx)
				return err
			},
		},
		{
			name:    "UB8",
			typ:     B8,
			errType: "UB8",
			decode: func(ctx context.Context, engine *MarshalEngine) error {
				_, err := engine.UnmarshalUB8(ctx)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, engine := newMarshalEngine(common.BIG_ENDIAN, test.typ, Universal, 1000)
			if err := engine.MarshalB1Array(ctx, value); err != nil {
				t.Fatalf("MarshalB1Array failed: %v", err)
			}

			err := test.decode(ctx, engine)
			if err == nil {
				t.Fatalf("Unmarshal%s should have failed", test.name)
			}
			checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), test.errType)
		})
	}
}

func TestUnmarshalSB4UniversalRangeValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tests := []struct {
		name    string
		value   common.B1Array
		want    common.SB4
		wantErr bool
	}{
		{
			name:    "positive overflow",
			value:   common.B1Array{0x04, 0x80, 0x00, 0x00, 0x00},
			wantErr: true,
		},
		{
			name:    "negative underflow",
			value:   common.B1Array{0x84, 0x80, 0x00, 0x00, 0x01},
			wantErr: true,
		},
		{
			name:  "min int32",
			value: common.B1Array{0x84, 0x80, 0x00, 0x00, 0x00},
			want:  common.SB4(math.MinInt32),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, engine := newMarshalEngine(common.BIG_ENDIAN, B4, Universal, 1000)
			if err := engine.MarshalB1Array(ctx, test.value); err != nil {
				t.Fatalf("MarshalB1Array failed: %v", err)
			}

			readValue, err := engine.UnmarshalSB4(ctx)
			if test.wantErr {
				if err == nil {
					t.Fatalf("UnmarshalSB4 should have failed")
				}
				checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "SB4")
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalSB4 failed: %v", err)
			}
			if readValue != test.want {
				t.Errorf("Unexpected value. Expected: %d. Value: %d", test.want, readValue)
			}
		})
	}
}

func TestDynamicAllocatedArrayRejectsNegativeUniversalLength(t *testing.T) {
	t.Parallel()
	value := common.B1Array{0x81, 0x01}
	_, engine := newMarshalEngine(common.BIG_ENDIAN, B4, Universal, 1000)
	if err := engine.MarshalB1Array(context.Background(), value); err != nil {
		t.Fatalf("MarshalB1Array failed: %v", err)
	}

	var dalc dynamicAllocatedArray
	err := dalc.UnMarshalFrom(context.Background(), engine)
	if err == nil {
		t.Fatalf("UnMarshalFrom should have failed")
	}
	checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "unmarshal value")
}

func TestMarshalUnmarshalUB8UsesB8Representation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("B8 universal with B4 native", func(t *testing.T) {
		dataBuffer := NewArrayDataBuffer(1000)
		engine := NewMarshalEngine(dataBuffer, common.BIG_ENDIAN, [5]byte{Native, Native, Native, Universal, Native})

		err := engine.MarshalUB8(ctx, 255)
		if err != nil {
			t.Fatalf("MarshalUB8 failed: %v", err)
		}
		if dataBuffer.currentWritePosition != 2 {
			t.Fatalf("MarshalUB8 wrote %d bytes, want 2", dataBuffer.currentWritePosition)
		}

		readValue, err := engine.UnmarshalUB8(ctx)
		if err != nil {
			t.Fatalf("UnmarshalUB8 failed: %v", err)
		}
		if readValue != 255 {
			t.Errorf("Unexpected value. Expected: 255. Value: %d", readValue)
		}
	})

	t.Run("B8 native with B4 universal", func(t *testing.T) {
		dataBuffer := NewArrayDataBuffer(1000)
		engine := NewMarshalEngine(dataBuffer, common.BIG_ENDIAN, [5]byte{Native, Native, Universal, Native, Native})

		err := engine.MarshalUB8(ctx, 255)
		if err != nil {
			t.Fatalf("MarshalUB8 failed: %v", err)
		}
		if dataBuffer.currentWritePosition != 8 {
			t.Fatalf("MarshalUB8 wrote %d bytes, want 8", dataBuffer.currentWritePosition)
		}

		readValue, err := engine.UnmarshalUB8(ctx)
		if err != nil {
			t.Fatalf("UnmarshalUB8 failed: %v", err)
		}
		if readValue != 255 {
			t.Errorf("Unexpected value. Expected: 255. Value: %d", readValue)
		}
	})
}

// Error case: unmarshalls an B1Array on a buffer that only contains part of
// the data and expects error
func TestUnmarshalUB1ArrayNoSpace(t *testing.T) {
	t.Parallel()
	var value common.B1Array = common.B1Array{128, 10, 12, 20}
	dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 1000)
	engine.MarshalB1Array(context.Background(), value)
	dataBuffer.currentWritePosition = 2
	_, err := engine.UnmarshalB1Array(context.Background(), len(value))
	if err == nil {
		t.Errorf("UnmarshalB1Array should have failed")
	}
	checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "B1Array")
}

// Error case: unmarshalls Text on a buffer that only contains part of
// the data and expects error
func TestUnmarshalTextNoSpace(t *testing.T) {
	t.Parallel()
	value := common.B1Array("test")
	dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 1000)
	engine.MarshalB1Array(context.Background(), value)
	dataBuffer.currentWritePosition = 1
	_, err := engine.UnmarshalText(context.Background(), len(value))
	if err == nil {
		t.Errorf("UnmarshalText should have failed")
	}
	checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "Text")
}

// Error case: unmarshalls CLR with escape flag and expects error
func TestUnmarshalCLREscapeValue(t *testing.T) {
	t.Parallel()
	value := common.B1Array{_espapeValue, 0x00, 0x01, 0x02}
	_, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 1000)
	engine.MarshalB1Array(context.Background(), value)
	readValue := make(common.B1Array, 4)
	_, err := engine.UnmarshalCLR(context.Background(), readValue, len(value))
	if err == nil {
		t.Errorf("UnmarshalCLR should have failed")
	}
	checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "CLR")
}

// Unmarshalls zero length CLR, checks correct length is returned
func TestUnmarshalCLRZeroLength(t *testing.T) {
	t.Parallel()
	value := common.B1Array{0x00, 0x00, 0x01, 0x02}
	_, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 1000)
	engine.MarshalB1Array(context.Background(), value)
	readValue := make(common.B1Array, 4)
	length, err := engine.UnmarshalCLR(context.Background(), readValue, len(value))
	if length != 0 {
		t.Errorf("UnmarshalCLR length should be 0 but was %d", length)
	}
	if err != nil {
		t.Errorf("UnmarshalCLR should not be an error")
	}
}

// Error case: unmarshalls CLR with null length indicator flag and expects zero
// length to be returned with no error
func TestUnmarshalCLRNullLengthIndicator(t *testing.T) {
	t.Parallel()
	value := common.B1Array{_nullLengthIndicator, 0x00, 0x01, 0x02}
	_, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 1000)
	engine.MarshalB1Array(context.Background(), value)
	readValue := make(common.B1Array, 4)
	readLength, err := engine.UnmarshalCLR(context.Background(), readValue, len(value))
	if err != nil {
		t.Errorf("UnmarshalCLR should not have failed %v", err)
	}
	if readLength != 0 {
		t.Errorf("UnmarshalCLR should read not values, bytes read: [%d]", readLength)
	}
}

// Error case: unmarshalls CLR with negative length value and expects error
func TestUnmarshalCLRNegativeLength(t *testing.T) {
	t.Parallel()
	_, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 1000)
	engine.MarshalUB1(context.Background(), common.UB1(_longLengthIndicator))
	engine.MarshalSB4(context.Background(), common.SB4(-1))
	readValue := make(common.B1Array, 10)
	_, err := engine.UnmarshalCLR(context.Background(), readValue, 10)
	if err == nil {
		t.Errorf("UnmarshalCLR should have failed")
	}
	checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "CLR")
}

// Error case: unmarshalls CLR with buffer containing part of the data only and
// expects error
func TestUnmarshalCLRNotEnoughBytesForLength(t *testing.T) {
	t.Parallel()
	_, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 1000)
	engine.MarshalUB1(context.Background(), common.UB1(_longLengthIndicator))
	value := common.B1Array{0x04, 0x77} // Invalid UB4
	engine.MarshalB1Array(context.Background(), value)
	readValue := make(common.B1Array, 10)
	_, err := engine.UnmarshalCLR(context.Background(), readValue, 10)
	if err == nil {
		t.Errorf("UnmarshalCLR should have failed")
	}
	checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "CLR")
}

// Error case: unmarshalls CLR with buffer containing part of the data only and
// expects error
func TestUnmarshalCLRBufferToSmallForLength(t *testing.T) {
	t.Parallel()
	_, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 1000)
	value := make(common.B1Array, 300)
	for i := 0; i < len(value); i++ {
		value[i] = byte(i % 256)
	}
	engine.MarshalCLR(context.Background(), value, 0, len(value))
	readValue := make(common.B1Array, 10)
	_, err := engine.UnmarshalCLR(context.Background(), readValue, len(value))
	if err == nil {
		t.Errorf("UnmarshalCLR should have failed")
	}
	checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "CLR")
}

// Unmarshalls CLR with buffer with max length shorter than the data length.
// Checks that correct length is returned.
func TestUnmarshalCLRReadLessThatTotalLength(t *testing.T) {
	t.Parallel()
	_, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 1000)
	value := make(common.B1Array, 300)
	for i := 0; i < len(value); i++ {
		value[i] = byte(i % 256)
	}
	engine.MarshalCLR(context.Background(), value, 0, len(value))
	readValue := make(common.B1Array, 250)
	readLength, err := engine.UnmarshalCLR(context.Background(), readValue, len(readValue))
	if err != nil {
		t.Errorf("Shouldn't have failed")
	}
	if readLength != 250 {
		t.Errorf("Invalid read length, expected 250 but was %d", readLength)
	}
}

// Unmarshalls CLR with buffer with buffer that is not long enough to contain
// the data. Check for expected errors.
func TestUnmarshalCLRReadLessThatTotalLengthNoSpace(t *testing.T) {
	t.Parallel()
	dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 1000)
	value := make(common.B1Array, 300)
	for i := 0; i < len(value); i++ {
		value[i] = byte(i % 256)
	}
	engine.MarshalCLR(context.Background(), value, 0, len(value))
	readValue := make(common.B1Array, 250)
	dataBuffer.currentWritePosition = 256
	_, err := engine.UnmarshalCLR(context.Background(), readValue, len(readValue))
	if err == nil {
		t.Errorf("Shouldn't have failed")
	}
	checkSQLErrorCodeContains(t, err, string(oracleErrors.MarshalEngineError), "CLR")
}

// Creates a marshal ending for testing
func newMarshalEngine(byteOrder common.ByteOrder, typ byte, rep byte, bufSize int) (*ArrayBasedDataBuffer, *MarshalEngine) {
	dataBuffer := NewArrayDataBuffer(bufSize)
	typeRep := newTypeRep()
	typeRep.setRep(typ, rep)
	engine := NewMarshalEngine(dataBuffer, byteOrder, [5]byte{rep, rep, rep, rep, rep})
	return dataBuffer, engine
}

// Runs numeric value writting tests
func runMarshalTest[T common.UB1 | common.UB2 | common.UB4 | common.SB4 | common.UB8](dataBuffer *ArrayBasedDataBuffer, values map[T][]byte, function func(context.Context, T) error, t *testing.T) {

	for k, v := range values {
		err := function(context.Background(), k)
		if err != nil {
			t.Errorf("MarshalUB2 failed: %v", err)
		}

		length := len(v)
		for j := 0; j < length; j++ {
			if dataBuffer.bytes[dataBuffer.currentWritePosition-(length-j)] != v[j] {
				t.Errorf("Invalid value at byte %d was: %d but should be: %d", j, dataBuffer.bytes[dataBuffer.currentWritePosition-(length-j)], v[j])
			}
		}
	}
}

// Marshals and unmarshals a DALC and checks that the sizes and values match
func TestMarshalDALC(t *testing.T) {
	t.Parallel()
	_, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 1000)
	var dalc dynamicAllocatedArray
	dalc.value = []byte{'T', 'e', 's', 't', ' ', 'm', 'a', 'r', 's', 'h', 'a', 'l', ' ', 'D', 'A', 'L', 'C'}
	dalc.MarshalTo(context.Background(), engine)

	var unMarshalledDalc dynamicAllocatedArray
	unMarshalledDalc.UnMarshalFrom(context.Background(), engine)

	if len(dalc.value) != len(unMarshalledDalc.value) {
		t.Fatalf("Invalid length expected %d, but was %d", len(dalc.value), len(unMarshalledDalc.value))
	}

	for i, value := range dalc.value {
		if value != unMarshalledDalc.value[i] {
			t.Fatalf("Invalid value at position %d, expected %d, but was %d", i, value, unMarshalledDalc.value[i])
		}
	}
}

// test basic iteration over sequence number.
// expectations: sequence numbers go from 1 to math.MaxInt8
func TestMarshalSequenceNumber(t *testing.T) {
	t.Parallel()
	dataBuffer := NewArrayDataBuffer(1024)
	byteOrder := common.BIG_ENDIAN
	engine := NewMarshalEngine(dataBuffer, byteOrder, [5]byte{Native, Native, Native, Native, Native})
	for i := 1; i <= math.MaxInt8; i++ {
		num, err := engine.marshalSeqNo(context.Background())
		if err != nil {
			t.Errorf("unexpected error while fetching sequence number [%v]", err)
		}
		if num != common.UB1(i) {
			t.Errorf("unexpected sequence number wanted [%d], got [%d]", i, num)
		}
	}
}

// test rotation of sequence number.
// expectations: sequence should go back to 1 after having reached math.MaxInt8
func TestMarshalSequenceNumberRotation(t *testing.T) {
	t.Parallel()
	dataBuffer := NewArrayDataBuffer(1024)
	byteOrder := common.BIG_ENDIAN
	engine := NewMarshalEngine(dataBuffer, byteOrder, [5]byte{Native, Native, Native, Native, Native})
	for i := 1; i <= math.MaxInt8; i++ {
		_, err := engine.marshalSeqNo(context.Background())
		if err != nil {
			t.Errorf("unexpected error while fetching sequence number [%v]", err)
		}
	}
	value, _ := engine.marshalSeqNo(context.Background())
	if value != 1 {
		t.Errorf("sequence number did not rotate as expected, should be 1, is [%d]", value)
	}
}

// test basic iteration over token number.
// expectations: token number alwasy 0
func TestMarshalTokenNumber(t *testing.T) {
	t.Parallel()
	dataBuffer := NewArrayDataBuffer(1024)
	byteOrder := common.BIG_ENDIAN
	engine := NewMarshalEngine(dataBuffer, byteOrder, [5]byte{Native, Native, Native, Native, Native})
	for i := 1; i <= 5; i++ {
		num, err := engine.marshalTokenNo(context.Background())
		if err != nil {
			t.Errorf("unexpected error while fetching token number [%v]", err)
		}
		if num != 0 {
			t.Errorf("unexpected token number wanted [%d], got [%d]", i, num)
		}
	}
}

func checkSQLErrorCodeContains(t *testing.T, err error, code, contains string) {
	if sqlError, ok := err.(oracleErrors.SQLError); ok {
		if sqlError.ErrorCode() != code {
			t.Fatalf("Wrong error code, should be %s but was %s", code, sqlError.ErrorCode())
		}
		if !strings.Contains(sqlError.Error(), contains) {
			t.Fatalf("Error message should contain type %s but was: %s", contains, sqlError.Error())
		}
	} else {
		t.Fatalf("Error should be instance of SQLError")
	}
}
