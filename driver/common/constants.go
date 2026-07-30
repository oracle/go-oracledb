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
	"context"
	"errors"
	"fmt"
	"slices"
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
const DriverVersion = "26.0.0-beta"
const DriverName = "oracledb"
const MaxIdentifierLength = 128

type Protocol int

const (
	ProtocolTCP Protocol = iota
	ProtocolTCPS
)

var protocolName = map[Protocol]string{
	ProtocolTCP:  "TCP",
	ProtocolTCPS: "TCPS",
}

var BackgroundContext context.Context = context.Background()

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

type LogonMode int64

const (
	// KpzLogonSysdba indicates SYSDBA privilege logon
	KpzLogonSysdba LogonMode = 0x00000020
	// KpzLogonSysoper indicates SYSOPER privilege logon
	KpzLogonSysoper LogonMode = 0x00000040
)

var logonModeName = map[LogonMode]string{
	KpzLogonSysdba:  "SYSDBA",
	KpzLogonSysoper: "SYSOPER",
}
var logonModeValues = map[string]LogonMode{
	"SYSDBA":  KpzLogonSysdba,
	"SYSOPER": KpzLogonSysoper,
}

// Stringer interface
func (p LogonMode) String() string {
	return logonModeName[p]
}

// Value return int64 version of this mode
func (p LogonMode) Value() int64 { return int64(p) }

// Enabled checks that this mode is set in the given mode
func (p LogonMode) Enabled(mode int64) bool {
	return p.Value()&mode > 0
}

var _allLogonModeNames = strings.Join(_getLogonModes(), ",")

// _getLogonModes gets logon mode name list
// returns:
//   - the list
func _getLogonModes() []string {
	return slices.Collect(func(yield func(string) bool) {
		for k, _ := range logonModeValues {
			if !yield(k) {
				return
			}
		}
	})
}

// GetLogonModeFromString parses a string as a logon mode
// returns:
//   - the logon mode
//
// error:
//   - the given string does not map to a logonmode.
func GetLogonModeFromString(strmode string) (LogonMode, error) {
	var mode = strings.ToUpper(strings.TrimSpace(strmode))
	if l, ok := logonModeValues[mode]; ok {
		return l, nil
	}
	return KpzLogonSysdba, NewOracleError(InvalidConnectionParameter, nil, mode, "logonMode", _allLogonModeNames)
}
