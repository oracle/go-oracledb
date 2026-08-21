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

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

/*
Constants for data type identifiers used in Oracle database interactions.
These define various data types like characters, numbers, dates, and more,
categorized by their usage in SQL and internal representations.
*/

// DtyType represents the TTC data type identifier.
type DtyType = int16

const (
	DtyExpBase  DtyType = 256
	Dty0        DtyType = 0
	DtyChr      DtyType = 1  // characters, maybe space padded
	DtyNum      DtyType = 2  // numeric
	DtyInt      DtyType = 3  // native integer
	DtyFlt      DtyType = 4  // see below for float types
	DtyStr      DtyType = 5  // text, null terminated
	DtyVnu      DtyType = 6  // NUM with length in 1st byte
	DtyPdn      DtyType = 7  // packed decimal
	DtyLng      DtyType = 8  // long
	DtyVCS      DtyType = 9  // variable character string
	DtyTi5      DtyType = 10 // tiddef
	DtyRiD      DtyType = 11 // riddef
	DtyDat      DtyType = 12 // date data type
	DtyIdt      DtyType = 13 // internal date format // some internal data types. not internal, and not kernel
	DtyIju      DtyType = 14 // internal julian format
	DtyVbi      DtyType = 15 // variable-length raw
	DtyDif      DtyType = 16 // date input format
	DtyDof      DtyType = 17 // date output format
	DtyDtz      DtyType = 18 // date time zone
	DtyDyn      DtyType = 19 // Day name
	DtyDpc      DtyType = 20 // Date precision code
	DtyBfloat   DtyType = 21 // Native C Float, might not be ieee754
	DtyBdouble  DtyType = 22 // Native C Double, might not be ieee754
	DtyBin      DtyType = 23 // binary data (SQL RAW, internal structures)
	DtyLbi      DtyType = 24 // long binary (SQL LONG RAW)
	DtyUb2      DtyType = 25 // integer data types
	DtyUb4      DtyType = 26
	DtyB1       DtyType = 27
	DtyB2       DtyType = 28
	DtyB4       DtyType = 29
	DtyWord     DtyType = 30
	DtyUword    DtyType = 31
	DtyPb       DtyType = 32 // pointer data types
	DtyPw       DtyType = 33
	DtyOer8     DtyType = 34 + DtyExpBase // structure data types - used for TTI messages
	DtyFun      DtyType = 35 + DtyExpBase
	DtyAua      DtyType = 36 + DtyExpBase
	DtyRxh7     DtyType = 37 + DtyExpBase
	DtyNa6      DtyType = 38 + DtyExpBase // network form // user data area descriptors
	DtyOac      DtyType = 39              // native form
	DtyAms      DtyType = 40              // opidef program interface request block types
	DtyBrn      DtyType = 41
	DtyBrp      DtyType = 42 + DtyExpBase
	DtyBrv      DtyType = 43 + DtyExpBase
	DtyKva      DtyType = 44 + DtyExpBase
	DtyCls      DtyType = 45 + DtyExpBase
	DtyCui      DtyType = 46 + DtyExpBase
	DtyDfn      DtyType = 47 + DtyExpBase
	DtyDqr      DtyType = 48 + DtyExpBase
	DtyDsc      DtyType = 49 + DtyExpBase
	DtyExe      DtyType = 50 + DtyExpBase
	DtyFch      DtyType = 51 + DtyExpBase
	DtyGbv      DtyType = 52 + DtyExpBase
	DtyGem      DtyType = 53 + DtyExpBase
	DtyGiv      DtyType = 54 + DtyExpBase
	DtyOkg      DtyType = 55 + DtyExpBase
	DtyHmi      DtyType = 56 + DtyExpBase
	DtyIno      DtyType = 57 + DtyExpBase
	DtyOpq      DtyType = 58 // Opaque Types
	DtyLnf      DtyType = 59 + DtyExpBase
	DtyOnt      DtyType = 60 + DtyExpBase
	DtyOpe      DtyType = 61 + DtyExpBase
	DtyOsq      DtyType = 62 + DtyExpBase
	DtySfe      DtyType = 63 + DtyExpBase
	DtySpf      DtyType = 64 + DtyExpBase
	DtyVsn      DtyType = 65 + DtyExpBase
	DtyUd7      DtyType = 66 + DtyExpBase
	DtyDsa      DtyType = 67 + DtyExpBase
	DtyUin      DtyType = 68 // unsigned DtyINT
	DtyBri      DtyType = 69 // internal byte-comparable, rowid (kd4lbrid)
	Dty70       DtyType = 70
	DtyPin      DtyType = 71 + DtyExpBase
	DtyPfn      DtyType = 72 + DtyExpBase
	DtyPpt      DtyType = 73 + DtyExpBase
	DtyOcu      DtyType = 74 // object context union
	DtySto      DtyType = 75 + DtyExpBase
	Dty76       DtyType = 76               // used to be DtyRAT int16 =
	DtyArc      DtyType = 77 + DtyExpBase  // archive op
	DtyMrs      DtyType = 78 + DtyExpBase  // media recovery start
	DtyMrt      DtyType = 79 + DtyExpBase  // media recovery record tablespace
	DtyMrg      DtyType = 80 + DtyExpBase  // media recovery get starting log sequence #
	DtyMrr      DtyType = 81 + DtyExpBase  // media recovery recover using offline log
	DtyMrc      DtyType = 82 + DtyExpBase  // media recovery cancel
	DtyVer      DtyType = 83 + DtyExpBase  // version number
	DtyLon2     DtyType = 84 + DtyExpBase  // logon, w/extra information
	DtyIno2     DtyType = 85 + DtyExpBase  // OINIT, w/extra information
	DtyAll      DtyType = 86 + DtyExpBase  // bundled call
	DtyUdb      DtyType = 87 + DtyExpBase  // array bind describe info
	DtyAqi      DtyType = 88 + DtyExpBase  // AQ array enq/deq IN params
	DtyUlb      DtyType = 89 + DtyExpBase  // loader buffer transfer
	DtyUld      DtyType = 90 + DtyExpBase  // loader function call
	DtySls      DtyType = 91               // Datatype for display sign leading separate
	DtySid      DtyType = 92 + DtyExpBase  // Oracle session id
	DtyNa7      DtyType = 93 + DtyExpBase  //new network uac type
	DtyLvc      DtyType = 94               // Long long varchars
	DtyLvb      DtyType = 95               // long long varraw
	DtyAfc      DtyType = 96               // ANSI fixed char
	DtyAvc      DtyType = 97               // ANSI fixed char Null terminated
	DtyAl7      DtyType = 98 + DtyExpBase  // Datatype for pisdef for deferred upi
	DtyK2Rpc    DtyType = 99 + DtyExpBase  // RPC between transaction managers
	DtyIbFloat  DtyType = 100              // Canonical representation of IEEE754 Float
	DtyIbDouble DtyType = 101              // Canonical representation of IEEE754 Double
	DtyCur      DtyType = 102              // ref cursorId type SQLT_CUR // Operating system dependent data types -- for user->oracle conversions
	DtyXdp      DtyType = 103 + DtyExpBase // direct path Export
	DtyRdd      DtyType = 104              // rowid descriptor
	DtyArr      DtyType = Dty70            // array column type (recycled) %TEMPORARY%
	DtyVar      DtyType = Dty76            // varray column type (recycled) %TEMPORARY%
	DtyLab      DtyType = 105              // label datatype // For MLS
	DtyOsl      DtyType = 106              // operating system label
	DtyOko8     DtyType = 107 + DtyExpBase // for KOD
	DtyNty      DtyType = 108              // External Named Type
	DtyINty     DtyType = 109              // Internal Named Type
	DtyRef      DtyType = 110              // External REF type
	DtyIref     DtyType = 111              // Internal REF type
	DtyClob     DtyType = 112              // character lob
	DtyBlob     DtyType = 113              // binary lob
	DtyBFil     DtyType = 114              // binary file lob
	DtyCFil     DtyType = 115              // character file lob
	DtyRSet     DtyType = 116              // result set type
	DtyCwd      DtyType = 117
	DtySvt      DtyType = 118              // structure value type
	DtyJSON     DtyType = 119              // JSON type support
	DtyNac122   DtyType = 120              // oacdef
	DtyAdt      DtyType = 121              // additional types for use on the server only
	DtyNtb      DtyType = 122              // named table type
	DtyNar      DtyType = 123              // named array type
	DtyVec      DtyType = 127              // vector type
	DtyUd12     DtyType = 124 + DtyExpBase // new v8 udsdef
	DtyAl8      DtyType = 125 + DtyExpBase // v8 execute structure
	DtyLfop     DtyType = 126 + DtyExpBase // LOB and FILE operations except file create
	DtyFcrt     DtyType = 127 + DtyExpBase // FILE create operation
	DtyDny      DtyType = 128 + DtyExpBase // new v80 describe any
	DtyOpr      DtyType = 129 + DtyExpBase // Used for recursive open calls
	DtyPls      DtyType = 130 + DtyExpBase // Datatype for pisdef for bundled PL/SQL calls
	DtyXid      DtyType = 131 + DtyExpBase // transaction start, attach and detach operation
	DtyTxn      DtyType = 132 + DtyExpBase // transaction end and recover operation
	DtyDcb      DtyType = 133 + DtyExpBase // old describe callback
	DtyCca      DtyType = 134 + DtyExpBase // Cursor close all piggyback function
	DtyWrn      DtyType = 135 + DtyExpBase // warning message
	DtyObj      DtyType = 136              // Object form. Used on server side only
	DtyTlh121   DtyType = 137 + DtyExpBase // 12.1 Load Header
	DtyToh121   DtyType = 138 + DtyExpBase // 12.1 Typed Object Header
	DtyFoi      DtyType = 139 + DtyExpBase // failover info
	DtySid2     DtyType = 140 + DtyExpBase // V8 Session switching piggyback
	DtyTch      DtyType = 141 + DtyExpBase // COR Header
	DtyPii      DtyType = 142 + DtyExpBase
	DtyPfi      DtyType = 143 + DtyExpBase
	DtyPpu      DtyType = 144 + DtyExpBase
	DtyPte      DtyType = 145 + DtyExpBase
	DtyClv      DtyType = 146              // Character Lob Value
	DtyBlv      DtyType = 147              // Binary Lob Value
	DtyRxh8     DtyType = 148 + DtyExpBase // v8 rxhdef
	DtyTn12     DtyType = 149 + DtyExpBase // name,pref
	DtyAuth     DtyType = 150 + DtyExpBase // New generic logon call
	DtyKval     DtyType = 151 + DtyExpBase // keyword value pair
	DtyDtr      DtyType = 152              // cobol: display trailing
	DtyDun      DtyType = 153              // cobol: display unsigned
	DtyDop      DtyType = 154              // cobol: display overpunch
	DtyVst      DtyType = 155              // orl: vstring format
	DtyOdt      DtyType = 156              // orl: date format
	DtyFgi      DtyType = 157 + DtyExpBase
	DtyDsy      DtyType = 158 + DtyExpBase // V8 descibe any
	DtyDsyR8    DtyType = 159 + DtyExpBase // top level descriptor for V8.0 describe any
	DtyDsyH8    DtyType = 160 + DtyExpBase // header descriptor for V8.0 describe any
	DtyDsyL     DtyType = 161 + DtyExpBase // list descriptor for V8 describe any
	DtyDsyT8    DtyType = 162 + DtyExpBase // table descriptor for V8.0 describe any
	DtyDsyV8    DtyType = 163 + DtyExpBase // view descriptor for V8 describe any
	DtyDsyP     DtyType = 164 + DtyExpBase // procedure descriptor for V8 describe any
	DtyDsyF     DtyType = 165 + DtyExpBase // function descriptor for V8 describe any
	DtyDsyK     DtyType = 166 + DtyExpBase // package descriptor for V8 describe any
	DtyDsyY     DtyType = 167 + DtyExpBase // synonym descriptor for V8 describe any
	DtyDsyQ     DtyType = 168 + DtyExpBase // sequence descriptor for V8 describe any
	DtyDsyC     DtyType = 169 + DtyExpBase // column descriptor for V8 describe any
	DtyDsyA     DtyType = 170 + DtyExpBase // argument descriptor for V8 describe any
	DtyOt8      DtyType = 171 + DtyExpBase // Oracle Transaction service: Commit remote sites
	DtyDol      DtyType = 172              // display overpunch leading
	DtyDsyTy    DtyType = 173 + DtyExpBase // function descriptor for V8 describe any
	DtyAqe      DtyType = 174 + DtyExpBase // AQ Enqueue structure
	DtyKv       DtyType = 175 + DtyExpBase // fast keyword value pair
	DtyAqd      DtyType = 176 + DtyExpBase // AQ Dequeue structure for 8.1
	DtyAQ8      DtyType = 177 + DtyExpBase // AQ Message properties structure
	DtyTime     DtyType = 178              // TIME
	DtyTtz      DtyType = 179              // TIME WITH TIME ZONE
	DtyStamp    DtyType = 180              // TIMESTAMP
	DtyStz      DtyType = 181              // TIMESTAMP WITH TIME ZONE
	DtyIym      DtyType = 182              // INTERVAL YEAR TO MONTH
	DtyIds      DtyType = 183              // INTERVAL DAY TO SECOND
	DtyEdate    DtyType = 184              // DATE in structured format
	DtyEtime    DtyType = 185              // TIME in structured format
	DtyEttz     DtyType = 186              // TIME WITH TIME ZONE in structured format
	DtyEstamp   DtyType = 187              // TIMESTAMP in structured format
	DtyEstz     DtyType = 188              // TIMESTAMP WITH TIME ZONE in structured format
	DtyEiym     DtyType = 189              // INTERVAL YEAR TO MONTH in structured format
	DtyEids     DtyType = 190              // INTERVAL DAY TO SECOND in structured format
	DtyLdiIf    DtyType = 191              // LDI input format
	DtyLdiOf    DtyType = 192              // LDI output format
	DtyRfs      DtyType = 193 + DtyExpBase // Remote archive file server
	DtyRxh10    DtyType = 194 + DtyExpBase // V8.1 row header definition
	DtyDclob    DtyType = 195              // Desriptor representations. for DtyCLOB int16 = - internal use only
	DtyDblob    DtyType = 196              // Descriptor representations for DtyBLOB int16 = - internal use only
	DtyDbfil    DtyType = 197              // Descriptor representations for DtyBFIL int16 = - internal use only
	DtyDjson    DtyType = 198              // Descriptor representations for DtyJSON int16 = - internal use only
	DtyKpn      DtyType = 198 + DtyExpBase // kernel programmatic notification
	DtyKpdnr    DtyType = 199 + DtyExpBase // notification registration info
	DtyDsyD     DtyType = 200 + DtyExpBase // Database descriptor for V8 describe any
	DtyDsyS     DtyType = 201 + DtyExpBase // Schema descriptor for V8 describe any
	DtyDsyR     DtyType = 202 + DtyExpBase // KPCDS structur for 8.1 onward
	DtyDsyH     DtyType = 203 + DtyExpBase // Header descriptor for 8.1 onward
	DtyDsyT     DtyType = 204 + DtyExpBase // table descriptor for 8.1 onward
	DtyDsyV     DtyType = 205 + DtyExpBase // future use
	DtyAqm      DtyType = 206 + DtyExpBase // AQ message properties structure for Oracle 8.1
	DtyOer11    DtyType = 207 + DtyExpBase // V8.1 oerdef structure
	DtyBuri     DtyType = 208              // internal universal rowid (kd4ubrid)
	DtyPsr      DtyType = 209              // partial sort record for parallel aggregates
	DtyAql      DtyType = 210 + DtyExpBase // aq listen
	DtyOtc      DtyType = 211 + DtyExpBase // OTs: Commit remote sites for version >int16 = 8.1.3.0.0
	DtyKfno     DtyType = 212 + DtyExpBase // KFN Operation
	DtyKfnp     DtyType = 213 + DtyExpBase // KFN Parameters
	DtyOkgt8    DtyType = 214 + DtyExpBase // for object transfer
	DtyRaSb4    DtyType = 215 + DtyExpBase // sb4 IDL piece
	DtyRaUb2    DtyType = 216 + DtyExpBase // ub2 IDL piece
	DtyRaUb1    DtyType = 217 + DtyExpBase // ub1 IDL piece
	DtyRaTxt    DtyType = 218 + DtyExpBase // txt IDL piece
	DtyRsSb4    DtyType = 219 + DtyExpBase // segment of sb4 IDL pieces
	DtyRsUb2    DtyType = 220 + DtyExpBase // segment of ub2 IDL pieces
	DtyRsUb1    DtyType = 221 + DtyExpBase // segment of ub1 IDL pieces
	DtyRsTxt    DtyType = 222 + DtyExpBase // segment of txt IDL pieces
	DtyRidl     DtyType = 223 + DtyExpBase // top most structure for IDL piece (diana or pcode)
	DtyGlrdd    DtyType = 224 + DtyExpBase // structure for transfering single dependency
	DtyGlrdg    DtyType = 225 + DtyExpBase // dependency segment
	DtyGlrdc    DtyType = 226 + DtyExpBase // array of arrays of dependencies
	DtyOko      DtyType = 227 + DtyExpBase // KOD operations post 8.1
	DtyDpp      DtyType = 228 + DtyExpBase // Direct Path Prepare descriptor
	DtyDpls     DtyType = 229 + DtyExpBase // Direct Path Load Stream descriptor
	DtyDpmop    DtyType = 230 + DtyExpBase // Direct Path Misc Operations descriptor
	DtySitz     DtyType = 231              // TIMESTAMP WITH IMPLICIT TIME ZONE
	DtyEsitz    DtyType = 232              // TIMESTAMP WITH IMPLICIT TIME ZONE in struct
	DtyUb8      DtyType = 233
	DtyStat     DtyType = 234 + DtyExpBase
	DtyRfx      DtyType = 235 + DtyExpBase // Remote archive file server
	DtyFal      DtyType = 236 + DtyExpBase // Fetch Archive Log operation
	DtyCkv      DtyType = 237 + DtyExpBase
	DtyDrcx     DtyType = 238 + DtyExpBase // DR Server Connection Process
	DtyKgh      DtyType = 239 + DtyExpBase // KGl Heap
	DtyAqo      DtyType = 240 + DtyExpBase // AQ array enq/deq OUT params
	DtyPnty     DtyType = 241              // pl/sql representation of named types
	DtyOkgt     DtyType = 242 + DtyExpBase // KGL transfer - 8.2
	DtyKpfc     DtyType = 243 + DtyExpBase // Fast interconnect exchange
	DtyFe2      DtyType = 244 + DtyExpBase // the new fetch interface
	DtySpfp     DtyType = 245 + DtyExpBase // put spfile parameters
	DtyDpuls    DtyType = 246 + DtyExpBase // Direct Path Unload Stream descriptor
	DtyRec      DtyType = 250              // pl/sql 'record' (or %rowtype)
	DtyTab      DtyType = 251              // pl/sql 'indexed table'
	DtyBol      DtyType = 252              // pl/sql 'boolean'
	DtyAqa      DtyType = 253 + DtyExpBase // AQ array enq/deq structure
	DtyKpbf     DtyType = 254 + DtyExpBase // File transfer
	DtyDty      DtyType = 255              // type code expasion

	// ====== VALUES 256-512 TAKEN =========

	DtyTsm           DtyType = 513 // transparent session migration
	DtyMss           DtyType = 514 // migration session state
	DtyAbs           DtyType = 515 // abstract type
	DtyKpc           DtyType = 516 // KPS Component Union ///
	DtyCrs           DtyType = 517 // cursorId state
	DtyKks           DtyType = 518 // KKS state
	DtyKsp           DtyType = 519 // parameter marshal for tsm
	DtyKspTop        DtyType = 520 // top level parameter marshal for tsm
	DtyKspVal        DtyType = 521 // parameter value migration
	DtyPss           DtyType = 522 // Plsql Session State
	DtyNls           DtyType = 523 // NLS session state
	DtyAls           DtyType = 524 // NLS parameter
	DtyKsdEvtVal     DtyType = 525 // each event string
	DtyKsdEvtTop     DtyType = 526 // top level list
	DtyKpspp         DtyType = 527 // TSM session state piece
	DtyKol           DtyType = 528 // LOB session state
	DtyLst           DtyType = 529 // lob state
	DtyAcx           DtyType = 530 // application context
	DtyScs           DtyType = 531 // set schema
	DtyRxh           DtyType = 532 // V10.2 row header definition
	DtyKpdns         DtyType = 533 // notification union of namespace IN attributes
	DtyKpdcn         DtyType = 534 // notification: dbchange IN attrs
	DtyKpnns         DtyType = 535 // notification: union of namespace OUT attrs
	DtyKpncn         DtyType = 536 // notification: dbchange out attrs
	DtyKps           DtyType = 537 // moved KPS to ub2 space
	DtyApinf         DtyType = 538 // dbms_application_info
	DtyTen           DtyType = 539 // table encryption information
	DtyXsscs         DtyType = 540 // XS Session Create Session
	DtyXssro         DtyType = 541 // XSS Session Roundtrip Operations
	DtyXsspo         DtyType = 542 // XSS Piggyback Operations
	DtyKsrpc         DtyType = 543 // KSRPC defn
	DtyKvl           DtyType = 560 // long keyword value pair
	DtySessGet       DtyType = 563 // Session get call
	DtySessRls       DtyType = 564 // Session rls cal
	DtyXsssDef       DtyType = 565 // XS State Sync DEFinition
	DtyKpdqcInv      DtyType = 572 // invalidation record
	DtyKpdqIdc       DtyType = 573 // in-band database change info
	DtyKpdqcSta      DtyType = 574 // query cache stats
	DtyKprs          DtyType = 575 // result set message
	DtyKpdqcID       DtyType = 576 // result set message
	DtyTrcevt        DtyType = 577 // OCI Trace event
	DtyRtstrm        DtyType = 578 // RPC Test stream
	DtySessRet       DtyType = 579 // Return values for OSESSGET
	DtyScn6          DtyType = 580
	DtyKecpa         DtyType = 581 // replay data for of plsql rpc
	DtyKecpp         DtyType = 582 // replay data for of plsql rpc
	DtySxa           DtyType = 583 // Streams external apply
	DtyKvarr         DtyType = 584 // Key-Value array
	DtyKpngn         DtyType = 585 // notification: generic out attrs
	DtyXsnsop        DtyType = 590 // XS NameSpace RPC
	DtyXsattr        DtyType = 591 // XS ATTRibute
	DtyXsns          DtyType = 592 // XS NameSpace
	DtyTxt           DtyType = 593 // XS String array // New records for Triton 12c:
	DtyXssessns      DtyType = 594 // XS System Namespace
	DtyXsattop       DtyType = 595 // XS Attach
	DtyXscreop       DtyType = 596 // XS Create Session
	DtyXsdetop       DtyType = 597 // XS Detach
	DtyXsdesop       DtyType = 598 // XS Destroy Session
	DtyXssetsp       DtyType = 599 // XS Set Session Parameter
	DtyXssidp        DtyType = 600 // XS Secure ID propagation
	DtyXsprin        DtyType = 601 // XS Principal
	DtyXskvl         DtyType = 602 // XS Key value pair
	DtyXsssdef2      DtyType = 603 // XS State Sync Definition
	DtyXsnsop2       DtyType = 604
	DtyXsns2         DtyType = 605
	DtyImplRes       DtyType = 611 // Implicit Results TTC union
	DtyOer19         DtyType = 612 // new oer def for 12.2,OerDty11 is for pre-12g clients
	DtyUb1Array      DtyType = 613 // general purpose ub1 array
	DtySessState     DtyType = 614 // session state ops
	DtyAppContReplay DtyType = 615 // application continuity replay RPC
	DtyAppContCtl    DtyType = 616 // application continuity s->c control
	DtyKpdssTemplate DtyType = 617 // session state template
	DtykpdnrEq       DtyType = 622 // jms event notification request record
	DtykpdnrNf       DtyType = 623 // jms event notification header record
	DtyKpngnc        DtyType = 624
	DtyKpnri         DtyType = 625
	DtyAqEnq         DtyType = 626 // jms enqueue RPC
	DtyAqDeq         DtyType = 627 // jms dequeu RPC
	DtyAqJms         DtyType = 628 // jms properties
	DtykpdnrPay      DtyType = 629 // jms notification event payload
	DtykpdnrAck      DtyType = 630 // jms notification event acknowledgement
	DtykpdnrMp       DtyType = 631 // jms notification event
	DtykpdnrDq       DtyType = 632
	DtyChunkInfo     DtyType = 636 // chunk info client to server piggyback
	DtyScn           DtyType = 637
	DtyScn8          DtyType = 638
	DtyUd21          DtyType = 639              // version 12.2 udsdef
	DtyTnp           DtyType = 640              // version 12.2 name,pref
	DtyTlh           DtyType = 643 + DtyExpBase // version 12.2 Load Header
	DtyToh           DtyType = 644 + DtyExpBase // version 12.2 Typed Object Header
	DtySnp           DtyType = 645 + DtyExpBase // kosnp record
	DtyNac           DtyType = 646              // latest oacdef
	DtySessSign      DtyType = 647              // session signature sync s->c
	DtyKpdxft        DtyType = 648              // client feature tracking piggyback
	DtyKpdxst        DtyType = 649              // client extended statistics
	DtyKpdxstprot    DtyType = 650              // extended stats protocol info
	DtyKpdxsttcp     DtyType = 651              // extended statistics tcp info
	DtyOer           DtyType = 652              // new oerdef. DtyOer19 sent to pre-20c clients
	DtyShrdKeySync   DtyType = 653              // Sharded database shard key
	DtySaga          DtyType = 656              // DBMS Sagas
	DtyPlend         DtyType = 660              // Pipeline End RPC
	DtyPlbgn         DtyType = 661              // Pipeline Begin piggyback
	DtyUds           DtyType = 663              // version 23 udsdef
	DtyPlopn         DtyType = 665              // Pipeline Operatiion piggyback
)

const (
	// RepUnv is the default representation for all types
	RepUnv byte = 1
	// RepBUnv is the universal UB1/bin representation.
	RepBUnv byte = 1
	// RepCUnv is the general character representation.
	RepCUnv byte = 1

	// RepIUnv is the universal integer representation: 1 byte sign+length followed by the digits.
	// Integers have the following names, where base 256 digits are numbered 8..1.
	// REPIrlh: | --- position of high order digit, 1 == first in memory
	//          | ---- position of low order digit
	//          | ----- representation: U == unsigned, T == 2's comp signed, O == 1's complement signed
	RepIUnv byte = 1

	// RepAUnv is the universal pointer address representation (0 == null, !0 == not null).
	RepAUnv byte = 1

	// RepNV51 is the Oracle number representation for version 5.1 numbers.
	RepNV51 byte = 10
	// RepDV51 is the Oracle date representation for version 5.1 dates.
	RepDV51 byte = 10

	// RepRUnv is the record representation where fields are byte packed.
	// TTCBUR always returns RepRUnv. TTCCLR notices if a record can be sent directly, and returns RepRUnv when this case occurs.
	RepRUnv byte = 1

	// Native is used for native data type representation.
	// Bit 1: 0 -- Native, 1 -- Universal
	// Bit 2: 0 -- MSB, 1 -- LSB
	// 00 (0) -> Native + MSB
	// 01 (1) -> UNIVERSAL + MSB
	// 10 (2) -> Native + LSB (not applicable, but maybe used)
	// 11 (3) -> UNIVERSAL + LSB
	Native byte = 0x00

	// Universal is used for universal data type representation (byte with length followed by data).
	Universal byte = 0x01

	// Lsb is used for Lsb data type representation (normally not used, but present for completeness).
	Lsb byte = 0x02

	// MaxRep is the max supported type representation (max possible conversion is 0x01 + 0x02 = 0x03).
	// If this is exceeded, an exception will be raised.
	MaxRep byte = 0x03

	// MaxType is the maximum supported type value.
	MaxType byte = 4

	// NumReps is the number of supported representations.
	NumReps = byte(MaxType + 1)

	// B1 index in basic type array
	B1 byte = 0
	// B2 index in basic type array
	B2 byte = 1
	// B4 index in basic type array
	B4 byte = 2
	// B8 index in basic type array
	B8 byte = 3
	// PTR index in basic type array
	PTR byte = 4

	// MaxReps has no specific meaning here. It's only used as a hint for the size of typeAndRep
	// so that we know that we have enough room and don't constantly reallocate a larger one.
	MaxReps int16 = 665

	// _maxReceivedReps is the cap for the number of type representations that will be unmarshalled,
	// if the number of type representations received exceeds this number a protocol violation error
	// will be returned.
	_maxReceivedReps int16 = 1024
)

// representationTable manages type representations and conversion flags for Oracle data types.
// It provides methods to set and get type representations, conversion flags, and server conversion state.
type representationTable struct {
	representations  []int16
	conversionFlags  byte
	serverConversion bool
	// keep track of native types being wether UNIVERSAL
	// or NATIVE
	nativeTypesRepresentation [5]byte
}

// this is filled in in package Init()
var typeRepresentationTable *representationTable = newTypeRep()

// newTypeRep makes a new blank TypeRep table
func newTypeRep() *representationTable {
	var t = &representationTable{
		conversionFlags:  0,
		serverConversion: false,
		representations:  make([]int16, MaxReps*4+1),
	}

	t.nativeTypesRepresentation[B1] = Native
	t.nativeTypesRepresentation[B2] = Universal
	t.nativeTypesRepresentation[B4] = Universal
	t.nativeTypesRepresentation[B8] = Universal
	t.nativeTypesRepresentation[PTR] = Universal

	// offset is the first bytes of this array
	// _oSessionKeyInit it to 1
	t.representations[0] = 1
	return t
}

func (t *representationTable) isNativeTypeAsUniversal(typ byte) bool {
	return t.nativeTypesRepresentation[typ] == Universal
}

// MarshalTo marshalTypeReps marshals the type representations based on capabilities.
func (t *representationTable) MarshalTo(ctx context.Context, mar driverCommon.Marshaller) error {
	// send dty and rep as UB2
	for i := 1; i < int(t.representations[0]); i++ {
		if err := mar.MarshalUB2(ctx, driverCommon.UB2(t.representations[i])); err != nil { // LSB is false
			return err
		}
	}
	if err := mar.MarshalUB2(ctx, 0); err != nil { // to mark the end
		return err
	}
	return nil
}

// UnMarshalFrom unmarshal the type representations.
func (t *representationTable) UnMarshalFrom(ctx context.Context, mar driverCommon.Marshaller) error {
	var b driverCommon.UB2
	var err error

	// It loops through the received data until two consecutive zeros are found,
	// indicating the end of the structure. No direct matching validation is performed
	// between sent and received types (commented out for compatibility with v80).
	//
	// The structure consists of type blocks starting with a non-zero byte, followed by
	// zero or more pairs of bytes. The first byte of each pair is non-zero, but the
	// second may be zero. A zero where a pair start is expected ends the block, and
	// a zero where a type block start is expected ends the entire structure.
	//
	// Examples:
	//
	//	NN 00
	//	NN WW ww 00
	//	NN XX xx YY yy 00
	//	00
	//

	// inTypeBlock is true when currently parsing UB2 pairs within a type block.
	// It is set to false when expecting the start of a new type block or the terminal zero.
	inTypeBlock := false

	// pairCount tracks the position within a pair in the block:
	// 0 when expecting the leading UB2 (type code) of a pair,
	// 1 when expecting the trailing UB2 (representation) of a pair.
	pairCount := 0

	var nbDty int16 = 0
	for {
		// Reading the next UB2
		b, err = mar.UnmarshalUB2(ctx)
		if err != nil {
			common.Odl.Warn("Failed to unmarshal typerep", "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, "TypeRep")
		}

		if !inTypeBlock {
			nbDty++
			// The type of the next block was read, or the terminal 0 was read
			if b == 0 {
				return nil
			}

			if nbDty > _maxReceivedReps {
				common.Odl.Warn("Failed to unmarshal typerep, number of type representations exceeded the maximum allowed")
				return common.NewOracleError(oracleErrors.ProtocolViolationLimitExceeded, nil, "nbDty", _maxReceivedReps, nbDty)
			}

			inTypeBlock = true
			pairCount = 0
		} else {
			switch pairCount {
			case 0:
				if b == 0 {
					inTypeBlock = false
				} else {
					pairCount = 1
				}
			case 1:
				pairCount = 0
			}
		}
	}
}

// addTypeRepToTable registers a data type with its type code, network type, and representation in the typeAndRep map.
func (t *representationTable) addTypeRepToTable(dty, ndty, rep int16) {
	if len(t.representations) < int(t.representations[0])+4 {
		typeAndRep2 := make([]int16, len(t.representations)*2)
		copy(typeAndRep2[0:], t.representations[0:t.representations[0]+1])
		t.representations = typeAndRep2
	}

	offset := t.representations[0]
	t.representations[offset] = dty
	t.representations[offset+1] = ndty

	if ndty == Dty0 {
		t.representations[0] = int16(offset + 2)
	} else {
		t.representations[offset+2] = rep
		t.representations[offset+3] = 0
		t.representations[0] = int16(offset + 4)
	}
}

// setRep sets the type representation for the given type.
// Returns an error if the type or representation is invalid.
func (t *representationTable) setRep(typ byte, rep byte) {
	t.nativeTypesRepresentation[typ] = rep
}

// getRep returns the representation for the given type.
// Returns an error if the type is invalid.
func (t *representationTable) getRep(typ byte) byte {
	return t.nativeTypesRepresentation[typ]
}

// SetFlags sets the conversion flags for the TypeRepresentationTable.
func (t *representationTable) SetFlags(flags byte) {
	t.conversionFlags = flags
}

// getFlags returns the current conversion flags value.
func (t *representationTable) getFlags() byte {
	return t.conversionFlags
}
