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
	"github.com/oracle/go-driver/driver/common"
)

// All TTC message code that must be kept ordered
const (
	// message types starts at '1'
	_ common.MessageType = iota
	TTIPRO
	TTIDTY
	TTIFUN
	TTIOER
	TTIAUA
	TTIRXH
	TTIRXD
	TTIRPA
	TTISTA
	TTINOER
	TTIIOV
	TTISLG
	TTIOAC
	TTILOBD
	TTIWRN
	TTIDCB
	TTIPFN
	TTI3GL
	TTIFOB
	TTISBND
	TTIBVC
	TTIEOB
	TTISPF
	TTIQC
	TTIRSH
	TTIONEWAYFN
	TTIIMPLRES
	TTIRENEG
	TTIDONE
	TTICOOKIE
	// unused
	_
	_
	TTITKN
	TTIINIT
)

// TTCMsgTypeName TTC message codes name map
var TTCMsgTypeName = map[common.MessageType]string{
	TTIPRO:      "TTIPRO",
	TTIDTY:      "TTIDTY",
	TTIFUN:      "TTIFUN",
	TTIOER:      "TTIOER",
	TTIAUA:      "TTIAUA",
	TTIRXH:      "TTIRXH",
	TTIRXD:      "TTIRXD",
	TTIRPA:      "TTIRPA",
	TTISTA:      "TTISTA",
	TTINOER:     "TTINOER",
	TTIIOV:      "TTIIOV",
	TTISLG:      "TTISLG",
	TTIOAC:      "TTIOAC",
	TTILOBD:     "TTILOBD",
	TTIWRN:      "TTIWRN",
	TTIDCB:      "TTIDCB",
	TTIPFN:      "TTIPFN",
	TTI3GL:      "TTI3GL",
	TTIFOB:      "TTIFOB",
	TTISBND:     "TTISBND",
	TTIBVC:      "TTIBVC",
	TTIEOB:      "TTIEOB",
	TTISPF:      "TTISPF",
	TTIQC:       "TTIQC",
	TTIRSH:      "TTIRSH",
	TTIONEWAYFN: "TTIONEWAYFN",
	TTIIMPLRES:  "TTIIMPLRES",
	TTIRENEG:    "TTIRENEG",
	TTIDONE:     "TTIDONE",
	TTICOOKIE:   "TTICOOKIE",
	TTITKN:      "TTITKN",
	TTIINIT:     "TTIINIT",
}

// TTCMsgTypeDescription TTC message codes description map
var TTCMsgTypeDescription = map[common.MessageType]string{
	TTIPRO:      "Protocol message",
	TTIDTY:      "Data type message",
	TTIFUN:      "User function message",
	TTIOER:      "Oracle error message code",
	TTIRXH:      "Row transfer messageType message",
	TTIRXD:      "Row transfer data message",
	TTIRPA:      "Return parameters message",
	TTISTA:      "Status returned message",
	TTIIOV:      "Input/output vector message",
	TTISLG:      "User describe message",
	TTIOAC:      "Oracle area access message code (record)",
	TTILOBD:     "LOB data message",
	TTIWRN:      "Warning message",
	TTIDCB:      "Describe information message",
	TTIPFN:      "Client-to-server piggyback message",
	TTIFOB:      "Flush out bind data in DML/RETURN when error",
	TTIQC:       "Query cache message",
	TTIRSH:      "Result set messageType message",
	TTIBVC:      "Bit vector for compressed fetch",
	TTISPF:      "Server-side piggyback message",
	TTIONEWAYFN: "One-way function message",
	TTIIMPLRES:  "Send implicit resultsets",
	TTIRENEG:    "Protocol re-negotiation message",
	TTICOOKIE:   "Protocol cookie-based fast negotiation message",
	TTITKN:      "Token message code",
	TTIINIT:     "Protocol fast negotiation message",
}

// Stringer interface
func toString(mt common.MessageType) string {
	return TTCMsgTypeName[mt]
}

// Checks that the given TTCMsgType is valid
// This assumes that the array of type is a contiguous  int array
func isValid(t common.MessageType) bool {
	return t > 0 && int(t) < len(TTCMsgTypeName)
}

// isFunction returns true if the message is a function otherwise false
func isFunction(mgsType common.MessageType) bool {
	if mgsType == TTIFUN || mgsType == TTIPFN || mgsType == TTISPF || mgsType == TTIONEWAYFN {
		return true
	}
	return false
}

const (
	// MinOversionSupported is the minimum supported OVERSION.
	MinOversionSupported common.UB2 = 7230
	// MinTtcverSupported is the minimum supported TTC version.
	MinTtcverSupported common.UB2 = 4
	// V8TtcverSupported is the supported TTC version for Oracle 8.
	V8TtcverSupported common.UB2 = 5
	// MaxTtcverSupported is the maximum supported TTC version.
	MaxTtcverSupported common.UB2 = 6

	// Oracle Client side Function Codes
	ocqcinv      byte = 1  // Query Cache Invalidations
	ocospid      byte = 2  // OS PID for MTS connection
	octrcevt     byte = 3  // OCI trace event piggyback
	ocsessret    byte = 4  // Server CP return values for GET
	ocssync      byte = 5  // Session state synchronization
	ocxsss       byte = 6  // eXtensible security Session State Sync
	ocltxid      byte = 7  // LTXID
	ocappcontctl byte = 8  // application continuity replay context
	ocxsss2      byte = 9  // eXtensible security Session State Sync 2
	osesssign    byte = 10 // session signature sync
	ocshrdkey    byte = 11 // sharding key to client
	maxOcfn      byte = 11 // last item allocated

	// TTCLXMULTI indicates Flags for multibyte conversions
	TTCLXMULTI = 0x01
	// TTCLXMCONV indicates conversion may affect length.
	TTCLXMCONV = 0x02

	// oerflg codes
	oerfplsw = 0x04 // back transaction - PL/SQL compiler warning
	oerfexit = 0x10 // transaction - this is the last oerdef the

	// TTC Warning codes
	oerwany = 0x01 /* set on any warning */
	/* flag (overloaded) */
	/* careful that this flag is not used by */
	/* nls in upilgn to alter the nls_lang */
	/* parameter as we forget it add set it */
	/* back to what it was for the logon call */
	owewnivc = 0x04 // null values not used in an
	oerwcper = 0x20 // Pkg/proc created with - /* Where; set by parse */
	// TtiEocEct : elapsed call time follows
	TtiEocEct = 0x08
	// TtiEocfDropWhenReturned indicates this connection is affected by a planned-down
	TtiEocfDropWhenReturned common.UB4 = 0x00000800

	// Piggyback/session property key for elastic pool LDR flag
	al8kwPdbElasticPoolLdrStr = "AL8KW_PDB_ELASTIC_POOL_LDR"

	al8kwTimezone            = 163
	al8kwPdbElasticPoolLdr   = 203
	al8kwEnabledRoleNames    = 199 // enabled role names
	al8kwEnabledRoleNamesStr = "AL8KW_ENABLED_ROLE_NAMES"
	al8kwPdbAppRoot          = 202
	al8kwPdbAppRootStr       = "AL8KW_PDB_APP_ROOT"
	ldiRegIDFlag             = byte(0x80) // region id present if (value[2] & 0xFF) > ldiRegIDFlag
	ldiRegIDSet              = byte(0x40) // base offset applied to hour when region id present
	ldiMaxTimeField          = byte(60)   // base offset applied to hour/minute when region id not present
)
