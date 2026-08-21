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

// assumedSrvRtCaps is Server's Run time capability for 23.4 which clients should assume.
var assumedSrvRtCaps = []byte{
	0x02, 0x01, 0x00, 0x01, 0x18, 0x00,
	0x7f, 0x01, 0x00, 0x00, 0x00, 0x00,
}

// assumedSrvCtCaps is Server's Compile time capability for 23.4 which clients should assume
var assumedSrvCtCaps = []byte{
	0x06, 0x01, 0x01, 0x01, 0xef, 0x0f, 0x01,
	ttcFldVsn231, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01,
	0x01, 0x7f, 0xff, 0x03, 0x10, 0x03, 0x03,
	0x01, 0x01, 0xff, 0x01, 0xff, 0xff, 0x01,
	0x0c, 0x01, 0x01, 0xff, 0x01, 0x06, 0x0c,
	0xf6, 0x01, 0x7f, 0x05, 0x0f, 0xff, 0x0d,
	0x0b, 0x00, 0x27, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00,
}

type capabilityMetadata struct {
	index     uint8
	value     byte
	isFlag    bool
	isDefault bool
}

// Extracted constant names from the map to avoid magic strings
const (
	kpccapCtSQLVersion             = "KPCCAP_CT_SQL_VERSION"
	kpccapCtXaUnderstandsBinaryXid = "KPCCAP_CT_XA_UNDERSTANDS_BINARY_XID"
	kpccapCtReplIntErrCre          = "KPCCAP_CT_REPL_INT_ERR_CRE"
	kpccapCtSidExtendedInitOra     = "KPCCAP_CT_SID_EXTENDED_INIT_ORA"
	kztvovKpclogNoName             = "KZTVOV_KPCLOG_NO_NAME"
	kztvovKpclogO3l                = "KZTVOV_KPCLOG_O3L"
	kztvovKpclogO5lNp              = "KZTVOV_KPCLOG_O5L_NP"
	kztvovKpclogO4l                = "KZTVOV_KPCLOG_O4L"
	kztvovKpclogO5l                = "KZTVOV_KPCLOG_O5L"
	kztvovKpclogO6l                = "KZTVOV_KPCLOG_O6L"
	kztvovKpclogO7lMr              = "KZTVOV_KPCLOG_O7L_MR"
	kpccapCtbPrefetchRows          = "KPCCAP_CTB_PREFETCH_ROWS"
	kpccapCtbImplicitPool          = "KPCCAP_CTB_IMPLICIT_POOL"
	kpccapCtbOauthmsgOnerr         = "KPCCAP_CTB_OAUTHMSG_ONERR"
	kpccapCtScrollableCursor       = "KPCCAP_CT_SCROLLABLE_CURSOR"
	kpccapCtTtcFldVsn              = "KPCCAP_CT_TTC_FLD_VSN"
	kpccapCt82pisal8               = "KPCCAP_CT_82PISAL8"
	kpccapCtDequeuewithselector    = "KPCCAP_CT_DEQUEUEWITHSELECTOR"
	kpccapCtTypeEvol               = "KPCCAP_CT_TYPE_EVOL"
	kpccapCt2rt                    = "KPCCAP_CT_2RT"
	kpccapCtClientidProp           = "KPCCAP_CT_CLIENTID_PROP"
	kpccapCtLobscnCache            = "KPCCAP_CT_LOBSCN_CACHE"
	kpccapCtDescribeOpen           = "KPCCAP_CT_DESCRIBE_OPEN"
	kpccapCtbTtc1Eocs              = "KPCCAP_CTB_TTC1_EOCS"
	kpccapCtbTtc1Fbvc              = "KPCCAP_CTB_TTC1_FBCV"
	kpccapCtbTtc1Pblb              = "KPCCAP_CTB_TTC1_PBLB"
	kpccapCtbTtc1Inrc              = "KPCCAP_CTB_TTC1_INRC"
	kpccapCtbOci1Fsap              = "KPCCAP_CTB_OCI1_FSAP"
	kpccapCtbOci1Apctx             = "KPCCAP_CTB_OCI1_APCTX"
	kpccapCtObjectTdsver           = "KPCCAP_CT_OBJECT_TDSVER"
	kpccapCtRpcver                 = "KPCCAP_CT_RPCVER"
	kpccapCtRpcsig                 = "KPCCAP_CT_RPCSIG"
	kpccapCtRpcflgs                = "KPCCAP_CT_RPCFLGS"
	kpccapCtDbfver                 = "KPCCAP_CT_DBFVER"
	kpccapCtPlsql                  = "KPCCAP_CT_PLSQL"
	koleLobCapUb8Size              = "KOLE_LOB_CAP_UB8_SIZE"
	koleLobCapEncs                 = "KOLE_LOB_CAP_ENCS"
	koleLobCapDil                  = "KOLE_LOB_CAP_DIL"
	koleLobCapTmplocSz             = "KOLE_LOB_CAP_TMPLOC_SZ"
	koleLobCapRemDataInt           = "KOLE_LOB_CAP_REM_DATA_INT"
	koleLobCapPrfch                = "KOLE_LOB_CAP_PRFCH"
	koleLobCap12c                  = "KOLE_LOB_CAP_12C"
	kpccapCtKpin                   = "KPCCAP_CT_KPIN"
	kpccapCtAqPropDqa              = "KPCCAP_CT_AQ_PROP_DQA"
	kpccapCtbTtc2Zlnp              = "KPCCAP_CTB_TTC2_ZLNP"
	kpccapCtUb2dty                 = "KPCCAP_CT_UB2DTY"
	kpccapCtReplLcrx2y             = "KPCCAP_CT_REPL_LCRX2Y"
	kpccapCtAstr                   = "KPCCAP_CT_ASTR"
	kpccapCtObfuscate              = "KPCCAP_CT_OBFUSCATE"
	kpccapCtbOci2Cqc               = "KPCCAP_CTB_OCI2_CQC"
	kpccapCtbOci2Edition           = "KPCCAP_CTB_OCI2_EDITION"
	kpccapCtbOci2Srvcp             = "KPCCAP_CTB_OCI2_SRVCP"
	kpccapCtProxy                  = "KPCCAP_CT_PROXY"
	kpccapCtStrmsCca               = "KPCCAP_CT_STRMS_CCA"
	kpccapCtMaxocfn                = "KPCCAP_CT_MAXOCFN"
	kpccapCtbOci3Ocssync           = "KPCCAP_CTB_OCI3_OCSSYNC"
	kpccapCtbOci3AppcontAuto       = "KPCCAP_CTB_OCI3_APPCONT_AUTO"
	kpccapCtbXMLCsxxmlt            = "KPCCAP_CTB_XML_CSXXMLT"
	kpccapCtbXMLLobstrImgOnly      = "KPCCAP_CTB_XML_LOBSTR_IMG_ONLY"
	kpccapCtbTtc3Colmetadata       = "KPCCAP_CTB_TTC3_COLMETADATA"
	kpccapCtbTtc3Tzver             = "KPCCAP_CTB_TTC3_TZVER"
	kpccapCtbTtc3Implres           = "KPCCAP_CTB_TTC3_IMPLRES"
	kpccapCtbTtc3BigchunkClr       = "KPCCAP_CTB_TTC3_BIGCHUNK_CLR"
	kpccapCtbTtc3KeepOutOrder      = "KPCCAP_CTB_TTC3_KEEP_OUT_ORDER"
	kpccapCtXstreamOut             = "KPCCAP_CT_XSTREAM_OUT"
	kpccapCtDtysesssignRecVsn      = "KPCCAP_CT_DTYSESSSIGN_REC_VSN"
	kpccapCtbTtc4FastReneg         = "KPCCAP_CTB_TTC4_FAST_RENEG"
	kpccapCtbTtc4Ibnm              = "KPCCAP_CTB_TTC4_IBNM"
	kpccapCtbTtc4BigTztc           = "KPCCAP_CTB_TTC4_BIG_TZTC"
	kpccapCtbTtc4ExplBound         = "KPCCAP_CTB_TTC4_EXPL_BOUND"
	kpccapCtSqlidLength            = "KPCCAP_CT_SQLID_LENGTH"
	koleLob2CapQuasi               = "KOLE_LOB2_CAP_QUASI"
	koleLob2CapVbl32               = "KOLE_LOB2_CAP_VBL32"
	koleLob2Cap2gbPrefetch         = "KOLE_LOB2_CAP_2GB_PREFETCH"
	kpccapCtbShrdKeys              = "KPCCAP_CTB_SHRD_KEYS"
	kpccapCtbTtc5Vector            = "KPCCAP_CTB_TTC5_VECTOR"
	kpccapCtbTtc5FlexTxn           = "KPCCAP_CTB_TTC5_FLEX_TXN"
	kpccapCtbTtc5CqnPull           = "KPCCAP_CTB_TTC5_CQN_PULL"
	kpccapCtbTtc5PdbParams         = "KPCCAP_CTB_TTC5_PDB_PARAMS"
	kpccapCtbTtcspareAltsess       = "KPCCAP_CTB_TTCSPARE_ALTSESS"
	kpccapCtbFeatureBackportSpare2 = "KPCCAP_CTB_FEATURE_BACKPORT_SPARE2"
	kpccapCtbFeatureBackportSpare3 = "KPCCAP_CTB_FEATURE_BACKPORT_SPARE3"
	kpccapCtbFeatureBackportSpare4 = "KPCCAP_CTB_FEATURE_BACKPORT_SPARE4"
	kpccapCtbFeatureBackportSpare5 = "KPCCAP_CTB_FEATURE_BACKPORT_SPARE5"
	kpccapCtbFeatureBackportSpare6 = "KPCCAP_CTB_FEATURE_BACKPORT_SPARE6"
	kpccapCtRaft                   = "KPCCAP_CT_RAFT"
	kpccapCtVectorFeatureBinary    = "KPCCAP_CT_VECTOR_FEATURE_BINARY"
	kpccapCtVectorFeatureSparse    = "KPCCAP_CT_VECTOR_FEATURE_SPARSE"
	kpccapCtResetState             = "KPCCAP_CT_RESET_STATE"
	// Runtime capabilities
	kpccapRtCompat                 = "KPCCAP_RT_COMPAT"
	kpccapRtTzEx                   = "KPCCAP_RT_TZ_EX"
	kpccapRtKpf01                  = "KPCCAP_RT_KPF01"
	kpccapRtInstTyp                = "KPCCAP_RT_INST_TYP"
	kpccapRtUb2Rep                 = "KPCCAP_RT_UB2_REP"
	kpccapRtAsmVolSprt             = "KPCCAP_RT_ASM_VOL_SPRT"
	kpccapRtbTtcZcpy               = "KPCCAP_RTB_TTC_ZCPY"
	kpccapRtbTtcTzlt               = "KPCCAP_RTB_TTC_TZLT"
	kpccapRtbTtc32k                = "KPCCAP_RTB_TTC_32K"
	kpccapRtbTtcCdb                = "KPCCAP_RTB_TTC_CDB"
	kpccapRtbTtcSessstateops       = "KPCCAP_RTB_TTC_SESSSTATEOPS"
	kpccapRtbTtcFeatureTrack       = "KPCCAP_RTB_TTC_FEATURE_TRACK"
	kpccapRtbTtcClientStats        = "KPCCAP_RTB_TTC_CLIENT_STATS"
	kpccapRtbTtcSvrchksum          = "KPCCAP_RTB_TTC_SVRCHKSUM"
	kpccapRtbMaxcols               = "KPCCAP_RTB_MAXCOLS"
	kpccapRtbTtc1Drcpv2            = "KPCCAP_RTB_TTC1_DRCPV2"
	kpccapRtbTtc1Iovoff            = "KPCCAP_RTB_TTC1_IOVOFF"
	kpccapRtbTtc1Mxstrsz           = "KPCCAP_RTB_TTC1_MXSTRSZ"
	kpccapRtbFeatureBackportSpare1 = "KPCCAP_RTB_FEATURE_BACKPORT_SPARE1"
	kpccapRtbFeatureBackportSpare2 = "KPCCAP_RTB_FEATURE_BACKPORT_SPARE2"
	kpccapRtFeatureBackportSpare3  = "KPCCAP_RT_FEATURE_BACKPORT_SPARE3"
	kpccapRtbOtelTrace             = "KPCCAP_RTB_OTEL_TRACE"
	kpccapRtbOtelLogs              = "KPCCAP_RTB_OTEL_LOGS"
	kpccapRtbOtelMetrics           = "KPCCAP_RTB_OTEL_METRICS"
)

const (
	// Constants
	kpulmaxl byte = 6 // maximum language types defined
	// TTC protocol version constants used in negotiation with the server.
	ttcFldVsn231      byte = 17
	ttcFldVsn23_1Ext1 byte = 18
	ttcFldVsn23_1Ext3 byte = 20
	ttcFldVsn234      byte = 24
	ttcFldVsnMax      byte = ttcFldVsn234 // TTC_FLD_VSN_MAX is the latest supported version.

	sqlidLength byte = 13 // support SQL ID
)

// capability encodes both server and client-side negotiation state across compile and runtime capabilities.
type capability struct {
	runTimeCapabilities              []byte
	compileTimeCapabilities          []byte
	knownUsedCompileTimeCapabilities map[string]capabilityMetadata
	knownUsedRuntimeCapabilities     map[string]capabilityMetadata
}

var clientRuntimeCapabilites []byte
var clientCompileTimeCapabilities []byte

// newCapabilityMetadata Creates capabilities metadata
func newCapabilityMetadata() *capability {
	return &capability{
		// Compile time capabilities
		knownUsedCompileTimeCapabilities: map[string]capabilityMetadata{
			kpccapCtSQLVersion:             {index: 0, value: kpulmaxl, isFlag: false, isDefault: true},     // Not used default value
			kpccapCtXaUnderstandsBinaryXid: {index: 1, value: 0x01, isFlag: true, isDefault: true},          // Not used default value
			kpccapCtReplIntErrCre:          {index: 2, value: 0x00, isFlag: true, isDefault: true},          // Not used default value
			kpccapCtSidExtendedInitOra:     {index: 3, value: 0x00, isFlag: true, isDefault: true},          // Not used default value
			kztvovKpclogNoName:             {index: 4, value: 0x00, isFlag: true, isDefault: true},          // Capable of performing the O3LOGON protocol (on versions newer than 9.0.1).
			kztvovKpclogO3l:                {index: 4, value: 0x01, isFlag: true, isDefault: true},          // Capable of performing the O3LOGON protocol (on versions older than 9.0.1).
			kztvovKpclogO5lNp:              {index: 4, value: 0x02, isFlag: true, isDefault: true},          // Capable of understanding "unpaddable plaintext".
			kztvovKpclogO4l:                {index: 4, value: 0x04, isFlag: true, isDefault: true},          // Capable of performing the O4LOGON protocol.
			kztvovKpclogO5l:                {index: 4, value: 0x08, isFlag: true, isDefault: true},          // Capable of performing the O5LOGON protocol.
			kztvovKpclogO6l:                {index: 4, value: 0x10, isFlag: true, isDefault: true},          // Capable of O5LOGON and generating SHA-512 verifier.
			kztvovKpclogO7lMr:              {index: 4, value: 0x20, isFlag: true, isDefault: true},          // Capable of O5LOGON and multi-round PBKDF2 SHA-512.
			kpccapCtbPrefetchRows:          {index: 5, value: 0x04, isFlag: true, isDefault: true},          // capability to support server directive row prefetch
			kpccapCtbImplicitPool:          {index: 5, value: 0x08, isFlag: true, isDefault: true},          // capability to support DRCP implicit pooling
			kpccapCtbOauthmsgOnerr:         {index: 5, value: 0x10, isFlag: true, isDefault: true},          // return message for OAUTH RPC even on error
			kpccapCtScrollableCursor:       {index: 6, value: 1, isFlag: false, isDefault: true},            // Not used default value
			kpccapCtTtcFldVsn:              {index: 7, value: ttcFldVsnMax, isFlag: false, isDefault: true}, // Can the client and server understand the same anonymous fields: Latest supported version
			kpccapCt82pisal8:               {index: 8, value: 1, isFlag: false, isDefault: true},            // Not used default value
			kpccapCtDequeuewithselector:    {index: 9, value: 1, isFlag: false, isDefault: true},            // Not used default value
			kpccapCtTypeEvol:               {index: 10, value: 1, isFlag: false, isDefault: true},           // Not used default value
			kpccapCt2rt:                    {index: 11, value: 1, isFlag: false, isDefault: true},           // Not used default value
			kpccapCtClientidProp:           {index: 12, value: 1, isFlag: false, isDefault: true},           // Not used default value
			kpccapCtLobscnCache:            {index: 13, value: 1, isFlag: false, isDefault: true},           // Not used default value
			kpccapCtDescribeOpen:           {index: 14, value: 1, isFlag: false, isDefault: true},           // Not used default value
			kpccapCtbTtc1Eocs:              {index: 15, value: 0x01, isFlag: true, isDefault: true},         // does peer know end of call status
			kpccapCtbTtc1Fbvc:              {index: 15, value: 0x20, isFlag: true, isDefault: true},         // does peer know fast bvec protocol
			kpccapCtbTtc1Pblb:              {index: 15, value: 0x02, isFlag: true, isDefault: true},         // can peer handle piggybs on loopbacks?
			kpccapCtbTtc1Inrc:              {index: 15, value: 0x08, isFlag: true, isDefault: true},         // does peer know new ind/rcd protocol
			kpccapCtbOci1Fsap:              {index: 16, value: 0x10, isFlag: true, isDefault: true},         // fast session attribute propagate
			kpccapCtbOci1Apctx:             {index: 16, value: 0x80, isFlag: true, isDefault: true},         // app ctx fast piggy back
			kpccapCtObjectTdsver:           {index: 17, value: 3, isFlag: false, isDefault: true},           // Not used default value
			kpccapCtRpcver:                 {index: 18, value: 7, isFlag: false, isDefault: true},           // Not used default value
			kpccapCtRpcsig:                 {index: 19, value: 3, isFlag: false, isDefault: true},           // Not used default value
			kpccapCtRpcflgs:                {index: 20, value: 0, isFlag: false, isDefault: true},           // Not used default value
			kpccapCtDbfver:                 {index: 21, value: 1, isFlag: false, isDefault: true},           // Not used default value
			kpccapCtPlsql:                  {index: 22, value: 0, isFlag: false, isDefault: true},           // Not used default value
			koleLobCapUb8Size:              {index: 23, value: 0x01, isFlag: true, isDefault: true},         // flag used for UB8 LOB feature created in 10i.
			koleLobCapEncs:                 {index: 23, value: 0x02, isFlag: true, isDefault: true},         // flag indicating the server size default varying-width lob is stored as Endian-Neutral Character Set
			koleLobCapDil:                  {index: 23, value: 0x04, isFlag: true, isDefault: true},         // flag indicating that LOB data follows the LOB locator
			koleLobCapTmplocSz:             {index: 23, value: 0x08, isFlag: true, isDefault: true},         // Flag indicating that temporary LOB locator size is KOLBLTLMXL
			koleLobCapRemDataInt:           {index: 23, value: 0x10, isFlag: true, isDefault: true},         // flag indicating that define of remote LOB as char works (data interface)
			// "KOLE_LOB_CAP_ARRAY": {index: 23, flag: 0x20, isFlag: true}, // flag for LOB array read/write -> not included for some reason
			koleLobCapPrfch:                {index: 23, value: 0x40, isFlag: true, isDefault: true},          // LOB capabilities for 11g begin here
			koleLobCap12c:                  {index: 23, value: 0x80, isFlag: true, isDefault: true},          // Enables smart lob creation on the server side
			kpccapCtKpin:                   {index: 24, value: 1, isFlag: false, isDefault: true},            // Not used default value
			kpccapCtAqPropDqa:              {index: 25, value: 0x00, isFlag: true, isDefault: true},          // AQ not supported
			kpccapCtbTtc2Zlnp:              {index: 26, value: 0x04, isFlag: true, isDefault: true},          // can peer handle new key-val pairs
			kpccapCtUb2dty:                 {index: 27, value: 0x01, isFlag: true, isDefault: true},          // ub2 dty support
			kpccapCtReplLcrx2y:             {index: 28, value: 0x01, isFlag: true, isDefault: true},          // Streams LCRX2Y propagation capability
			kpccapCtAstr:                   {index: 29, value: 0x00, isFlag: true, isDefault: true},          // Not used default value
			kpccapCtObfuscate:              {index: 30, value: 0x00, isFlag: true, isDefault: true},          // Not used default value
			kpccapCtbOci2Cqc:               {index: 31, value: 0x04, isFlag: true, isDefault: true},          // remote handles OCI query caching
			kpccapCtbOci2Edition:           {index: 31, value: 0x08, isFlag: true, isDefault: true},          // application EDITION
			kpccapCtbOci2Srvcp:             {index: 31, value: 0x10, isFlag: true, isDefault: true},          // Server-side Connection pooling
			kpccapCtProxy:                  {index: 32, value: 0x00, isFlag: true, isDefault: true},          // Not used default value
			kpccapCtStrmsCca:               {index: 33, value: 0x00, isFlag: true, isDefault: true},          // Not used default value
			kpccapCtMaxocfn:                {index: 34, value: maxOcfn, isFlag: false, isDefault: true},      // understanding Server Piggyback Function code
			kpccapCtbOci3Ocssync:           {index: 35, value: 0x20, isFlag: true, isDefault: true},          // sessstate sync via s2c piggyback
			kpccapCtbOci3AppcontAuto:       {index: 35, value: 0x80, isFlag: true, isDefault: true},          // Client to send pdbuid when getting PDB's startup time in OFGI request
			kpccapCtbXMLCsxxmlt:            {index: 36, value: 0x01, isFlag: true, isDefault: true},          // client can decode CSX XMLType image
			kpccapCtbXMLLobstrImgOnly:      {index: 36, value: 0x02, isFlag: true, isDefault: true},          // client can understand only lob/string based images
			kpccapCtbTtc3Colmetadata:       {index: 37, value: 0x01, isFlag: true, isDefault: true},          // column metadata byte
			kpccapCtbTtc3Tzver:             {index: 37, value: 0x02, isFlag: true, isDefault: true},          // remote to send timezone version
			kpccapCtbTtc3Implres:           {index: 37, value: 0x10, isFlag: true, isDefault: true},          // Implicit Results feature, 12g
			kpccapCtbTtc3BigchunkClr:       {index: 37, value: 0x20, isFlag: true, isDefault: true},          // ttcclr supports big chunks
			kpccapCtbTtc3KeepOutOrder:      {index: 37, value: 0x80, isFlag: true, isDefault: true},          // preserve out bind order
			kpccapCtXstreamOut:             {index: 38, value: 0x00, isFlag: true, isDefault: true},          // XStream Out capability
			kpccapCtDtysesssignRecVsn:      {index: 39, value: ttcFldVsn231, isFlag: false, isDefault: true}, // 20.1 ext 1
			kpccapCtbTtc4FastReneg:         {index: 40, value: 0x02, isFlag: true, isDefault: true},          // can renegotiate post logon with 1 RPC
			kpccapCtbTtc4Ibnm:              {index: 40, value: 0x04, isFlag: true, isDefault: true},          // supports inband notifications
			kpccapCtbTtc4BigTztc:           {index: 40, value: 0x10, isFlag: true, isDefault: true},          // understands 4 byte length DST tables
			kpccapCtbTtc4ExplBound:         {index: 40, value: 0x40, isFlag: true, isDefault: true},          // Explicit request boundary support
			kpccapCtSqlidLength:            {index: 41, value: sqlidLength, isFlag: false, isDefault: true},
			koleLob2CapQuasi:               {index: 42, value: 0x01, isFlag: true, isDefault: true}, // flag used for V4 value based locator feature in 20c
			koleLob2CapVbl32:               {index: 42, value: 0x02, isFlag: true, isDefault: true}, // For Value based locators, assume default prefetch of 32K
			koleLob2Cap2gbPrefetch:         {index: 42, value: 0x04, isFlag: true, isDefault: true}, // LOB prefetch buffer can be up to 2G
			kpccapCtbShrdKeys:              {index: 43, value: 0x00, isFlag: true, isDefault: true}, // Not used default value
			kpccapCtbTtc5Vector:            {index: 44, value: 0x08, isFlag: true, isDefault: true}, // vector type supported
			kpccapCtbTtc5FlexTxn:           {index: 44, value: 0x10, isFlag: true, isDefault: true}, // supports Flex transaction
			kpccapCtbTtc5CqnPull:           {index: 44, value: 0x40, isFlag: true, isDefault: true}, // supports CQN pull model
			kpccapCtbTtc5PdbParams:         {index: 44, value: 0x80, isFlag: true, isDefault: true}, // container params
			kpccapCtbTtcspareAltsess:       {index: 45, value: 0x01, isFlag: true, isDefault: true}, // understands alter session pbk
			kpccapCtbFeatureBackportSpare2: {index: 46, value: 0x00, isFlag: true, isDefault: true},
			kpccapCtbFeatureBackportSpare3: {index: 47, value: 0x00, isFlag: true, isDefault: true},
			kpccapCtbFeatureBackportSpare4: {index: 48, value: 0x00, isFlag: true, isDefault: true},
			kpccapCtbFeatureBackportSpare5: {index: 49, value: 0x00, isFlag: true, isDefault: true},
			kpccapCtbFeatureBackportSpare6: {index: 50, value: 0x00, isFlag: true, isDefault: true},
			kpccapCtRaft:                   {index: 51, value: 0x00, isFlag: true, isDefault: true},
			kpccapCtVectorFeatureBinary:    {index: 52, value: 0x01, isFlag: true, isDefault: true}, // BINARY dimension type for VECTOR is supported
			kpccapCtVectorFeatureSparse:    {index: 52, value: 0x02, isFlag: true, isDefault: true}, // Sparse encoding format for VECTOR is supported
			kpccapCtResetState:             {index: 53, value: 0x35, isFlag: true, isDefault: true}, // supports reset of session state on connection reuse
		},
		// Runtime capabilities
		knownUsedRuntimeCapabilities: map[string]capabilityMetadata{
			kpccapRtCompat:                 {index: 0, value: 2, isFlag: false, isDefault: true},    // 8.1 compatibility
			kpccapRtTzEx:                   {index: 1, value: 0x01, isFlag: true, isDefault: true},  // Exchange time zone info
			kpccapRtKpf01:                  {index: 2, value: 0, isFlag: false, isDefault: true},    // Not used default value
			kpccapRtInstTyp:                {index: 3, value: 0, isFlag: true, isDefault: true},     // // Not used default value
			kpccapRtUb2Rep:                 {index: 4, value: 0, isFlag: true, isDefault: true},     // // Not used default value
			kpccapRtAsmVolSprt:             {index: 5, value: 0, isFlag: true, isDefault: true},     // // Not used default value
			kpccapRtbTtcZcpy:               {index: 6, value: 0x01, isFlag: true, isDefault: false}, // Zero copy (disabled by default)
			kpccapRtbTtcTzlt:               {index: 6, value: 0x02, isFlag: true, isDefault: true},  // understand localTime format for TSTZ
			kpccapRtbTtc32k:                {index: 6, value: 0x04, isFlag: true, isDefault: true},  // understand 32K VARCHAR
			kpccapRtbTtcCdb:                {index: 6, value: 0x08, isFlag: true, isDefault: true},  // connected to a CDB
			kpccapRtbTtcSessstateops:       {index: 6, value: 0x10, isFlag: true, isDefault: true},  // supports session state ops
			kpccapRtbTtcFeatureTrack:       {index: 6, value: 0x20, isFlag: true, isDefault: true},  // client feature tracking
			kpccapRtbTtcClientStats:        {index: 6, value: 0x40, isFlag: true, isDefault: true},  // client statistics
			kpccapRtbTtcSvrchksum:          {index: 6, value: 0x80, isFlag: true, isDefault: true},  // server checksum
			kpccapRtbMaxcols:               {index: 7, value: 0, isFlag: false, isDefault: true},    // column limit for table/view
			kpccapRtbTtc1Drcpv2:            {index: 8, value: 0x01, isFlag: true, isDefault: false}, // Not used
			kpccapRtbTtc1Iovoff:            {index: 8, value: 0x02, isFlag: true, isDefault: true},  // Dynamic Vectored IO
			kpccapRtbTtc1Mxstrsz:           {index: 8, value: 0x04, isFlag: true, isDefault: false}, // max varchar length
			kpccapRtbFeatureBackportSpare1: {index: 9, value: 0, isFlag: false, isDefault: true},    // Not used default value
			kpccapRtbFeatureBackportSpare2: {index: 10, value: 0, isFlag: false, isDefault: true},   // Not used default value
			kpccapRtFeatureBackportSpare3:  {index: 11, value: 0, isFlag: false, isDefault: true},   // Not used default value
			kpccapRtbOtelTrace:             {index: 12, value: 0x01, isFlag: true, isDefault: true}, // Supports OpenTelemetry Trace
			kpccapRtbOtelLogs:              {index: 12, value: 0x02, isFlag: true, isDefault: true}, // Supports OpenTelemetry Logs
			kpccapRtbOtelMetrics:           {index: 12, value: 0x04, isFlag: true, isDefault: true}, // Supports OpenTelemetry Metrics
		},
	}
}

// newCapability creates and initializes a new capability instance with default client capabilities.
func newCapability() *capability {
	capability := newCapabilityMetadata()
	capability.runTimeCapabilities = assumedSrvRtCaps
	capability.compileTimeCapabilities = assumedSrvCtCaps
	return capability
}

// newDefaultCapability creates and initializes a new capability instance with default client capabilities.
func newDefaultCapability() *capability {
	capability := newCapabilityMetadata()
	capability.compileTimeCapabilities = make([]byte, len(capability.knownUsedCompileTimeCapabilities))
	// build compile time capabilities from metadata
	for _, value := range capability.knownUsedCompileTimeCapabilities {
		if value.isDefault {
			capability.compileTimeCapabilities[value.index] = capability.compileTimeCapabilities[value.index] | value.value
		}
	}
	// build runtime capabilities from metadata
	capability.runTimeCapabilities = make([]byte, len(capability.knownUsedRuntimeCapabilities))
	for _, value := range capability.knownUsedRuntimeCapabilities {
		if value.isDefault {
			capability.runTimeCapabilities[value.index] = capability.runTimeCapabilities[value.index] | value.value
		}
	}
	return capability
}

// UnMarshalFrom reads server capabilities from the marshal engine.
func (cap *capability) UnMarshalFrom(ctx context.Context, mar driverCommon.Marshaller) error {
	// Read server's compile time capabilities
	length, err := mar.UnmarshalUB1(ctx)
	if err != nil {
		return err
	}

	cap.compileTimeCapabilities = make([]byte, length)
	for i := range cap.compileTimeCapabilities {
		val, err := mar.UnmarshalUB1(ctx)
		if err != nil {
			common.Odl.Warn("Failed to unmarshal compile time capabilities", "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, "Capability")
		}
		cap.compileTimeCapabilities[i] = byte(val)
	}

	// Read runtime capabilities
	length, err = mar.UnmarshalUB1(ctx)
	if err != nil {
		common.Odl.Warn("Failed to unmarshal runtime capabilities", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "Capability")
	}
	if length > 0 {
		cap.runTimeCapabilities = make([]byte, length)
		for i := range cap.runTimeCapabilities {
			val, err := mar.UnmarshalUB1(ctx)
			if err != nil {
				common.Odl.Warn("Failed to unmarshal runtime capabilities", "error", err)
				return common.NewOracleError(oracleErrors.FailUnmarshal, err, "Capability")
			}
			cap.runTimeCapabilities[i] = byte(val)
		}
	}
	return nil
}

// MarshalTo writes capabilities to the marshal engine.
func (cap *capability) MarshalTo(ctx context.Context, mar driverCommon.Marshaller) error {
	if err := mar.MarshalUB1(ctx, driverCommon.UB1(len(cap.compileTimeCapabilities))); err != nil {
		common.Odl.Warn("Failed to marshal compile time capabilities", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "Capability")
	}

	if err := mar.MarshalB1Array(ctx, (driverCommon.B1Array)(cap.compileTimeCapabilities)); err != nil {
		common.Odl.Warn("Failed to marshal compile time capabilities", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "Capability")
	}

	if err := mar.MarshalUB1(ctx, driverCommon.UB1(len(cap.runTimeCapabilities))); err != nil {
		common.Odl.Warn("Failed to marshal runtime capabilities", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "Capability")
	}

	if err := mar.MarshalB1Array(ctx, (driverCommon.B1Array)(cap.runTimeCapabilities)); err != nil {
		common.Odl.Warn("failed to marshal runtime capabilities", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "Capability")
	}
	return nil
}

// adjustCapabilityFrom updates the client's capability fields to align with those presented by the server.
// This ensures compatibility during TTC protocol negotiation by adapting client compile-time and
// runtime capability settings according to server support and restrictions.
// It is typically called after server capabilities have been unmarshalled,
// so that the client can disable or enable features as needed before further negotiation.
func (cap *capability) adjustCapabilityFrom(serverCap *capability) {
	kpccapCtUb2DtyIndex := int(cap.knownUsedCompileTimeCapabilities[kpccapCtUb2dty].index)
	if len(serverCap.compileTimeCapabilities) > kpccapCtUb2DtyIndex && serverCap.compileTimeCapabilities[kpccapCtUb2DtyIndex] == 0 {
		cap.compileTimeCapabilities[kpccapCtUb2DtyIndex] = 0
	}

	kpccapRtTz := cap.knownUsedRuntimeCapabilities[kpccapRtTzEx]
	if len(serverCap.runTimeCapabilities) <= int(kpccapRtTz.index) ||
		(serverCap.runTimeCapabilities[kpccapRtTz.index]&kpccapRtTz.value) != kpccapRtTz.value {
		cap.runTimeCapabilities[kpccapRtTz.index] &= ^kpccapRtTz.value
	}

	kpccapRtbTtc := cap.knownUsedRuntimeCapabilities[kpccapRtbTtc32k]
	cap.runTimeCapabilities[kpccapRtbTtc.index] |= kpccapRtbTtc.value

	kpccapCtbTtc3Index := int(cap.knownUsedCompileTimeCapabilities[kpccapCtbTtc3Tzver].index)
	if len(serverCap.compileTimeCapabilities) > kpccapCtbTtc3Index && serverCap.compileTimeCapabilities[kpccapCtbTtc3Index] != cap.knownUsedCompileTimeCapabilities[kpccapCtbTtc3Tzver].value {
		cap.compileTimeCapabilities[kpccapCtbTtc3Index] &= ^cap.knownUsedCompileTimeCapabilities[kpccapCtbTtc3Tzver].value
		cap.runTimeCapabilities[kpccapRtTz.index] &= ^kpccapRtTz.value
	}
}

func (cap *capability) toMap() map[string]driverCommon.Capability {
	totalCount := len(cap.knownUsedCompileTimeCapabilities) + len(cap.knownUsedRuntimeCapabilities)
	var capabilityMap = make(map[string]driverCommon.Capability, totalCount)
	capabilitiesToMap(cap.compileTimeCapabilities, cap.knownUsedCompileTimeCapabilities, capabilityMap)
	capabilitiesToMap(cap.runTimeCapabilities, cap.knownUsedRuntimeCapabilities, capabilityMap)
	return capabilityMap
}

func capabilitiesToMap(capabilities []byte, metadata map[string]capabilityMetadata, capabilityMap map[string]driverCommon.Capability) {
	for key, value := range metadata {
		if int(value.index) < len(capabilities) {
			var capability driverCommon.Capability
			if value.isFlag {
				capability = driverCommon.Capability{
					Value: capabilities[value.index] & value.value,
					IsSet: (capabilities[value.index] & value.value) != 0,
				}
			} else {
				capability = driverCommon.Capability{
					Value: capabilities[value.index],
					IsSet: capabilities[value.index] != 0,
				}
			}
			capabilityMap[key] = capability
		}
	}
}
