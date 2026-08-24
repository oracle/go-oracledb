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
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strconv"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/converters"
)

const MinTTCProtocolVersion = 12 // 19.1

var currentDriverName driverCommon.B1Array
var currentDriverExternalName = driverCommon.StringToB1Array(driverNameDefault + "_" + driverDefaultResourceManagerID)
var currentDriverACLValue = driverCommon.StringToB1Array(defaultACLValue)
var currentDriverInternalName = driverCommon.StringToB1Array("go_ttc_impl")

var _authTerminalKey = driverCommon.StringToB1Array(authTerminal)
var _authConnectStringKey = driverCommon.StringToB1Array(authConnectString)
var _authProgramNmKey = driverCommon.StringToB1Array(authProgramNm)

var _authMachineKey = driverCommon.StringToB1Array(authMachine)
var _authPidKey = driverCommon.StringToB1Array(authPid)
var _authACLKey = driverCommon.StringToB1Array(authACL)
var _authClientCapabilitiesKey = driverCommon.StringToB1Array(authClientCapabilities)
var _authClientCapabilitiesVal = driverCommon.StringToB1Array(strconv.Itoa(0x00000001 + 0x00000002))

// static information used by oauth message
var _keyValStaticInfoForOsesskey = list.New()
var _keyValStaticInfoForOAuth1 = list.New()
var _keyValStaticInfoForOAuth2 = list.New()
var _keyValStaticInfoForOAuthConnectString *list.Element

var _dummyTerminalName = driverCommon.StringToB1Array("unknown")
var currentUserName driverCommon.B1Array

// _initEnvironmentStaticInformation initializes static values from the current environment.
func _initEnvironmentStaticInformation() {

	currentTerminal := driverCommon.StringToB1Array("unknown")
	currentDriverName = driverCommon.StringToB1Array(driverNameDefault + " : " + common.DriverVersion)

	if u, err := user.Current(); err == nil {
		currentUserName = driverCommon.StringToB1Array(u.Username)
	} else {
		common.Odl.Info(fmt.Sprintf("using default as user name"))
		currentUserName = driverCommon.StringToB1Array("unknown")
	}

	var currentProcessPath driverCommon.B1Array

	if e, err := os.Executable(); err == nil {
		currentProcessPath = driverCommon.StringToB1Array(filepath.Base(e))
	} else {
		common.Odl.Info(fmt.Sprintf("using default as process path"))
		currentProcessPath = driverCommon.StringToB1Array("unknown")
	}
	var currentProcessId driverCommon.B1Array
	currentProcessId = driverCommon.StringToB1Array(strconv.Itoa(os.Getpid()))
	var currentMachineName driverCommon.B1Array
	if h, err := os.Hostname(); err == nil {
		currentMachineName = driverCommon.StringToB1Array(h)
	} else {
		common.Odl.Info(fmt.Sprintf("using default as machine name"))
		currentMachineName = driverCommon.StringToB1Array("oraclegoclient")
	}
	currentDriverInternalName = driverCommon.StringToB1Array(driverInternalName)

	_keyValStaticInfoForOsesskey.PushBack(&driverCommon.KeyValue{Key: driverCommon.StringToB1Array(authTerminal), Value: currentTerminal})
	_keyValStaticInfoForOsesskey.PushBack(&driverCommon.KeyValue{Key: driverCommon.StringToB1Array(authProgramNm), Value: currentProcessPath})
	_keyValStaticInfoForOsesskey.PushBack(&driverCommon.KeyValue{Key: driverCommon.StringToB1Array(authMachine), Value: currentMachineName})
	_keyValStaticInfoForOsesskey.PushBack(&driverCommon.KeyValue{Key: driverCommon.StringToB1Array(authPid), Value: currentProcessId})
	_keyValStaticInfoForOsesskey.PushBack(&driverCommon.KeyValue{Key: driverCommon.StringToB1Array(authSid), Value: currentUserName})

	_keyValStaticInfoForOAuth1.PushBack(&driverCommon.KeyValue{Key: _authTerminalKey, Value: _dummyTerminalName})
	_keyValStaticInfoForOAuth1.PushBack(&driverCommon.KeyValue{Key: _authConnectStringKey, Value: nil})
	_keyValStaticInfoForOAuthConnectString = _keyValStaticInfoForOAuth1.Back()
	_keyValStaticInfoForOAuth1.PushBack(&driverCommon.KeyValue{Key: _authProgramNmKey, Value: currentProcessPath})

	_keyValStaticInfoForOAuth2.PushBack(&driverCommon.KeyValue{Key: _authMachineKey, Value: currentMachineName})
	_keyValStaticInfoForOAuth2.PushBack(&driverCommon.KeyValue{Key: _authPidKey, Value: currentProcessId})
	_keyValStaticInfoForOAuth2.PushBack(&driverCommon.KeyValue{Key: _authACLKey, Value: currentDriverACLValue})
	_keyValStaticInfoForOAuth2.PushBack(&driverCommon.KeyValue{Key: _authClientCapabilitiesKey, Value: _authClientCapabilitiesVal})

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

	err = FunctionRegistry.Register(functionRegistryKey{messageType: TTISPF, functionType: driverCommon.FunctionType(ocssync)}, -1, NewttiSPFOCSSync)
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
	typeRepresentationTable.addTypeRepToTable(DtyChr, DtyChr, int16(RepCUnv))
	typeRepresentationTable.addTypeRepToTable(DtyNum, DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(DtyBol, DtyBol, int16(RepIUnv))
	typeRepresentationTable.addTypeRepToTable(DtyLng, DtyLng, int16(RepCUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDat, DtyDat, int16(RepDV51))
	typeRepresentationTable.addTypeRepToTable(DtyBin, DtyBin, int16(RepBUnv))
	typeRepresentationTable.addTypeRepToTable(DtyLbi, DtyLbi, int16(RepBUnv))
	// data types for moving structures etc across the interface
	// integer data types
	typeRepresentationTable.addTypeRepToTable(DtyUb2, DtyUb2, int16(RepIUnv))
	typeRepresentationTable.addTypeRepToTable(DtyUb4, DtyUb4, int16(RepIUnv))
	typeRepresentationTable.addTypeRepToTable(DtyB1, DtyB1, int16(RepIUnv))
	typeRepresentationTable.addTypeRepToTable(DtyB2, DtyB2, int16(RepIUnv))
	typeRepresentationTable.addTypeRepToTable(DtyB4, DtyB4, int16(RepIUnv))
	typeRepresentationTable.addTypeRepToTable(DtyWord, DtyWord, int16(RepIUnv))
	typeRepresentationTable.addTypeRepToTable(DtyUword, DtyUword, int16(RepIUnv))
	// pointer data types
	typeRepresentationTable.addTypeRepToTable(DtyPb, DtyPb, int16(RepAUnv))
	typeRepresentationTable.addTypeRepToTable(DtyPw, DtyPw, int16(RepAUnv))

	// next send the records
	typeRepresentationTable.addTypeRepToTable(DtyTi5, DtyTi5, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRiD, DtyRiD, int16(RepRUnv))
	// opidef program interface request block types
	typeRepresentationTable.addTypeRepToTable(DtyAms, DtyAms, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyBrn, DtyBrn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyCwd, DtyCwd, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyNac122, DtyNac122, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyOer8, DtyOer8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyFun, DtyFun, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAua, DtyAua, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRxh7, DtyRxh7, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyNa6, DtyNa6, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyBrp, DtyBrp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyBrv, DtyBrv, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKva, DtyKva, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyCls, DtyCls, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyCui, DtyCui, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDfn, DtyDfn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDqr, DtyDqr, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsc, DtyDsc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyExe, DtyExe, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyFch, DtyFch, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyGbv, DtyGbv, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyGem, DtyGem, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyGiv, DtyGiv, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyOkg, DtyOkg, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyHmi, DtyHmi, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyIno, DtyIno, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyLnf, DtyLnf, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyOnt, DtyOnt, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyOpe, DtyOpe, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyOsq, DtyOsq, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtySfe, DtySfe, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtySpf, DtySpf, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyVsn, DtyVsn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyUd7, DtyUd7, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsa, DtyDsa, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyPin, DtyPin, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyPfn, DtyPfn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyPpt, DtyPpt, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtySto, DtySto, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyArc, DtyArc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyMrs, DtyMrs, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyMrt, DtyMrt, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyMrg, DtyMrg, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyMrr, DtyMrr, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyMrc, DtyMrc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyVer, DtyVer, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyLon2, DtyLon2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyIno2, DtyIno2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAll, DtyAll, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyUdb, DtyUdb, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAqi, DtyAqi, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyUlb, DtyUlb, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyUld, DtyUld, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtySid, DtySid, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyNa7, DtyNa7, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAl7, DtyAl7, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyK2Rpc, DtyK2Rpc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXdp, DtyXdp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyOko8, DtyOko8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyUd12, DtyUd12, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAl8, DtyAl8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyLfop, DtyLfop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyFcrt, DtyFcrt, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDny, DtyDny, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyOpr, DtyOpr, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyPls, DtyPls, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXid, DtyXid, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyTxn, DtyTxn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDcb, DtyDcb, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyCca, DtyCca, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyWrn, DtyWrn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyTlh121, DtyTlh121, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyToh121, DtyToh121, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyFoi, DtyFoi, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtySid2, DtySid2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyTch, DtyTch, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyPii, DtyPii, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyPfi, DtyPfi, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyPpu, DtyPpu, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyPte, DtyPte, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRxh8, DtyRxh8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyTn12, DtyTn12, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAuth, DtyAuth, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKval, DtyKval, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyFgi, DtyFgi, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsy, DtyDsy, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyR8, DtyDsyR8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyH8, DtyDsyH8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyL, DtyDsyL, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyT8, DtyDsyT8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyV8, DtyDsyV8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyP, DtyDsyP, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyF, DtyDsyF, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyK, DtyDsyK, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyY, DtyDsyY, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyQ, DtyDsyQ, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyC, DtyDsyC, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyA, DtyDsyA, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyOt8, DtyOt8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyTy, DtyDsyTy, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAqe, DtyAqe, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKv, DtyKv, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAqd, DtyAqd, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAQ8, DtyAQ8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRfs, DtyRfs, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRxh10, DtyRxh10, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpn, DtyKpn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpdnr, DtyKpdnr, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyD, DtyDsyD, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyS, DtyDsyS, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyR, DtyDsyR, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyH, DtyDsyH, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyT, DtyDsyT, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDsyV, DtyDsyV, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAqm, DtyAqm, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyOer11, DtyOer11, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAql, DtyAql, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyOtc, DtyOtc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKfno, DtyKfno, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKfnp, DtyKfnp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyOkgt8, DtyOkgt8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRaSb4, DtyRaSb4, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRaUb2, DtyRaUb2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRaUb1, DtyRaUb1, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRaTxt, DtyRaTxt, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRsSb4, DtyRsSb4, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRsUb2, DtyRsUb2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRsUb1, DtyRsUb1, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRsTxt, DtyRsTxt, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRidl, DtyRidl, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyGlrdd, DtyGlrdd, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyGlrdg, DtyGlrdg, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyGlrdc, DtyGlrdc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyOko, DtyOko, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDpp, DtyDpp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDpls, DtyDpls, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDpmop, DtyDpmop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyStat, DtyStat, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRfx, DtyRfx, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyFal, DtyFal, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyCkv, DtyCkv, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDrcx, DtyDrcx, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKgh, DtyKgh, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAqo, DtyAqo, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyOkgt, DtyOkgt, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpfc, DtyKpfc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyFe2, DtyFe2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtySpfp, DtySpfp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDpuls, DtyDpuls, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAqa, DtyAqa, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpbf, DtyKpbf, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyTsm, DtyTsm, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyMss, DtyMss, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpc, DtyKpc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyCrs, DtyCrs, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKks, DtyKks, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKsp, DtyKsp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKspTop, DtyKspTop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKspVal, DtyKspVal, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyPss, DtyPss, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyNls, DtyNls, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAls, DtyAls, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKsdEvtVal, DtyKsdEvtVal, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKsdEvtTop, DtyKsdEvtTop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpspp, DtyKpspp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKol, DtyKol, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyLst, DtyLst, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAcx, DtyAcx, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyScs, DtyScs, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRxh, DtyRxh, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpdns, DtyKpdns, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpdcn, DtyKpdcn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpnns, DtyKpnns, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpncn, DtyKpncn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKps, DtyKps, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyApinf, DtyApinf, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyTen, DtyTen, int16(RepRUnv))

	typeRepresentationTable.addTypeRepToTable(DtyXsscs, DtyXsscs, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXssro, DtyXssro, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXsspo, DtyXsspo, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKsrpc, DtyKsrpc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKvl, DtyKvl, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXsssDef, DtyXsssDef, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpdqcInv, DtyKpdqcInv, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpdqIdc, DtyKpdqIdc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpdqcSta, DtyKpdqcSta, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKprs, DtyKprs, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpdqcID, DtyKpdqcID, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRtstrm, DtyRtstrm, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtySessGet, DtySessGet, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtySessRls, DtySessRls, int16(RepRUnv))
	// server to client piggyback:
	typeRepresentationTable.addTypeRepToTable(DtySessRet, DtySessRet, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyScn6, DtyScn6, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKecpa, DtyKecpa, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKecpp, DtyKecpp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtySxa, DtySxa, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKvarr, DtyKvarr, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpngn, DtyKpngn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyInt, DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(DtyFlt, DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(DtyStr, DtyChr, int16(RepCUnv))
	typeRepresentationTable.addTypeRepToTable(DtyVnu, DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(DtyPdn, DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(DtyVCS, DtyChr, int16(RepCUnv))
	// some internal data types. not internal, and not kernel
	typeRepresentationTable.addTypeRepToTable(DtyIdt, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyIju, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyVbi, DtyBin, int16(RepBUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDif, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyDof, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyDtz, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyDyn, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyDpc, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyBfloat, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyBdouble, Dty0, int16(0))
	// structure data types - used for TTI messages
	// oracle version of uac - one form for native, one for network
	typeRepresentationTable.addTypeRepToTable(DtyOac, DtyNac, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyOpq, Dty0, int16(0)) // Opaque

	typeRepresentationTable.addTypeRepToTable(DtyUin, DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(DtyBri, Dty0, int16(0))
	// Array - recycling Dty70 - %TEMPORARY%
	typeRepresentationTable.addTypeRepToTable(DtyArr, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyOcu, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyVar, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtySls, DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(DtyLvc, DtyChr, int16(RepCUnv))
	typeRepresentationTable.addTypeRepToTable(DtyLvb, DtyBin, int16(RepBUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAfc, DtyAfc, int16(RepCUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAvc, DtyAfc, int16(RepCUnv))
	// The datatype for canonical format binary_float is lfp_cf
	// The datatype for canonical format binary_double is lfp_cd
	typeRepresentationTable.addTypeRepToTable(DtyIbFloat, DtyIbFloat, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyIbDouble, DtyIbDouble, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyCur, DtyCur, int16(RepUnv))
	// direct path Export
	typeRepresentationTable.addTypeRepToTable(DtyRdd, DtyRiD, int16(RepUnv))
	// datatypes for labels
	typeRepresentationTable.addTypeRepToTable(DtyLab, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyOsl, DtyOsl, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyNty, DtyINty, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyINty, DtyINty, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRef, DtyIref, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyIref, DtyIref, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyClob, DtyClob, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyBlob, DtyBlob, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyBFil, DtyBFil, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyCFil, DtyCFil, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyRSet, DtyCur, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtySvt, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyJSON, DtyJSON, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAdt, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyNtb, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyNar, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyVec, DtyVec, int16(RepUnv)) // 23.4 Vector
	typeRepresentationTable.addTypeRepToTable(DtyObj, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyClv, DtyClv, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyBlv, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyDtr, DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(DtyDun, DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(DtyDop, DtyNum, int16(RepNV51))
	typeRepresentationTable.addTypeRepToTable(DtyVst, DtyChr, int16(RepCUnv))
	typeRepresentationTable.addTypeRepToTable(DtyOdt, DtyDat, int16(RepDV51))
	typeRepresentationTable.addTypeRepToTable(DtyDol, DtyNum, int16(RepNV51))
	// old byte array stops here
	typeRepresentationTable.addTypeRepToTable(DtyTime, DtyTime, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyTtz, DtyTtz, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyStamp, DtyStamp, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyStz, DtyStz, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyIym, DtyIym, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyIds, DtyIds, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyEdate, DtyDat, int16(RepDV51))
	typeRepresentationTable.addTypeRepToTable(DtyEtime, DtyEtime, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyEttz, DtyEttz, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyEstamp, DtyEstamp, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyEstz, DtyEstz, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyEiym, DtyEiym, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyEids, DtyEids, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyLdiIf, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyLdiOf, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtyDclob, DtyClob, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDblob, DtyBlob, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDjson, DtyJSON, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyDbfil, DtyBFil, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyBuri, DtyBuri, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyPsr, Dty0, int16(0))
	typeRepresentationTable.addTypeRepToTable(DtySitz, DtySitz, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyEsitz, DtySitz, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyUb8, DtyUb8, int16(RepIUnv))
	typeRepresentationTable.addTypeRepToTable(DtyPnty, DtyINty, int16(RepUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAbs, Dty0, int16(0))

	// !!!! NEW RECORDS SHOULD BE INSERTED HERE !!!!
	typeRepresentationTable.addTypeRepToTable(DtyXsnsop, DtyXsnsop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXsattr, DtyXsattr, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXsns, DtyXsns, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyUb1Array, DtyUb1Array, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtySessState, DtySessState, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAppContReplay, DtyAppContReplay, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAppContCtl, DtyAppContCtl, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtySessSign, DtySessSign, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyImplRes, DtyImplRes, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyOer, DtyOer, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpdxft, DtyKpdxft, int16(RepRUnv))

	// 20c
	typeRepresentationTable.addTypeRepToTable(DtyShrdKeySync, DtyShrdKeySync, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyOer19, DtyOer19, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpdssTemplate, DtyKpdssTemplate, int16(RepRUnv))

	// 23ai
	typeRepresentationTable.addTypeRepToTable(DtySaga, DtySaga, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyUd21, DtyUd21, int16(RepRUnv))

	// New records for triton 12c:
	typeRepresentationTable.addTypeRepToTable(DtyTxt, DtyTxt, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXssessns, DtyXssessns, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXsattop, DtyXsattop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXscreop, DtyXscreop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXsdetop, DtyXsdetop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXsdesop, DtyXsdesop, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXssetsp, DtyXssetsp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXssidp, DtyXssidp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXsprin, DtyXsprin, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXskvl, DtyXskvl, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXsssdef2, DtyXsssdef2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXsnsop2, DtyXsnsop2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyXsns2, DtyXsns2, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtykpdnrEq, DtykpdnrEq, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtykpdnrNf, DtykpdnrNf, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpngnc, DtyKpngnc, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyKpnri, DtyKpnri, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAqEnq, DtyAqEnq, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAqDeq, DtyAqDeq, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyAqJms, DtyAqJms, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtykpdnrPay, DtykpdnrPay, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtykpdnrAck, DtykpdnrAck, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtykpdnrMp, DtykpdnrMp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtykpdnrDq, DtykpdnrDq, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyScn, DtyScn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyScn8, DtyScn8, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyChunkInfo, DtyChunkInfo, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyUds, DtyUds, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyTnp, DtyTnp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyTlh, DtyTlh, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyToh, DtyToh, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtySnp, DtySnp, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyNac, DtyNac, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyPlend, DtyPlend, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyPlbgn, DtyPlbgn, int16(RepRUnv))
	typeRepresentationTable.addTypeRepToTable(DtyPlopn, DtyPlopn, int16(RepRUnv))

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
	if err := EncoderRegistry.Register(reflect.TypeOf(time.Time{}), MinTTCProtocolVersion, converters.EncodeTimestampWithTimeZone); err != nil {
		common.Odl.Warn("Failed to register time.Time encoder", "error", err)
	}

	if err := EncoderRegistry.Register(reflect.TypeOf([]byte(nil)), MinTTCProtocolVersion, converters.EncodeBinary); err != nil {
		common.Odl.Warn("Failed to register []byte encoder", "error", err)
	}
	if err := EncoderRegistry.Register(reflect.TypeOf(blobLocator(nil)), MinTTCProtocolVersion, converters.EncodeBinary); err != nil {
		common.Odl.Warn("Failed to register BLOB locator encoder", "error", err)
	}

	if err := EncoderRegistry.Register(reflect.TypeOf(nil), MinTTCProtocolVersion, converters.EncodeNull); err != nil {
		common.Odl.Warn("Failed to register nil encoder", "error", err)
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
	if err := DecoderRegistry.Register(DtyNum, MinTTCProtocolVersion,
		newTypeDecoder(
			DecodeNumberColumn,
			GetScanTypeForNumberColumn)); err != nil {
		common.Odl.Warn("Failed to register number decoder", "error", err)
	}
	if err := DecoderRegistry.Register(DtyVnu, MinTTCProtocolVersion,
		newTypeDecoder(
			DecodeNumberColumn,
			GetScanTypeForNumberColumn)); err != nil {
		common.Odl.Warn("Failed to register VNU decoder", "error", err)
	}
	if err := DecoderRegistry.Register(DtyChr, MinTTCProtocolVersion, newTypeDecoder(
		DecodeVarcharColumn,
		GetScanTypeForVarcharColumn)); err != nil {
		common.Odl.Warn("Failed to register varchar decoder", "error", err)
	}
	if err := DecoderRegistry.Register(DtyAfc, MinTTCProtocolVersion, newTypeDecoder(
		DecodeCharColumn,
		GetScanTypeForCharColumn)); err != nil {
		common.Odl.Warn("Failed to register char decoder", "error", err)
	}
	// Note: bool decoder is NOT version-dependent; only encoding/OAC are.
	if err := DecoderRegistry.Register(DtyBol, MinTTCProtocolVersion, newTypeDecoder(
		DecodeBooleanColumn,
		GetScanTypeForBooleanColumn)); err != nil {
		common.Odl.Warn("Failed to register boolean decoder", "error", err)
	}
	if err := DecoderRegistry.Register(DtyIbFloat, MinTTCProtocolVersion, newTypeDecoder(
		DecodeBinaryFloatColumn,
		GetScanTypeForBinaryFloatColumn)); err != nil {
		common.Odl.Warn("Failed to register binary float decoder", "error", err)
	}
	if err := DecoderRegistry.Register(DtyIbDouble, MinTTCProtocolVersion,
		newTypeDecoder(
			DecodeBinaryDoubleColumn,
			GetScanTypeForDoubleColumn)); err != nil {
		common.Odl.Warn("Failed to register binary double decoder", "error", err)
	}
	if err := DecoderRegistry.Register(DtyIym, MinTTCProtocolVersion, newTypeDecoder(
		DecodeIntervalYearToMonthColumn,
		GetScanTypeForIntervalYearToMonthColumn)); err != nil {
		common.Odl.Warn("Failed to register interval year-to-month decoder", "error", err)
	}
	if err := DecoderRegistry.Register(DtyEiym, MinTTCProtocolVersion, newTypeDecoder(DecodeIntervalYearToMonthColumn,
		GetScanTypeForIntervalYearToMonthColumn)); err != nil {
		common.Odl.Warn("Failed to register interval year-to-month (extended) decoder", "error", err)
	}
	if err := DecoderRegistry.Register(DtyIds, MinTTCProtocolVersion, newTypeDecoder(DecodeIntervalDayToSecondColumn,
		GetScanTypeForIntervalDayToSecondColumn)); err != nil {
		common.Odl.Warn("Failed to register interval day-to-second decoder", "error", err)
	}
	if err := DecoderRegistry.Register(DtyEids, MinTTCProtocolVersion, newTypeDecoder(DecodeIntervalDayToSecondColumn,
		GetScanTypeForIntervalDayToSecondColumn)); err != nil {
		common.Odl.Warn("Failed to register interval day-to-second (extended) decoder", "error", err)
	}
	if err := DecoderRegistry.Register(DtyDat, MinTTCProtocolVersion, newTypeDecoder(DecodeDateColumn,
		GetScanTypeForDateColumn)); err != nil {
		common.Odl.Warn("Failed to register date decoder", "error", err)
	}
	if err := DecoderRegistry.Register(DtyEdate, MinTTCProtocolVersion, newTypeDecoder(DecodeDateColumn,
		GetScanTypeForDateColumn)); err != nil {
		common.Odl.Warn("Failed to register date (extended) decoder", "error", err)
	}
	if err := DecoderRegistry.Register(DtyStamp, MinTTCProtocolVersion, newTypeDecoder(DecodeTimestampColumn,
		GetScanTypeForTimestampColumn)); err != nil {
		common.Odl.Warn("Failed to register timestamp decoder", "error", err)
	}
	if err := DecoderRegistry.Register(DtyEstamp, MinTTCProtocolVersion, newTypeDecoder(DecodeTimestampColumn,
		GetScanTypeForTimestampColumn)); err != nil {
		common.Odl.Warn("Failed to register timestamp (extended) decoder", "error", err)
	}
	if err := DecoderRegistry.Register(DtyStz, MinTTCProtocolVersion, newTypeDecoder(DecodeTimestampWithTimeZoneColumn,
		GetScanTypeForTimestampWithTimeZoneColumn)); err != nil {
		common.Odl.Warn("Failed to register timestamp with time zone decoder", "error", err)
	}
	if err := DecoderRegistry.Register(DtyEstz, MinTTCProtocolVersion, newTypeDecoder(DecodeTimestampWithTimeZoneColumn,
		GetScanTypeForTimestampWithTimeZoneColumn)); err != nil {
		common.Odl.Warn("Failed to register timestamp with time zone (extended) decoder", "error", err)
	}
	if err := DecoderRegistry.Register(DtySitz, MinTTCProtocolVersion, newTypeDecoder(DecodeTimestampWithLocalTimeZoneColumn,
		GetScanTypeForTimestampWithLocalTimeZonColumn)); err != nil {
		common.Odl.Warn("Failed to register timestamp with local time zone decoder", "error", err)
	}
	if err := DecoderRegistry.Register(DtyEsitz, MinTTCProtocolVersion, newTypeDecoder(DecodeTimestampWithLocalTimeZoneColumn,
		GetScanTypeForTimestampWithLocalTimeZonColumn)); err != nil {
		common.Odl.Warn("Failed to register timestamp with local time zone (extended) decoder", "error", err)
	}

	if err := DecoderRegistry.Register(DtyBin, MinTTCProtocolVersion, newTypeDecoder(DecodeBinaryColumn,
		GetScanTypeForBinaryColumn)); err != nil {
		common.Odl.Warn("Failed to register binary decoder", "error", err)
	}

	if err := DecoderRegistry.Register(DtyClob, MinTTCProtocolVersion,
		newTypeDecoder(DecodeClob, GetScanTypeForCLOBColumn)); err != nil {
		common.Odl.Warn("Failed to register CLOB decoder", "error", err)
	}

	if err := DecoderRegistry.Register(DtyJSON, MinTTCProtocolVersion,
		newTypeDecoder(DecodeJson, GetScanTypeForJsonColumn)); err != nil {
		common.Odl.Warn("Failed to register JSON decoder", "error", err)
	}

	if err := DecoderRegistry.Register(DtyBlob, MinTTCProtocolVersion, newTypeDecoder(DecodeBlob, GetScanTypeForBLOBColumn)); err != nil {
		common.Odl.Warn("Failed to register BLOB decoder", "error", err)
	}

	// Register default bind OACs.
	if err := BindOacRegistry.Register(reflect.TypeOf(""), MinTTCProtocolVersion, bindOacType{bindOacFunc: newTTIOacString, maxLength: converters.MaxVarcharLength}); err != nil {
		common.Odl.Warn("Failed to register string bind OAC", "error", err)
	}

	if err := BindOacRegistry.Register(reflect.TypeOf(int8(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(driverCommon.UB4) driverCommon.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register int8 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(int16(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(driverCommon.UB4) driverCommon.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register int16 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(int32(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(driverCommon.UB4) driverCommon.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register int32 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(int(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(driverCommon.UB4) driverCommon.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register int bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(int64(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(driverCommon.UB4) driverCommon.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register int64 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(uint8(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(driverCommon.UB4) driverCommon.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register uint8 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(uint16(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(driverCommon.UB4) driverCommon.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register uint16 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(uint32(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(driverCommon.UB4) driverCommon.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register uint32 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(uint(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(driverCommon.UB4) driverCommon.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register uint bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(uint64(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(driverCommon.UB4) driverCommon.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength}); err != nil {
		common.Odl.Warn("Failed to register uint64 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(float32(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(driverCommon.UB4) driverCommon.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength, scale: driverCommon.SB1(NumberScaleFloatSentinel)}); err != nil {
		common.Odl.Warn("Failed to register float32 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(float64(0)), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(driverCommon.UB4) driverCommon.Marshallable { return newTTIOacNumber() }, maxLength: converters.MaxNumberLength, scale: driverCommon.SB1(NumberScaleFloatSentinel)}); err != nil {
		common.Odl.Warn("Failed to register float64 bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf([]byte(nil)), MinTTCProtocolVersion, bindOacType{bindOacFunc: newTTIOacBytes, maxLength: 32767}); err != nil {
		common.Odl.Warn("Failed to register []byte bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(blobLocator(nil)), MinTTCProtocolVersion, bindOacType{bindOacFunc: newTTIOacBlobBind, maxLength: max_lob_length}); err != nil {
		common.Odl.Warn("Failed to register BLOB locator bind OAC", "error", err)
	}

	if err := BindOacRegistry.Register(reflect.TypeOf(nil), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(driverCommon.UB4) driverCommon.Marshallable { return newTTIOacNull() }, maxLength: converters.MaxNullLength}); err != nil {
		common.Odl.Warn("Failed to register nil bind OAC", "error", err)
	}
	if err := BindOacRegistry.Register(reflect.TypeOf(time.Time{}), MinTTCProtocolVersion, bindOacType{bindOacFunc: func(driverCommon.UB4) driverCommon.Marshallable { return newTTIOacTime() }, maxLength: converters.MaxTimeStampLength}); err != nil {
		common.Odl.Warn("Failed to register time.Time bind OAC", "error", err)
	}

	// Bool is version-dependent
	if err := BindOacRegistry.Register(reflect.TypeOf(true), MinTTCProtocolVersion, bindOacType{bindOacFunc: newTTIOacBoolV17, maxLength: converters.MaxBoolLength}); err != nil {
		common.Odl.Warn("Failed to register bool (v<=17) bind OAC", "error", err)
	}

	if err := BindOacRegistry.Register(reflect.TypeOf(true), 18, bindOacType{bindOacFunc: func(driverCommon.UB4) driverCommon.Marshallable { return newTTIOacBool() }, maxLength: converters.MaxBoolLength}); err != nil {
		common.Odl.Warn("Failed to register bool (v>=18) bind OAC", "error", err)
	}

	// Register default define OAC makers.
	// JSON currently uses a BLOB-based define OAC with max length 4000.
	if err := DefineOacRegistry.Register(DtyJSON, MinTTCProtocolVersion, newTTIOacJSONDefine); err != nil {
		common.Odl.Warn("Failed to register JSON define OAC", "error", err)
	}

	if err := DefineOacRegistry.Register(DtyClob, MinTTCProtocolVersion, newTTIOacClobDefine); err != nil {
		common.Odl.Warn("Failed to register CLOB define OAC", "error", err)
	}

	if err := DefineOacRegistry.Register(DtyBlob, MinTTCProtocolVersion, newTTIOacBlobDefine); err != nil {
		common.Odl.Warn("Failed to register BLOB define OAC", "error", err)
	}

	if err := DefineOacRegistry.Register(DtyVCS, MinTTCProtocolVersion, func(columnContext ColumnContext, _ driverCommon.UB4) driverCommon.Marshallable {
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
