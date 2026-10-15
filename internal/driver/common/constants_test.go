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

// TestConstants_Protocol checks parsing of protocol names
// expectations:
//   - Invalid name raises errors.
//   - tcp and tcps get properly mapped
func TestConstants_Protocol(t *testing.T) {
	t.Parallel()
	var p Protocol
	var err error

	_, err = NormalizeProtocol("")
	if err == nil {
		t.Errorf("should have receive an error for empty string")
	}
	_, err = NormalizeProtocol("XXXx")
	if err == nil {
		t.Errorf("should have receive an error for non-protocol string")
	}

	p, _ = NormalizeProtocol("tcp")
	if p != ProtocolTCP {
		t.Errorf("wrong protocol value for [%s], wanted [%s], got [%s]", "tcp", ProtocolTCP, p)
	}

	p, _ = NormalizeProtocol("tcps")
	if p != ProtocolTCPS {
		t.Errorf("wrong protocol value for [%s], wanted [%s], got [%s]", "tcps", ProtocolTCPS, p)
	}

	p, _ = NormalizeProtocol("TcP")
	if p != ProtocolTCP {
		t.Errorf("wrong protocol value for [%s], wanted [%s], got [%s]", "TcP", ProtocolTCP, p)
	}
	p, _ = NormalizeProtocol("TCPS")
	if p != ProtocolTCPS {
		t.Errorf("wrong protocol value for [%s], wanted [%s], got [%s]", "TCPS", ProtocolTCPS, p)
	}

}

// TestConstants_Protocol checks protocol names  constant mappings
// expectations:
//   - ProtocolTCP and ProtocolTCPS and mapped to tcp and tcps
func TestConstants_ProtocolString(t *testing.T) {
	t.Parallel()
	if strings.Compare(ProtocolTCP.String(), protocolName[ProtocolTCP]) != 0 {
		t.Errorf("wrong protocol string value  for [%s], wanted [%s], got [%s]",
			ProtocolTCP, protocolName[ProtocolTCP],
			ProtocolTCP.String())
	}
	if strings.Compare(ProtocolTCPS.String(), protocolName[ProtocolTCPS]) != 0 {
		t.Errorf("wrong protocol string value  for [%s], wanted [%s], got [%s]",
			ProtocolTCPS, protocolName[ProtocolTCPS],
			ProtocolTCPS.String())
	}
}
