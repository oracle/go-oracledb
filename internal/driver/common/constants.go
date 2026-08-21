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

package common

import (
	"errors"
	"fmt"
	"strings"
)

// The driver vesrion has to be sent to the server as a number, the
// formula to convert from string to number is:
//
// Version string = "V.RU.RUR.Inc.Ext
//
// Version number = V<<24 | RU<<16 | RUR<<12 | Inc<<4 | Ext
//
// where V is the version number,
// RU is the Release Update,
// RUR is the Release Update Revision
// Inc is the release update Increment, and
// Ext is the extension (currently unused).
//
// In go we only have the first 3 values V.RU.RUR, Inc and Ext will be 0.
var NumericDriverVersion = StringToB1Array("436207616") // 26<<24 | 0<<16 | 0<<12 | 0<<4 | 0

// ByteOrder defines endianness for multi-byte reads/writes.
type ByteOrder struct {
	name string
}

var (
	BIG_ENDIAN    = ByteOrder{name: "BIG_ENDIAN"}
	LITTLE_ENDIAN = ByteOrder{name: "LITTLE_ENDIAN"}
)

type Protocol int

const (
	ProtocolTCP Protocol = iota
	ProtocolTCPS
)

var protocolName = map[Protocol]string{
	ProtocolTCP:  "TCP",
	ProtocolTCPS: "TCPS",
}

func (p Protocol) String() string {
	return protocolName[p]
}

// NormalizeProtocol parses a supported protocol name expressed as string
//
//	Supported values are "TCP" and "TCPS".
//
// inputs:
//   - protocol as string.
//
// outputs:
//   - normalized value boolean
//
// errors:
//   - invalid value
func NormalizeProtocol(ps string) (Protocol, error) {
	normalized := strings.ToLower(strings.TrimSpace(ps))
	if normalized == "tcp" {
		return ProtocolTCP, nil
	}
	if normalized == "tcps" {
		return ProtocolTCPS, nil
	}
	return ProtocolTCP, errors.New(fmt.Sprintf("Invalid boolean value [%s]", ps)) // TODO : use ORA error for this.
}
