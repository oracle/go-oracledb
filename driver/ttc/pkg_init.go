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
	"container/list"
	"database/sql/driver"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strconv"
	"time"

	"github.com/oracle/go-oracledb/driver/common"
	"github.com/oracle/go-oracledb/driver/ttc/converters"
)

const MinTTCProtocolVersion = 12 // 19.1

var currentDriverName common.B1Array
var currentDriverExternalName = common.StringToB1Array(driverNameDefault + "_" + driverDefaultResourceManagerID)
var currentDriverACLValue = common.StringToB1Array(defaultACLValue)
var currentDriverInternalName = common.StringToB1Array("go_ttc_impl")

var _authTerminalKey = common.StringToB1Array(authTerminal)
var _authConnectStringKey = common.StringToB1Array(authConnectString)
var _authProgramNmKey = common.StringToB1Array(authProgramNm)

var _authMachineKey = common.StringToB1Array(authMachine)
var _authPidKey = common.StringToB1Array(authPid)
var _authACLKey = common.StringToB1Array(authACL)
var _authClientCapabilitiesKey = common.StringToB1Array(authClientCapabilities)
var _authClientCapabilitiesVal = common.StringToB1Array(strconv.Itoa(0x00000001 + 0x00000002))

// static information used by oauth message
var _keyValStaticInfoForOsesskey = list.New()
var _keyValStaticInfoForOAuth1 = list.New()
var _keyValStaticInfoForOAuth2 = list.New()
var _keyValStaticInfoForOAuthConnectString *list.Element

var _dummyTerminalName = common.StringToB1Array("unknown")
var currentUserName common.B1Array

// _initEnvironmentStaticInformation initializes static values from the current environment.
func _initEnvironmentStaticInformation() {

	currentTerminal := common.StringToB1Array("unknown")
	currentDriverName = common.StringToB1Array(driverNameDefault + " : " + common.DriverVersion)

	if u, err := user.Current(); err == nil {
		currentUserName = common.StringToB1Array(u.Username)
	} else {
		common.Odl.Info(fmt.Sprintf("using default as user name"))
		currentUserName = common.StringToB1Array("unknown")
	}

	var currentProcessPath common.B1Array

	if e, err := os.Executable(); err == nil {
		currentProcessPath = common.StringToB1Array(filepath.Base(e))
	} else {
		common.Odl.Info(fmt.Sprintf("using default as process path"))
		currentProcessPath = common.StringToB1Array("unknown")
	}
	var currentProcessId common.B1Array
	currentProcessId = common.StringToB1Array(strconv.Itoa(os.Getpid()))
	var currentMachineName common.B1Array
	if h, err := os.Hostname(); err == nil {
		currentMachineName = common.StringToB1Array(h)
	} else {
		common.Odl.Info(fmt.Sprintf("using default as machine name"))
		currentMachineName = common.StringToB1Array("oraclegoclient")
	}
	currentDriverInternalName = common.StringToB1Array(driverInternalName)

	_keyValStaticInfoForOsesskey.PushBack(&common.KeyValue{Key: common.StringToB1Array(authTerminal), Value: currentTerminal})
	_keyValStaticInfoForOsesskey.PushBack(&common.KeyValue{Key: common.StringToB1Array(authProgramNm), Value: currentProcessPath})
	_keyValStaticInfoForOsesskey.PushBack(&common.KeyValue{Key: common.StringToB1Array(authMachine), Value: currentMachineName})
	_keyValStaticInfoForOsesskey.PushBack(&common.KeyValue{Key: common.StringToB1Array(authPid), Value: currentProcessId})
	_keyValStaticInfoForOsesskey.PushBack(&common.KeyValue{Key: common.StringToB1Array(authSid), Value: currentUserName})

	_keyValStaticInfoForOAuth1.PushBack(&common.KeyValue{Key: _authTerminalKey, Value: _dummyTerminalName})
	_keyValStaticInfoForOAuth1.PushBack(&common.KeyValue{Key: _authConnectStringKey, Value: nil})
	_keyValStaticInfoForOAuthConnectString = _keyValStaticInfoForOAuth1.Back()
	_keyValStaticInfoForOAuth1.PushBack(&common.KeyValue{Key: _authProgramNmKey, Value: currentProcessPath})

	_keyValStaticInfoForOAuth2.PushBack(&common.KeyValue{Key: _authMachineKey, Value: currentMachineName})
	_keyValStaticInfoForOAuth2.PushBack(&common.KeyValue{Key: _authPidKey, Value: currentProcessId})
	_keyValStaticInfoForOAuth2.PushBack(&common.KeyValue{Key: _authACLKey, Value: currentDriverACLValue})
	_keyValStaticInfoForOAuth2.PushBack(&common.KeyValue{Key: _authClientCapabilitiesKey, Value: _authClientCapabilitiesVal})

}

// internal map for SQL statemenet qualification
var _sqlKindMap = make(map[string]sqlKind)

func init() {

	_initEnvironmentStaticInformation()

	// ========================= TTC MESSAGE Registry =========================
	// Registers all TTC messages. All TTC Message implementors must be registered
	// here.
	// Register MessageType → Message implementations in the TTCRegistry
	err := MessageRegistry.Register(TTIPRO, -1, NewTTIpro)
	if err != nil {
		// Handle error appropriately, e.g., log or panic
		common.Odl.Warn("Failed to register message TTIPro", "error", err)
	}

	err = MessageRegistry.Register(TTIDTY, -1, NewTTIdty)
	if err != nil {
		common.Odl.Warn("Failed to register message tTIdty", "error", err)
	}

	err = MessageRegistry.Register(TTIOER, 14, NewTTIoer14)
	if err != nil {
		common.Odl.Warn("Failed to register message TTIOER version 2", "error", err)
	}

	err = MessageRegistry.Register(TTIOER, MinTTCProtocolVersion, NewTTIoer)
	if err != nil {
		common.Odl.Warn("Failed to register message TTIOER version 1", "error", err)
	}

	err = MessageRegistry.Register(TTIDCB, 24, newTTIdcb24)
	if err != nil {
		common.Odl.Warn("Failed to register message TTIDCB version 4", "error", err)
	}

	err = MessageRegistry.Register(TTIDCB, 20, newTTIdcb20)
	if err != nil {
		common.Odl.Warn("Failed to register message TTIDCB version 3", "error", err)
	}

	err = MessageRegistry.Register(TTIDCB, 17, newTTIdcb17)
	if err != nil {
		common.Odl.Warn("Failed to register message TTIDCB version 2", "error", err)
	}

	err = MessageRegistry.Register(TTIDCB, MinTTCProtocolVersion, newTTIdcb)
	if err != nil {
		common.Odl.Warn("Failed to register message TTIDCB version 1", "error", err)
	}

	err = MessageRegistry.Register(TTIRXH, MinTTCProtocolVersion, newTTIrxh)
	if err != nil {
		common.Odl.Warn("Failed to register message TTIRXH", "error", err)
	}

	err = MessageRegistry.Register(TTIRXD, MinTTCProtocolVersion, newTTIrxd)
	if err != nil {
		common.Odl.Warn("Failed to register message TTIRXD", "error", err)
	}

	err = MessageRegistry.Register(TTIBVC, MinTTCProtocolVersion, newTTIbvc)
	if err != nil {
		common.Odl.Warn("Failed to register message TTIBVC", "error", err)
	}

	// Register status functions
	err = MessageRegistry.Register(TTISTA, -1, newTTISTA)
	if err != nil {
		common.Odl.Warn("Failed to register STA function", "error", err)
	}

	err = MessageRegistry.Register(TTILOBD, MinTTCProtocolVersion, newTTIlobd)
	if err != nil {
		common.Odl.Warn("Failed to register LOBD function", "error", err)
	}

	err = MessageRegistry.Register(TTIIOV, MinTTCProtocolVersion, newTTIiov)
	if err != nil {
		common.Odl.Warn("Failed to register TTIIOV", "error", err)
	}

	err = MessageRegistry.Register(TTIWRN, MinTTCProtocolVersion, newTTIwrn)
	if err != nil {
		common.Odl.Warn("Failed to register TTIWRN", "error", err)
	}

	err = MessageRegistry.Register(TTIFOB, MinTTCProtocolVersion, newTTIfob)
	if err != nil {
		common.Odl.Warn("Failed to register TTIFOB", "error", err)
	}

	// ========================= FUNCTION -> MESSAGE Registry =========================
	// Register FunctionType → Message implementations
	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oauth}, 18, NewOAuth18)
	if err != nil {
		common.Odl.Warn("Failed to register function oauth", "error", err)
	}

	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oauth}, MinTTCProtocolVersion, NewOAuth)
	if err != nil {
		common.Odl.Warn("Failed to register function oauth", "error", err)
	}

	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oSesskey}, 18, NewOSesskey18)
	if err != nil {
		common.Odl.Warn("Failed to register function oSesskey", "error", err)
	}

	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oSesskey}, MinTTCProtocolVersion, NewOSesskey)
	if err != nil {
		common.Odl.Warn("Failed to register function oSesskey", "error", err)
	}

	// Register OALL8 execute/query function
	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, 18, NewOall18)
	if err != nil {
		common.Odl.Warn("Failed to register function oAll8", "error", err)
	}

	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, MinTTCProtocolVersion, NewOall)
	if err != nil {
		common.Odl.Warn("Failed to register function oAll8", "error", err)
	}

	// Register OEXFEN fast-path execute+fetch function
	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oExfen}, MinTTCProtocolVersion, NewOexfen)
	if err != nil {
		common.Odl.Warn("Failed to register function oExfen", "error", err)
	}
	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oExfen}, 18, NewOexfen18)
	if err != nil {
		common.Odl.Warn("Failed to register function oExfen", "error", err)
	}

	// Register oauth function response handler
	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIRPA, functionType: oauth}, MinTTCProtocolVersion, NewOAuthRPA)
	if err != nil {
		common.Odl.Warn("Failed to register oauth function reply", "error", err)
	}

	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIRPA, functionType: oSesskey}, MinTTCProtocolVersion, NewOSesskeyRPA)
	if err != nil {
		common.Odl.Warn("Failed to register oSessionKey function reply", "error", err)
	}

	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTISPF, functionType: common.FunctionType(ocssync)}, -1, NewttiSPFOCSSync)
	if err != nil {
		common.Odl.Warn("Failed to register SPF function", "error", err)
	}

	// Register logOff functions (used to disconnect from the database)
	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: logOff}, 18, newLogOff18)
	if err != nil {
		common.Odl.Warn("Failed to register LogOff function", "error", err)
	}

	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: logOff}, MinTTCProtocolVersion, newLogOff)
	if err != nil {
		common.Odl.Warn("Failed to register LogOff function", "error", err)
	}

	// Register ping functions
	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: ping}, 18, newPing18)
	if err != nil {
		common.Odl.Warn("Failed to register ping function", "error", err)
	}

	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: ping}, MinTTCProtocolVersion, newPing)
	if err != nil {
		common.Odl.Warn("Failed to register ping function", "error", err)
	}

	// Register OCCA cursorId close/cancel function
	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIPFN, functionType: occa}, 18, newOcca18)
	if err != nil {
		common.Odl.Warn("Failed to register function occa", "error", err)
	}
	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIPFN, functionType: occa}, MinTTCProtocolVersion, newOcca)
	if err != nil {
		common.Odl.Warn("Failed to register function occa", "error", err)
	}

	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIRPA, functionType: oAll8}, MinTTCProtocolVersion, newTTIOallRPA)
	if err != nil {
		common.Odl.Warn("Failed to register OAll8 function reply", "error", err)
	}

	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oLobOps}, MinTTCProtocolVersion, newTTIlob)
	if err != nil {
		common.Odl.Warn("Failed to register function oLobOps", "error", err)
	}

	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIRPA, functionType: oLobOps}, MinTTCProtocolVersion, newTTILobRPA)
	if err != nil {
		common.Odl.Warn("Failed to register OLobOps function reply RPA ", "error", err)
	}

	// Register commit functions (used to commit a transaction)
	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: commit}, MinTTCProtocolVersion, newCommit)
	if err != nil {
		common.Odl.Warn("Failed to register Commit function", "error", err)
	}

	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: commit}, 18, newCommit18)
	if err != nil {
		common.Odl.Warn("Failed to register Commit function", "error", err)
	}

	// Register rollback functions (used to rollback a transaction)
	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: rollback}, MinTTCProtocolVersion, newRollback)
	if err != nil {
		common.Odl.Warn("Failed to register Commit function", "error", err)
	}

	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: rollback}, 18, newRollback18)
	if err != nil {
		common.Odl.Warn("Failed to register Commit function", "error", err)
	}

	// Initialize our type representation table
	// _oSessionKeyInit initializes the typeAndRep mapping for data types.
	// It sets up scalar and record types with their respective representations.

	typeRepresentationTable.SetFlags(TTCLXMULTI)
	typeRepresentationTable.addTypeRepToTable(common.DtyChr, common.DtyChr, int16(RepCUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyNum, common.DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(common.DtyBol, common.DtyBol, int16(RepIUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyLng, common.DtyLng, int16(RepCUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDat, common.DtyDat, int16(RepDV51))
	typeRepresentationTable.addTypeRepToTable(common.DtyBin, common.DtyBin, int16(RepBUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyLbi, common.DtyLbi, int16(RepBUnv))
	// data types for moving structures etc across the interface
	// integer data types
	typeRepresentationTable.addTypeRepToTable(common.DtyUb2, common.DtyUb2, int16(RepIUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyUb4, common.DtyUb4, int16(RepIUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyB1, common.DtyB1, int16(RepIUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyB2, common.DtyB2, int16(RepIUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyB4, common.DtyB4, int16(RepIUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyWord, common.DtyWord, int16(RepIUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyUword, common.DtyUword, int16(RepIUnv))
	// pointer data types
	typeRepresentationTable.addTypeRepToTable(common.DtyPb, common.DtyPb, int16(RepAUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyPw, common.DtyPw, int16(RepAUnv))

	// next send the records
	typeRepresentationTable.addTypeRepToTable(common.DtyTi5, common.DtyTi5, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRiD, common.DtyRiD, int16(RepRUnv))
	// opidef program interface request block types
	typeRepresentationTable.addTypeRepToTable(common.DtyAms, common.DtyAms, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyBrn, common.DtyBrn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyCwd, common.DtyCwd, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyNac122, common.DtyNac122, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyOer8, common.DtyOer8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyFun, common.DtyFun, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAua, common.DtyAua, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRxh7, common.DtyRxh7, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyNa6, common.DtyNa6, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyBrp, common.DtyBrp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyBrv, common.DtyBrv, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKva, common.DtyKva, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyCls, common.DtyCls, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyCui, common.DtyCui, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDfn, common.DtyDfn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDqr, common.DtyDqr, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsc, common.DtyDsc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyExe, common.DtyExe, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyFch, common.DtyFch, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyGbv, common.DtyGbv, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyGem, common.DtyGem, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyGiv, common.DtyGiv, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyOkg, common.DtyOkg, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyHmi, common.DtyHmi, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyIno, common.DtyIno, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyLnf, common.DtyLnf, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyOnt, common.DtyOnt, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyOpe, common.DtyOpe, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyOsq, common.DtyOsq, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtySfe, common.DtySfe, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtySpf, common.DtySpf, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyVsn, common.DtyVsn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyUd7, common.DtyUd7, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsa, common.DtyDsa, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyPin, common.DtyPin, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyPfn, common.DtyPfn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyPpt, common.DtyPpt, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtySto, common.DtySto, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyArc, common.DtyArc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyMrs, common.DtyMrs, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyMrt, common.DtyMrt, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyMrg, common.DtyMrg, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyMrr, common.DtyMrr, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyMrc, common.DtyMrc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyVer, common.DtyVer, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyLon2, common.DtyLon2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyIno2, common.DtyIno2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAll, common.DtyAll, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyUdb, common.DtyUdb, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAqi, common.DtyAqi, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyUlb, common.DtyUlb, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyUld, common.DtyUld, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtySid, common.DtySid, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyNa7, common.DtyNa7, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAl7, common.DtyAl7, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyK2Rpc, common.DtyK2Rpc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXdp, common.DtyXdp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyOko8, common.DtyOko8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyUd12, common.DtyUd12, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAl8, common.DtyAl8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyLfop, common.DtyLfop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyFcrt, common.DtyFcrt, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDny, common.DtyDny, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyOpr, common.DtyOpr, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyPls, common.DtyPls, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXid, common.DtyXid, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyTxn, common.DtyTxn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDcb, common.DtyDcb, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyCca, common.DtyCca, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyWrn, common.DtyWrn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyTlh121, common.DtyTlh121, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyToh121, common.DtyToh121, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyFoi, common.DtyFoi, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtySid2, common.DtySid2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyTch, common.DtyTch, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyPii, common.DtyPii, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyPfi, common.DtyPfi, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyPpu, common.DtyPpu, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyPte, common.DtyPte, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRxh8, common.DtyRxh8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyTn12, common.DtyTn12, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAuth, common.DtyAuth, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKval, common.DtyKval, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyFgi, common.DtyFgi, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsy, common.DtyDsy, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyR8, common.DtyDsyR8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyH8, common.DtyDsyH8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyL, common.DtyDsyL, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyT8, common.DtyDsyT8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyV8, common.DtyDsyV8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyP, common.DtyDsyP, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyF, common.DtyDsyF, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyK, common.DtyDsyK, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyY, common.DtyDsyY, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyQ, common.DtyDsyQ, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyC, common.DtyDsyC, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyA, common.DtyDsyA, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyOt8, common.DtyOt8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyTy, common.DtyDsyTy, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAqe, common.DtyAqe, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKv, common.DtyKv, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAqd, common.DtyAqd, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAQ8, common.DtyAQ8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRfs, common.DtyRfs, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRxh10, common.DtyRxh10, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpn, common.DtyKpn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpdnr, common.DtyKpdnr, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyD, common.DtyDsyD, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyS, common.DtyDsyS, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyR, common.DtyDsyR, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyH, common.DtyDsyH, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyT, common.DtyDsyT, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDsyV, common.DtyDsyV, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAqm, common.DtyAqm, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyOer11, common.DtyOer11, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAql, common.DtyAql, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyOtc, common.DtyOtc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKfno, common.DtyKfno, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKfnp, common.DtyKfnp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyOkgt8, common.DtyOkgt8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRaSb4, common.DtyRaSb4, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRaUb2, common.DtyRaUb2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRaUb1, common.DtyRaUb1, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRaTxt, common.DtyRaTxt, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRsSb4, common.DtyRsSb4, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRsUb2, common.DtyRsUb2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRsUb1, common.DtyRsUb1, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRsTxt, common.DtyRsTxt, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRidl, common.DtyRidl, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyGlrdd, common.DtyGlrdd, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyGlrdg, common.DtyGlrdg, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyGlrdc, common.DtyGlrdc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyOko, common.DtyOko, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDpp, common.DtyDpp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDpls, common.DtyDpls, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDpmop, common.DtyDpmop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyStat, common.DtyStat, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRfx, common.DtyRfx, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyFal, common.DtyFal, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyCkv, common.DtyCkv, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDrcx, common.DtyDrcx, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKgh, common.DtyKgh, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAqo, common.DtyAqo, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyOkgt, common.DtyOkgt, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpfc, common.DtyKpfc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyFe2, common.DtyFe2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtySpfp, common.DtySpfp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDpuls, common.DtyDpuls, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAqa, common.DtyAqa, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpbf, common.DtyKpbf, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyTsm, common.DtyTsm, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyMss, common.DtyMss, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpc, common.DtyKpc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyCrs, common.DtyCrs, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKks, common.DtyKks, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKsp, common.DtyKsp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKspTop, common.DtyKspTop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKspVal, common.DtyKspVal, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyPss, common.DtyPss, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyNls, common.DtyNls, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAls, common.DtyAls, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKsdEvtVal, common.DtyKsdEvtVal, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKsdEvtTop, common.DtyKsdEvtTop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpspp, common.DtyKpspp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKol, common.DtyKol, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyLst, common.DtyLst, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAcx, common.DtyAcx, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyScs, common.DtyScs, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRxh, common.DtyRxh, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpdns, common.DtyKpdns, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpdcn, common.DtyKpdcn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpnns, common.DtyKpnns, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpncn, common.DtyKpncn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKps, common.DtyKps, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyApinf, common.DtyApinf, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyTen, common.DtyTen, int16(RepRUnv))

	typeRepresentationTable.addTypeRepToTable(common.DtyXsscs, common.DtyXsscs, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXssro, common.DtyXssro, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXsspo, common.DtyXsspo, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKsrpc, common.DtyKsrpc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKvl, common.DtyKvl, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXsssDef, common.DtyXsssDef, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpdqcInv, common.DtyKpdqcInv, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpdqIdc, common.DtyKpdqIdc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpdqcSta, common.DtyKpdqcSta, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKprs, common.DtyKprs, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpdqcID, common.DtyKpdqcID, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRtstrm, common.DtyRtstrm, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtySessGet, common.DtySessGet, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtySessRls, common.DtySessRls, int16(RepRUnv))
	// server to client piggyback:
	typeRepresentationTable.addTypeRepToTable(common.DtySessRet, common.DtySessRet, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyScn6, common.DtyScn6, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKecpa, common.DtyKecpa, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKecpp, common.DtyKecpp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtySxa, common.DtySxa, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKvarr, common.DtyKvarr, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpngn, common.DtyKpngn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyInt, common.DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(common.DtyFlt, common.DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(common.DtyStr, common.DtyChr, int16(RepCUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyVnu, common.DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(common.DtyPdn, common.DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(common.DtyVCS, common.DtyChr, int16(RepCUnv))
	// some internal data types. not internal, and not kernel
	typeRepresentationTable.addTypeRepToTable(common.DtyIdt, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyIju, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyVbi, common.DtyBin, int16(RepBUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDif, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyDof, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyDtz, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyDyn, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyDpc, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyBfloat, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyBdouble, common.Dty0, int16(0))
	// structure data types - used for TTI messages
	// oracle version of uac - one form for native, one for network
	typeRepresentationTable.addTypeRepToTable(common.DtyOac, common.DtyNac, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyOpq, common.Dty0, int16(0)) // Opaque

	typeRepresentationTable.addTypeRepToTable(common.DtyUin, common.DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(common.DtyBri, common.Dty0, int16(0))
	// Array - recycling common.Dty70 - %TEMPORARY%
	typeRepresentationTable.addTypeRepToTable(common.DtyArr, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyOcu, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyVar, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtySls, common.DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(common.DtyLvc, common.DtyChr, int16(RepCUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyLvb, common.DtyBin, int16(RepBUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAfc, common.DtyAfc, int16(RepCUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAvc, common.DtyAfc, int16(RepCUnv))
	// The datatype for canonical format binary_float is lfp_cf
	// The datatype for canonical format binary_double is lfp_cd
	typeRepresentationTable.addTypeRepToTable(common.DtyIbFloat, common.DtyIbFloat, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyIbDouble, common.DtyIbDouble, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyCur, common.DtyCur, int16(RepUnv))
	// direct path Export
	typeRepresentationTable.addTypeRepToTable(common.DtyRdd, common.DtyRiD, int16(RepUnv))
	// datatypes for labels
	typeRepresentationTable.addTypeRepToTable(common.DtyLab, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyOsl, common.DtyOsl, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyNty, common.DtyINty, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyINty, common.DtyINty, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRef, common.DtyIref, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyIref, common.DtyIref, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyClob, common.DtyClob, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyBlob, common.DtyBlob, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyBFil, common.DtyBFil, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyCFil, common.DtyCFil, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyRSet, common.DtyCur, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtySvt, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyJSON, common.DtyJSON, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAdt, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyNtb, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyNar, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyVec, common.DtyVec, int16(RepUnv)) // 23.4 Vector
	typeRepresentationTable.addTypeRepToTable(common.DtyObj, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyClv, common.DtyClv, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyBlv, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyDtr, common.DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(common.DtyDun, common.DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(common.DtyDop, common.DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(common.DtyVst, common.DtyChr, int16(RepCUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyOdt, common.DtyDat, int16(RepDV51))
	typeRepresentationTable.addTypeRepToTable(common.DtyDol, common.DtyNum, int16(RepNV51))
	// old byte array stops here
	typeRepresentationTable.addTypeRepToTable(common.DtyTime, common.DtyTime, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyTtz, common.DtyTtz, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyStamp, common.DtyStamp, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyStz, common.DtyStz, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyIym, common.DtyIym, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyIds, common.DtyIds, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyEdate, common.DtyDat, int16(RepDV51))
	typeRepresentationTable.addTypeRepToTable(common.DtyEtime, common.DtyEtime, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyEttz, common.DtyEttz, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyEstamp, common.DtyEstamp, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyEstz, common.DtyEstz, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyEiym, common.DtyEiym, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyEids, common.DtyEids, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyLdiIf, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyLdiOf, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtyDclob, common.DtyClob, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDblob, common.DtyBlob, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDjson, common.DtyJSON, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyDbfil, common.DtyBFil, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyBuri, common.DtyBuri, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyPsr, common.Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(common.DtySitz, common.DtySitz, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyEsitz, common.DtySitz, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyUb8, common.DtyUb8, int16(RepIUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyPnty, common.DtyINty, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAbs, common.Dty0, int16(0))

	// !!!! NEW RECORDS SHOULD BE INSERTED HERE !!!!
	typeRepresentationTable.addTypeRepToTable(common.DtyXsnsop, common.DtyXsnsop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXsattr, common.DtyXsattr, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXsns, common.DtyXsns, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyUb1Array, common.DtyUb1Array, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtySessState, common.DtySessState, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAppContReplay, common.DtyAppContReplay, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAppContCtl, common.DtyAppContCtl, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtySessSign, common.DtySessSign, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyImplRes, common.DtyImplRes, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyOer, common.DtyOer, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpdxft, common.DtyKpdxft, int16(RepRUnv))

	// 20c
	typeRepresentationTable.addTypeRepToTable(common.DtyShrdKeySync, common.DtyShrdKeySync, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyOer19, common.DtyOer19, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpdssTemplate, common.DtyKpdssTemplate, int16(RepRUnv))

	// 23ai
	typeRepresentationTable.addTypeRepToTable(common.DtySaga, common.DtySaga, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyUd21, common.DtyUd21, int16(RepRUnv))

	// New records for triton 12c:
	typeRepresentationTable.addTypeRepToTable(common.DtyTxt, common.DtyTxt, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXssessns, common.DtyXssessns, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXsattop, common.DtyXsattop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXscreop, common.DtyXscreop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXsdetop, common.DtyXsdetop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXsdesop, common.DtyXsdesop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXssetsp, common.DtyXssetsp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXssidp, common.DtyXssidp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXsprin, common.DtyXsprin, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXskvl, common.DtyXskvl, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXsssdef2, common.DtyXsssdef2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXsnsop2, common.DtyXsnsop2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyXsns2, common.DtyXsns2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtykpdnrEq, common.DtykpdnrEq, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtykpdnrNf, common.DtykpdnrNf, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpngnc, common.DtyKpngnc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyKpnri, common.DtyKpnri, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAqEnq, common.DtyAqEnq, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAqDeq, common.DtyAqDeq, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyAqJms, common.DtyAqJms, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtykpdnrPay, common.DtykpdnrPay, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtykpdnrAck, common.DtykpdnrAck, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtykpdnrMp, common.DtykpdnrMp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtykpdnrDq, common.DtykpdnrDq, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyScn, common.DtyScn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyScn8, common.DtyScn8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyChunkInfo, common.DtyChunkInfo, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyUds, common.DtyUds, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyTnp, common.DtyTnp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyTlh, common.DtyTlh, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyToh, common.DtyToh, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtySnp, common.DtySnp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyNac, common.DtyNac, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyPlend, common.DtyPlend, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyPlbgn, common.DtyPlbgn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(common.DtyPlopn, common.DtyPlopn, int16(RepRUnv))

	// ========================= TYPE CODEC Registry =========================
	// Register default encoders.
	if err := EncoderRegistry.Register(reflect.TypeOf(""), MinTTCProtocolVersion, converters.EncodeVarchar); err != nil {
		common.Odl.Warn("Failed to register string encoder", "error", err)
	}
	if err := EncoderRegistry.Register(reflect.TypeOf(int16(0)), MinTTCProtocolVersion, converters.EncodeInt); err != nil {
		common.Odl.Warn("Failed to register int16 encoder", "error", err)
	}
	if err := EncoderRegistry.Register(reflect.TypeOf(int32(0)), MinTTCProtocolVersion, converters.EncodeInt); err != nil {
		common.Odl.Warn("Failed to register int32 encoder", "error", err)
	}
	if err := EncoderRegistry.Register(reflect.TypeOf(int(0)), MinTTCProtocolVersion, converters.EncodeInt); err != nil {
		common.Odl.Warn("Failed to register int encoder", "error", err)
	}
	if err := EncoderRegistry.Register(reflect.TypeOf(int64(0)), MinTTCProtocolVersion, converters.EncodeInt); err != nil {
		common.Odl.Warn("Failed to register int64 encoder", "error", err)
	}
	if err := EncoderRegistry.Register(reflect.TypeOf(int8(0)), MinTTCProtocolVersion, converters.EncodeInt); err != nil {
		common.Odl.Warn("Failed to register int8 encoder", "error", err)
	}

	if err := EncoderRegistry.Register(reflect.TypeOf(uint8(0)), MinTTCProtocolVersion, converters.EncodeUInt); err != nil {
		common.Odl.Warn("Failed to register uint8 encoder", "error", err)
	}
	if err := EncoderRegistry.Register(reflect.TypeOf(uint16(0)), MinTTCProtocolVersion, converters.EncodeUInt); err != nil {
		common.Odl.Warn("Failed to register uint16 encoder", "error", err)
	}
	if err := EncoderRegistry.Register(reflect.TypeOf(uint32(0)), MinTTCProtocolVersion, converters.EncodeUInt); err != nil {
		common.Odl.Warn("Failed to register uint32 encoder", "error", err)
	}
	if err := EncoderRegistry.Register(reflect.TypeOf(uint(0)), MinTTCProtocolVersion, converters.EncodeUInt); err != nil {
		common.Odl.Warn("Failed to register uint encoder", "error", err)
	}
	if err := EncoderRegistry.Register(reflect.TypeOf(uint64(0)), MinTTCProtocolVersion, converters.EncodeUInt); err != nil {
		common.Odl.Warn("Failed to register uint64 encoder", "error", err)
	}

	if err := EncoderRegistry.Register(reflect.TypeOf(float32(0)), MinTTCProtocolVersion, converters.EncodeFloat); err != nil {
		common.Odl.Warn("Failed to register float32 encoder", "error", err)
	}
	if err := EncoderRegistry.Register(reflect.TypeOf(float64(0)), MinTTCProtocolVersion, converters.EncodeFloat); err != nil {
		common.Odl.Warn("Failed to register float64 encoder", "error", err)
	}
	if err := CollectionEncoderRegistry.Register(common.DtyIbFloat, MinTTCProtocolVersion, func(v driver.Value) (common.B1Array, error) {
		encoded, err := converters.EncodeBinaryFloat(v)
		return common.B1Array(encoded), err
	}); err != nil {
		common.Odl.Warn("Failed to register BINARY_FLOAT collection encoder", "error", err)
	}
	if err := CollectionEncoderRegistry.Register(common.DtyIbDouble, MinTTCProtocolVersion, func(v driver.Value) (common.B1Array, error) {
		encoded, err := converters.EncodeBinaryDouble(v)
		return common.B1Array(encoded), err
	}); err != nil {
		common.Odl.Warn("Failed to register BINARY_DOUBLE collection encoder", "error", err)
	}
	if err := EncoderRegistry.Register(reflect.TypeOf(time.Time{}), MinTTCProtocolVersion, converters.EncodeTimestampWithTimeZone); err != nil {
		common.Odl.Warn("Failed to register time.Time encoder", "error", err)
	}

	if err := EncoderRegistry.Register(reflect.TypeOf([]byte(nil)), MinTTCProtocolVersion, converters.EncodeBinary); err != nil {
		common.Odl.Warn("Failed to register []byte encoder", "error", err)
	}

	if err := EncoderRegistry.Register(reflect.TypeOf(nil), MinTTCProtocolVersion, converters.EncodeNull); err != nil {
		common.Odl.Warn("Failed to register nil encoder", "error", err)
	}
	if err := EncoderRegistry.Register(reflect.TypeOf(RefCursor{}), MinTTCProtocolVersion, converters.EncodeNull); err != nil {
		common.Odl.Warn("Failed to register REF CURSOR encoder", "error", err)
	}
	if err := EncoderRegistry.Register(reflect.TypeOf((*driver.Rows)(nil)).Elem(), MinTTCProtocolVersion, converters.EncodeNull); err != nil {
		common.Odl.Warn("Failed to register REF CURSOR rows encoder", "error", err)
	}

	// bool is version dependent
	err = EncoderRegistry.Register(reflect.TypeOf(true), MinTTCProtocolVersion, converters.EncodeBooleanAsNumber)
	if err != nil {
		common.Odl.Warn("Failed to register string bool (v<=17) encoder", "error", err)
	}
	err = EncoderRegistry.Register(reflect.TypeOf(true), 18, converters.EncodeBoolean)
	if err != nil {
		common.Odl.Warn("Failed to register string bool (v>=18) encoder", "error", err)
	}

	// Register default decoders.
	if err := DecoderRegistry.Register(common.DtyNum, MinTTCProtocolVersion,
		newTypeDecoder(
			DecodeNumberColumn,
			GetScanTypeForNumberColumn)); err != nil {
		common.Odl.Warn("Failed to register number decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyVnu, MinTTCProtocolVersion,
		newTypeDecoder(
			DecodeNumberColumn,
			GetScanTypeForNumberColumn)); err != nil {
		common.Odl.Warn("Failed to register VNU decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyChr, MinTTCProtocolVersion, newTypeDecoder(
		DecodeVarcharColumn,
		GetScanTypeForVarcharColumn)); err != nil {
		common.Odl.Warn("Failed to register varchar decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyVCS, MinTTCProtocolVersion, newTypeDecoder(
		DecodeVarcharColumn,
		GetScanTypeForVarcharColumn)); err != nil {
		common.Odl.Warn("Failed to register varchar decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyAfc, MinTTCProtocolVersion, newTypeDecoder(
		DecodeCharColumn,
		GetScanTypeForCharColumn)); err != nil {
		common.Odl.Warn("Failed to register char decoder", "error", err)
	}
	// Note: bool decoder is NOT version-dependent; only encoding/OAC are.
	if err := DecoderRegistry.Register(common.DtyBol, MinTTCProtocolVersion, newTypeDecoder(
		DecodeBooleanColumn,
		GetScanTypeForBooleanColumn)); err != nil {
		common.Odl.Warn("Failed to register boolean decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyIbFloat, MinTTCProtocolVersion, newTypeDecoder(
		DecodeBinaryFloatColumn,
		GetScanTypeForBinaryFloatColumn)); err != nil {
		common.Odl.Warn("Failed to register binary float decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyIbDouble, MinTTCProtocolVersion,
		newTypeDecoder(
			DecodeBinaryDoubleColumn,
			GetScanTypeForDoubleColumn)); err != nil {
		common.Odl.Warn("Failed to register binary double decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyIym, MinTTCProtocolVersion, newTypeDecoder(
		DecodeIntervalYearToMonthColumn,
		GetScanTypeForIntervalYearToMonthColumn)); err != nil {
		common.Odl.Warn("Failed to register interval year-to-month decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyEiym, MinTTCProtocolVersion, newTypeDecoder(DecodeIntervalYearToMonthColumn,
		GetScanTypeForIntervalYearToMonthColumn)); err != nil {
		common.Odl.Warn("Failed to register interval year-to-month (extended) decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyIds, MinTTCProtocolVersion, newTypeDecoder(DecodeIntervalDayToSecondColumn,
		GetScanTypeForIntervalDayToSecondColumn)); err != nil {
		common.Odl.Warn("Failed to register interval day-to-second decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyEids, MinTTCProtocolVersion, newTypeDecoder(DecodeIntervalDayToSecondColumn,
		GetScanTypeForIntervalDayToSecondColumn)); err != nil {
		common.Odl.Warn("Failed to register interval day-to-second (extended) decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyDat, MinTTCProtocolVersion, newTypeDecoder(DecodeDateColumn,
		GetScanTypeForDateColumn)); err != nil {
		common.Odl.Warn("Failed to register date decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyEdate, MinTTCProtocolVersion, newTypeDecoder(DecodeDateColumn,
		GetScanTypeForDateColumn)); err != nil {
		common.Odl.Warn("Failed to register date (extended) decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyStamp, MinTTCProtocolVersion, newTypeDecoder(DecodeTimestampColumn,
		GetScanTypeForTimestampColumn)); err != nil {
		common.Odl.Warn("Failed to register timestamp decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyEstamp, MinTTCProtocolVersion, newTypeDecoder(DecodeTimestampColumn,
		GetScanTypeForTimestampColumn)); err != nil {
		common.Odl.Warn("Failed to register timestamp (extended) decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyStz, MinTTCProtocolVersion, newTypeDecoder(DecodeTimestampWithTimeZoneColumn,
		GetScanTypeForTimestampWithTimeZoneColumn)); err != nil {
		common.Odl.Warn("Failed to register timestamp with time zone decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyEstz, MinTTCProtocolVersion, newTypeDecoder(DecodeTimestampWithTimeZoneColumn,
		GetScanTypeForTimestampWithTimeZoneColumn)); err != nil {
		common.Odl.Warn("Failed to register timestamp with time zone (extended) decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtySitz, MinTTCProtocolVersion, newTypeDecoder(DecodeTimestampWithLocalTimeZoneColumn,
		GetScanTypeForTimestampWithLocalTimeZonColumn)); err != nil {
		common.Odl.Warn("Failed to register timestamp with local time zone decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyEsitz, MinTTCProtocolVersion, newTypeDecoder(DecodeTimestampWithLocalTimeZoneColumn,
		GetScanTypeForTimestampWithLocalTimeZonColumn)); err != nil {
		common.Odl.Warn("Failed to register timestamp with local time zone (extended) decoder", "error", err)
	}

	if err := DecoderRegistry.Register(common.DtyBin, MinTTCProtocolVersion, newTypeDecoder(DecodeBinaryColumn,
		GetScanTypeForBinaryColumn)); err != nil {
		common.Odl.Warn("Failed to register binary decoder", "error", err)
	}

	if err := DecoderRegistry.Register(common.DtyClob, MinTTCProtocolVersion,
		newTypeDecoder(DecodeClob, GetScanTypeForCLOBColumn)); err != nil {
		common.Odl.Warn("Failed to register CLOB decoder", "error", err)
	}

	if err := DecoderRegistry.Register(common.DtyJSON, MinTTCProtocolVersion,
		newTypeDecoder(DecodeJson, GetScanTypeForJsonColumn)); err != nil {
		common.Odl.Warn("Failed to register JSON decoder", "error", err)
	}

	if err := DecoderRegistry.Register(common.DtyBlob, MinTTCProtocolVersion, newTypeDecoder(DecodeBlob, GetScanTypeForBLOBColumn)); err != nil {
		common.Odl.Warn("Failed to register BLOB decoder", "error", err)
	}
	if err := DecoderRegistry.Register(common.DtyCur, MinTTCProtocolVersion, newTypeDecoder(
		func(_ ColumnContext, _ common.B1Array) (driver.Value, error) { return RefCursor{}, nil },
		func(_ ColumnContext) reflect.Type { return reflect.TypeOf(RefCursor{}) },
	)); err != nil {
		common.Odl.Warn("Failed to register REF CURSOR decoder", "error", err)
	}

	// Register default bind OACs.
	if err := BindOacRegistry.Register(reflect.TypeOf(""), MinTTCProtocolVersion, bindOacType{bindOacFunc: newTTIOacString, maxLength: converters.MaxVarcharLength}); err != nil {
		common.Odl.Warn("Failed to register string bind OAC", "error", err)
	}

	if err := BindOacRegistry.Register(reflect.TypeOf(int8(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(common.UB4) common.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register int8 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(int16(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(common.UB4) common.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register int16 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(int32(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(common.UB4) common.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register int32 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(int(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(common.UB4) common.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register int bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(int64(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(common.UB4) common.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register int64 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(uint8(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(common.UB4) common.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register uint8 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(uint16(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(common.UB4) common.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register uint16 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(uint32(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(common.UB4) common.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register uint32 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(uint(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(common.UB4) common.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register uint bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(uint64(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(common.UB4) common.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register uint64 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(float32(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(common.UB4) common.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength, scale: common.SB1(NumberScaleFloatSentinel)}); err != nil {
		common.Odl.Warn("Failed to register float32 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(float64(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(common.UB4) common.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength, scale: common.SB1(NumberScaleFloatSentinel)}); err != nil {
		common.Odl.Warn("Failed to register float64 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf([]byte(nil)), MinTTCProtocolVersion, bindOacType{bindOacFunc: newTTIOacBytes, maxLength: 32767}); err != nil {
		common.Odl.Warn("Failed to register []byte bind OAC", "error", err)
	}

	if err := BindOacRegistry.Register(reflect.TypeOf(nil), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(common.UB4) common.Marshallable { return newTTIOacNull() }, maxLength: converters.MaxNullLength}); err != nil {
		common.Odl.Warn("Failed to register nil bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(RefCursor{}), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(common.UB4) common.Marshallable { return newTTIoac(common.DtyRSet, 4) }, maxLength: 4}); err != nil {
		common.Odl.Warn("Failed to register REF CURSOR bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf((*driver.Rows)(nil)).Elem(), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(common.UB4) common.Marshallable { return newTTIoac(common.DtyRSet, 4) }, maxLength: 4}); err != nil {
		common.Odl.Warn("Failed to register REF CURSOR rows bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(time.Time{}), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(common.UB4) common.Marshallable { return newTTIOacTime() }, maxLength: converters.MaxTimeStampLength}); err != nil {
		common.Odl.Warn("Failed to register time.Time bind OAC", "error", err)
	}

	// Bool is version-dependent
	if err := BindOacRegistry.Register(reflect.TypeOf(true), MinTTCProtocolVersion, bindOacType{bindOacFunc: newTTIOacBoolV17, maxLength: converters.MaxBoolLength}); err != nil {
		common.Odl.Warn("Failed to register bool (v<=17) bind OAC", "error", err)
	}

	if err := BindOacRegistry.Register(reflect.TypeOf(true), 18, bindOacType{bindOacFunc: func(common.UB4) common.Marshallable { return newTTIOacBool() }, maxLength: converters.MaxBoolLength}); err != nil {
		common.Odl.Warn("Failed to register bool (v>=18) bind OAC", "error", err)
	}

	// Register default define OAC makers.
	// JSON currently uses a BLOB-based define OAC with max length 4000.
	if err := DefineOacRegistry.Register(common.DtyJSON, MinTTCProtocolVersion, newTTIOacJSONDefine); err != nil {
		common.Odl.Warn("Failed to register JSON define OAC", "error", err)
	}

	if err := DefineOacRegistry.Register(common.DtyClob, MinTTCProtocolVersion, newTTIOacClobDefine); err != nil {
		common.Odl.Warn("Failed to register CLOB define OAC", "error", err)
	}

	if err := DefineOacRegistry.Register(common.DtyBlob, MinTTCProtocolVersion, newTTIOacBlobDefine); err != nil {
		common.Odl.Warn("Failed to register BLOB define OAC", "error", err)
	}

	if err := DefineOacRegistry.Register(common.DtyVCS, MinTTCProtocolVersion, func(columnContext ColumnContext, _ common.UB4) common.Marshallable {
		return newTTIOacVarcharDefine(columnContext)
	}); err != nil {
		common.Odl.Warn("Failed to register VARCHAR define OAC", "error", err)
	}

	_sqlKindMap["select"] = select_
	_sqlKindMap["insert"] = dml
	_sqlKindMap["update"] = dml
	_sqlKindMap["delete"] = dml
	_sqlKindMap["merge"] = dml
	_sqlKindMap["call"] = plsql
	_sqlKindMap["begin"] = plsql
	_sqlKindMap["declare"] = plsql
	_sqlKindMap["create"] = other
	_sqlKindMap["alter"] = other
	_sqlKindMap["drop"] = other
	_sqlKindMap["truncate"] = other
	_sqlKindMap["rename"] = other
	_sqlKindMap["comment"] = other
	_sqlKindMap["grant"] = other
	_sqlKindMap["revoke"] = other
	_sqlKindMap["analyze"] = other
	_sqlKindMap["purge"] = other
	_sqlKindMap["lock"] = other
	_sqlKindMap["commit"] = other
	_sqlKindMap["rollback"] = other
	_sqlKindMap["savepoint"] = other
	_sqlKindMap["set"] = other
	_sqlKindMap["explain"] = other
	_sqlKindMap["flashback"] = other

}
