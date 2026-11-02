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
	"strings"
	"testing"
)

// TestConstants_GetLogonModeFromString checks parsing of string to protocol name.
// expectations:
//   - Invalid string raise error
//   - SYSOPER and SYSDBA properly mapped
func TestConstants_GetLogonModeFromString(t *testing.T) {
	t.Parallel()
	l, err := GetLogonModeFromString("SYSDBA")
	if err != nil {
		t.Errorf("expected no error for \"SYSDBA\", got: %v", err)
	}
	if l != KpzLogonSysdba {
		t.Errorf("wrong logon mode value for [%s], wanted [%s], got [%s]",
			"SYSDBA", KpzLogonSysdba,
			l)
	}
	l, err = GetLogonModeFromString("SYSOPER")
	if err != nil {
		t.Errorf("expected no error for \"SYSOPER\", got: %v", err)
	}
	if l != KpzLogonSysoper {
		t.Errorf("wrong logon mode value for [%s], wanted [%s], got [%s]",
			"SYSOPER", KpzLogonSysoper,
			l)
	}
	l, err = GetLogonModeFromString("XXXXXX")
	if err == nil {
		t.Errorf("expected an error for \"XXXXXX\", got nil")
	}

}

// TestConstants_LogonModeEnabled validate logon modes bitmask
//
//	expectations:
//	  - test varity of logon modes ans validates that associated bitmask ois correct
func TestConstants_LogonModeEnabled(t *testing.T) {
	t.Parallel()
	var mode LogonMode = 0
	if !KpzLogonSysdba.Enabled(KpzLogonSysdba.Value()) {
		t.Errorf("logon mode should be enabled")
	}
	if KpzLogonSysdba.Enabled(KpzLogonSysoper.Value()) {
		t.Errorf("logon mode should not be enabled")
	}
	if mode.Enabled(KpzLogonSysdba.Value()) {
		t.Errorf("logon mode should not be enabled")
	}
	mode |= KpzLogonSysdba
	if !mode.Enabled(KpzLogonSysdba.Value()) {
		t.Errorf("logon mode should be enabled")
	}
	mode |= KpzLogonSysoper
	if !mode.Enabled(KpzLogonSysoper.Value()) {
		t.Errorf("logon mode should be enabled")
	}
	if !mode.Enabled(int64(KpzLogonSysoper | KpzLogonSysdba)) {
		t.Errorf("logon mode should be enabled")
	}
}

// TestConstants_LogonModeString validate logon modes string values
//
//	expectations:
//	  - KpzLogonXXXX are actually mapped on correct names. i.e KpzLogonSysdba mapped to "SYSDBA"
func TestConstants_LogonModeString(t *testing.T) {
	t.Parallel()

	if strings.Compare(KpzLogonSysdba.String(), "SYSDBA") != 0 {
		t.Errorf("KpzLogonSysdba not mapped to \"SYSDBA\"")
	}
	if strings.Compare(KpzLogonSysoper.String(), "SYSOPER") != 0 {
		t.Errorf("KpzLogonSysoper not mapped to \"SYSOPER\"")
	}
}
