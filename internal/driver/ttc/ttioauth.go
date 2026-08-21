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
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	oracleutils "github.com/oracle/go-oracledb/v26/oracle/utils"
)

// Package ttc provides TTC (Two-Task Common) protocol message implementations
// for Oracle database communication, including authentication and session management.
// Authentication logon mode flags used in Oracle database connections.
const (

	// KpzPasswdEncrypted indicates password is encrypted
	KpzPasswdEncrypted = 0x00000100

	// KpzLogon indicates standard logon
	KpzLogon = 0x001

	// private const
	authSesskey         = "AUTH_SESSKEY"
	authVFRData         = "AUTH_VFR_DATA"
	authPDKDF2CSKSalt   = "AUTH_PBKDF2_CSK_SALT"
	authPassword        = "AUTH_PASSWORD"
	authAlterSession    = "AUTH_ALTER_SESSION"
	authProxyClientName = "AUTH_PROXY_CLIENT_NAME"

	authServerResponse = "AUTH_SVR_RESPONSE"

	authTerminal      = "AUTH_TERMINAL"
	authConnectString = "AUTH_CONNECT_STRING"
	authProgramNm     = "AUTH_PROGRAM_NM"
	authMachine       = "AUTH_MACHINE"
	authPid           = "AUTH_PID"
	authACL           = "AUTH_ACL"
	authSid           = "AUTH_SID"

	authSessionClientDrvnm = "SESSION_CLIENT_DRIVER_NAME"
	authSessionClientVsn   = "SESSION_CLIENT_VERSION"

	authPbkdf2SpeedyKey    = "AUTH_PBKDF2_SPEEDY_KEY"
	authClientCapabilities = "AUTH_CLIENT_CAPABILITIES"
	authPbkdf2VgenCount    = "AUTH_PBKDF2_VGEN_COUNT"
	authPbkdf2SderCount    = "AUTH_PBKDF2_SDER_COUNT"
	driverNameDefault      = "oracledb"
	authOraEdition         = "AUTH_ORA_EDITION"

	passwordBufferLength = 2112
	kolrugEnable         = 0x0001

	authScServerHost   = "AUTH_SC_SERVER_HOST"
	authScInstanceName = "AUTH_SC_INSTANCE_NAME"
	authScDbuniqueName = "AUTH_SC_DBUNIQUE_NAME"
	authScServiceName  = "AUTH_SC_SERVICE_NAME"
	authDbMountID      = "AUTH_DB_MOUNT_ID"

	databaseName = "DATABASE_NAME"
	serverHost   = "SERVER_HOST"
	instanceName = "INSTANCE_NAME"
	serviceName  = "SERVICE_NAME"

	SessionTimeZone = "SESSION_TIME_ZONE"

	defaultACLValue    = "4400"
	driverInternalName = "go_ttc_impl"

	driverDefaultResourceManagerID = "0000"

	// NLS session-related AUTH_* keys (camelCase variables mapped to server keys)
	authNlsLxlan        = "AUTH_NLS_LXLAN"
	authNlsLxcTerritory = "AUTH_NLS_LXCTERRITORY"
	authNlsLxcCurrency  = "AUTH_NLS_LXCCURRENCY"
	authNlsLxcIsoCurr   = "AUTH_NLS_LXCISOCURR"
	authNlsLxcNumerics  = "AUTH_NLS_LXCNUMERICS"
	authNlsLxcDateFm    = "AUTH_NLS_LXCDATEFM"
	authNlsLxcDateLang  = "AUTH_NLS_LXCDATELANG"
	authNlsLxcSort      = "AUTH_NLS_LXCSORT"
	authNlsLxNlsComp    = "AUTH_NLS_LXNLSCOMP"
	authNlsLxcCalendar  = "AUTH_NLS_LXCCALENDAR"
	authNlsLxcUnionCur  = "AUTH_NLS_LXCUNIONCUR"
	authNlsLxcTimeFm    = "AUTH_NLS_LXCTIMEFM"
	authNlsLxcStmpFm    = "AUTH_NLS_LXCSTMPFM"
	authNlsLxcTtznfm    = "AUTH_NLS_LXCTTZNFM"
	authNlsLxcStznfm    = "AUTH_NLS_LXCSTZNFM"

	// Additional SESSION_* NLS keys used by server piggybacks (OCSSYNC)
	sessionNlsLxcCharset   = "SESSION_NLS_LXCCHARSET"
	sessionNlsLxcNlsLenSem = "SESSION_NLS_LXCNLSLENSEM"
	sessionNlsLxcNcharExcp = "SESSION_NLS_LXCNCHAREXCP"
	sessionNlsLxcNcharImp  = "SESSION_NLS_LXCNCHARIMP"
)

// oAuth represents an Oracle Authentication (OAUTH) TTC message used for database login.
// It implements the common.Message interface and handles user authentication,
// session setup, and credential management for Oracle database connections.
type oAuth struct {
	// header is the TTC-compatible envelope that emits the oLobOps function
	// metadata.
	header              driverCommon.Marshallable
	enableTempLobRefCnt driverCommon.B1Array
	encryptedSK         []byte
	alterSession        driverCommon.B1Array
	clientName          []byte // proxy client name
	editionName         []byte
	salt                []byte //SHA-1 salt
	encryptedKB         []byte //encrypted Kb (second key fragment)

	_bUseO5Logon    bool
	verifierType    int //we expect ORCL7 or SHA-1 for now.
	isSessionTZ     bool
	sessionTimeZone string

	keyValList *keyValueList
	user       driverCommon.B1Array
	logonMode  int64

	PBKDF2Salt              []byte
	PBKDF2VgenCount         int
	PBKDF2SderCount         int
	o5logonHelper           *o5Logon
	timezoneAsRegion        bool
	enableTempLobRefCntBool bool
	connectString           []byte
	_hasO5LNPSupport        bool
	_hasO7LMRSupport        bool
}

// NewOAuth creates a new Oracle Authentication (OAUTH) TTC message with the legacy
// TTIFUN header format. The returned oAuth value is pre-initialized via _oauthInit
// and ready to be configured with credentials and session attributes before marshal.
func NewOAuth() driverCommon.Message[driverCommon.MessageType] {
	t := &oAuth{
		header:           &ttiFunHeader{_funcType: oauth},
		_hasO5LNPSupport: false,
		_hasO7LMRSupport: false,
		keyValList:       newKeyValueList(),
	}
	t._oauthInit()
	return t
}

// NewOAuth18 creates a new Oracle Authentication (OAUTH) TTC message using the
// TTIFUN header format (ttiFunHeader18) while reusing the standard oAuth
// initialization. It is returned as a Message interface ready for configuration.
func NewOAuth18() driverCommon.Message[driverCommon.MessageType] {
	t := &oAuth{
		header:           &ttiFunHeader18{ttiFunHeader: &ttiFunHeader{_funcType: oauth}},
		_hasO5LNPSupport: false,
		_hasO7LMRSupport: false,
		keyValList:       newKeyValueList(),
	}
	t._oauthInit()
	return t
}

func (o *oAuth) _oauthInit() {

	if o.enableTempLobRefCntBool {
		o.enableTempLobRefCnt = driverCommon.StringToB1Array(strconv.Itoa(kolrugEnable))
	} else {
		o.enableTempLobRefCnt = driverCommon.StringToB1Array(strconv.Itoa(0))
	}

	o.o5logonHelper = newO5Logon(true) // TODO Why is this hard-coded here ???

	o.setSessionFields()
}

func (o *oAuth) setHasO5LNPSupport(hasO5LNPSupport bool) {
	o._hasO5LNPSupport = hasO5LNPSupport
}
func (o *oAuth) setBUseO5Logon(use bool) {
	o._bUseO5Logon = use
}
func (o *oAuth) setHasO7LMRSupport(hasO7LMRSupport bool) {
	o._hasO7LMRSupport = hasO7LMRSupport
}

func (o *oAuth) setSessionFields() {
	// TODO : why not reuse utility.GetTimeZoneBytes() ?

	defaultTimeZone := time.Now().Location().String()
	// TODO: ZONEIDMAP.isValidRegion(defaultTimeZone)
	loc := time.Now().Location()
	_, offset := time.Now().In(loc).Zone()

	// Convert offset (in seconds) to hours and minutes
	hr := offset / 3600
	mi := int(math.Abs(float64((offset / 60) % 60)))

	// Format as string like +05:30 or -08:00
	if hr < 0 {
		defaultTimeZone = fmt.Sprintf("%d:%02d", hr, mi)
	} else {
		defaultTimeZone = fmt.Sprintf("+%d:%02d", hr, mi)
	}

	o.sessionTimeZone = defaultTimeZone

	nlslanguage := os.Getenv("NLS_LANGUAGE")
	// nlslanguage := CharacterSetMetaData.getNLSLanguage(Locale.getDefault(Category.FORMAT))

	if nlslanguage == "" {
		nlslanguage = "AMERICAN"
	}
	quotedNLSLanguage := oracleutils.EnquoteLiteral(nlslanguage)
	alterNLSLanguage := "NLS_LANGUAGE=" + quotedNLSLanguage + " "

	nlsterritory := os.Getenv("NLS_TERRITORY")
	// nlsterritory := CharacterSetMetaData.getNLSTerritory(Locale.getDefault(Category.FORMAT))
	if nlsterritory == "" {
		nlsterritory = "AMERICA"
	}
	quotedNLSTerritory := oracleutils.EnquoteLiteral(nlsterritory)
	alterNLSTerritory := "NLS_TERRITORY=" + quotedNLSTerritory + " "

	// Note, isSessionTZ is currently always true ...
	if alterNLSLanguage != "" || alterNLSTerritory != "" || o.isSessionTZ {
		quotedTimeZone := oracleutils.EnquoteLiteral(o.sessionTimeZone)
		timezone := "TIME_ZONE=" + quotedTimeZone + " "

		doAlter := "ALTER SESSION SET " +
			timezone +
			alterNLSLanguage +
			alterNLSTerritory

		o.alterSession = driverCommon.StringToB1Array(doAlter)

		(o.alterSession)[len(o.alterSession)-1] = 0
	}

}

// GetMsgCode returns the TTC message code for this oAuth message.
// It implements the common.Message interface and returns TTIFUN.
func (o *oAuth) GetMsgCode() driverCommon.MessageType {
	return TTIFUN
}

func (o *oAuth) GetFuncCode() driverCommon.FunctionType {
	return oauth
}

// MarshalTo serializes the oAuth message to the provided marshaller engine.
// It implements the common.Marshallable interface and writes the TTC protocol
// data for the authentication request, including user credentials and session parameters.
func (o *oAuth) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	err := o.header.MarshalTo(ctx, engine)
	if err != nil {
		common.Odl.Error("oAuth.MarshalTo: header marshal failed",
			"error", err,
			"msgCode", o.GetMsgCode(),
			"FunCode", o.GetFuncCode(),
		)
		return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}

	if len(o.user) > 0 {
		err = engine.MarshalPTR(ctx) // authusr*
		if err != nil {
			common.Odl.Warn("Error marshalling authusr", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
		err = engine.MarshalSB4(ctx, driverCommon.SB4(len(o.user))) // authusrl
		if err != nil {
			common.Odl.Warn("Error marshalling authusrl", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	} else {
		err = engine.MarshalNullPTR(ctx) // authusr*
		if err != nil {
			common.Odl.Warn("Error marshalling authusr", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
		err = engine.MarshalSB4(ctx, 0)
		if err != nil {
			common.Odl.Warn("Error marshalling authusrl", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	}
	err = engine.MarshalUB4(ctx, driverCommon.UB4(o.logonMode)) // authflg
	if err != nil {
		common.Odl.Warn("Error marshalling authflg", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	err = engine.MarshalPTR(ctx) // authivl
	if err != nil {
		common.Odl.Warn("Error marshalling authivl", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	err = engine.MarshalUB4(ctx, driverCommon.UB4(o.keyValList.Len())) // authivln
	if err != nil {
		common.Odl.Warn("Error marshalling authivln", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	err = engine.MarshalPTR(ctx) // authovl
	if err != nil {
		common.Odl.Warn("Error marshalling authovl", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	err = engine.MarshalPTR(ctx) // authovln
	if err != nil {
		common.Odl.Warn("Error marshalling authovln", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	// --------pisdef finished. Now send data
	if len(o.user) > 0 {
		err = engine.MarshalChar(ctx, o.user)
		if err != nil {
			common.Odl.Warn("Error marshalling oauth", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	}
	err = ((driverCommon.Marshallable)(o.keyValList)).MarshalTo(ctx, engine)
	if err != nil {
		common.Odl.Warn("Error marshalling oauth", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	return nil

}

// _isPrivilegedLogon checks if privileged flags are on is the givne mode
func _isPrivilegedLogon(mode int64) bool {
	return mode&(common.KpzLogonSysdba.Value()|common.KpzLogonSysoper.Value()) != 0
}

// Prepares for the execution of an oauth RPC. This method initializes the
// fields of this object with values that are sent in the TTI message that
// Marshal will write.
func (o *oAuth) prepareForOAUTH(luser driverCommon.B1Array,
	lpassword []byte,
	speedyKey []byte) error {

	if (lpassword == nil || len(lpassword) == 0) && !_isPrivilegedLogon(o.logonMode) {
		return common.NewOracleError(oracleErrors.InvalidCredential, nil, "empty password for non privileged logon")
	}

	o.initializeLogonModeForOAUTH(luser, o.logonMode, lpassword)
	o.setPasswordKeyValsForOAUTH(lpassword, speedyKey)
	o.setVSessionKeyValsForOAUTH()
	o.setAlterSessionKeyValsForOAUTH()
	o.setDriverIdentityKeyValsForOAUTH()

	return nil
}

// Initializes logonMode before executing an oauth call.
// llogonMode Logon mode specified by an external caller.
// for instance.
func (o *oAuth) initializeLogonModeForOAUTH(luser driverCommon.B1Array, llogonMode int64, lpassword []byte) {
	o.logonMode |= KpzLogon

	o.logonMode |= llogonMode

	if len(luser) != 0 && lpassword != nil {
		o.logonMode |= KpzPasswdEncrypted
	}
}

var _authPasswordKey = driverCommon.StringToB1Array(authPassword)
var _authPbkdf2SpeedyKey = driverCommon.StringToB1Array(authPbkdf2SpeedyKey)
var _authSesskey = driverCommon.StringToB1Array(authSesskey)

// Adds password-related key-value pairs to the authentication request, including encrypted password and speedy key if provided.
func (o *oAuth) setPasswordKeyValsForOAUTH(lpassword []byte, speedyKey []byte) {
	if lpassword != nil {
		o.keyValList.PushBack(&driverCommon.KeyValue{Key: _authPasswordKey, Value: lpassword})
	}

	if speedyKey != nil {
		o.keyValList.PushBack(&driverCommon.KeyValue{Key: _authPbkdf2SpeedyKey, Value: speedyKey})
	}

	if o.encryptedKB != nil { /* o5logon: send AUTH_SESSKEY */
		o.keyValList.PushBack(&driverCommon.KeyValue{Key: _authSesskey, Value: o.encryptedKB, Flag: 1})
	}
}

var _authProxyClientNameKey = driverCommon.StringToB1Array(authProxyClientName)

// Adds virtual session key-value pairs to the authentication request, including terminal,
// program, machine, and process information.
func (o *oAuth) setVSessionKeyValsForOAUTH() {

	o.keyValList.PushBackList(_keyValStaticInfoForOAuth1)
	// fill non-static information
	v := _keyValStaticInfoForOAuthConnectString.Value.(*driverCommon.KeyValue)
	v.Value = o.connectString
	if len(o.clientName) != 0 {
		o.keyValList.PushBack(&driverCommon.KeyValue{Key: _authProxyClientNameKey, Value: o.clientName})
	}

	o.keyValList.PushBackList(_keyValStaticInfoForOAuth2)

}

var _authOraEditionKey = driverCommon.StringToB1Array(authOraEdition)
var _authSessionClientDrvnmKey = driverCommon.StringToB1Array(authSessionClientDrvnm)
var _authSessionClientVsnKey = driverCommon.StringToB1Array(authSessionClientVsn)

// setDriverIdentityKeyValsForOAUTH adds driver identity key-value pairs
// to the authentication request for Oracle database authentication (oauth).
// This includes the driver name, version, and optionally the edition name
// if specified. These values help identify the client driver to the server.
func (o *oAuth) setDriverIdentityKeyValsForOAUTH() {
	if len(o.editionName) != 0 {
		o.keyValList.PushBack(&driverCommon.KeyValue{Key: _authOraEditionKey, Value: o.editionName})
	}

	o.keyValList.PushBack(&driverCommon.KeyValue{Key: _authSessionClientDrvnmKey, Value: currentDriverName})

	o.keyValList.PushBack(&driverCommon.KeyValue{Key: _authSessionClientVsnKey, Value: driverCommon.NumericDriverVersion})
}

var _authAlterSessionKey = driverCommon.StringToB1Array(authAlterSession)

func (o *oAuth) setAlterSessionKeyValsForOAUTH() {
	o.keyValList.PushBack(&driverCommon.KeyValue{Key: _authAlterSessionKey, Value: o.alterSession, Flag: 1})
}

func (o *oAuth) _initializeOAuthResponse(userPassword driverCommon.B1Array) error {

	err := o.validateKeySizeForOAUTH()
	if err != nil {
		common.Odl.Debug("failed to validate keys", "error", err)
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	passwordNet := userPassword

	encryptedPasswordLength := make([]int, 1)
	o5logonpassword := make([]byte, passwordBufferLength)
	for k := 0; k < passwordBufferLength; k++ {
		o5logonpassword[k] = 0
	}

	encryptedPkLength := make([]int, 1)
	encryptedPkTemp := make([]byte, passwordBufferLength)

	o.encryptedKB = make([]byte, len(o.encryptedSK))
	for k := 0; k < len(o.encryptedKB); k++ {
		o.encryptedKB[k] = 1
	}

	err = o.o5logonHelper.generateOAuthResponse(
		o.verifierType,
		o.salt,
		o.user,
		userPassword,
		passwordNet,
		o.encryptedSK,
		o.encryptedKB, //  new encryptedKB value is getting generated, so may be that is causing the error.
		o5logonpassword,
		encryptedPasswordLength,
		true,
		o._hasO5LNPSupport,
		o.PBKDF2Salt,
		o.PBKDF2VgenCount,
		o.PBKDF2SderCount,
		encryptedPkTemp,
		encryptedPkLength)
	if err != nil {
		return err
	}

	password := make([]byte, encryptedPasswordLength[0])
	copy(password[:encryptedPasswordLength[0]], o5logonpassword)

	var encryptedPk []byte
	if o.verifierType == ZtvtSha512 && o._hasO7LMRSupport {
		encryptedPk = make([]byte, encryptedPkLength[0])
		copy(encryptedPk[:encryptedPkLength[0]], encryptedPkTemp)
	} else {
		encryptedPk = nil
	}

	err = o.prepareForOAUTH(o.user,
		password,
		encryptedPk)
	if err != nil {
		return err
	}

	return nil
}

const (
	_encryptedSKO3logonLength      = 16
	_encryptedSKO5logonSmallLength = 64
	_encryptedSKO5logonLargeLength = 96
)

func (o *oAuth) validateKeySizeForOAUTH() error {
	if o.encryptedSK == nil {
		return errors.New("encryptedSK can't be nil")
	}
	if o._bUseO5Logon {
		if len(o.encryptedSK) != _encryptedSKO5logonSmallLength && len(o.encryptedSK) != _encryptedSKO5logonLargeLength {
			return errors.New("invalid encryptedSK size with O5logon")
		}
	} else {
		// Should always be 16 bytes long (16 nobbles)
		if len(o.encryptedSK) > _encryptedSKO3logonLength {
			return errors.New("invalid encryptedSK size without O5logon")
		}
	}
	return nil
}

// getLogonHelper gets the logon helper
func (o *oAuth) getLogonHelper() *o5Logon {
	return o.o5logonHelper
}

func (o *oAuth) setSalt(salt driverCommon.B1Array) {
	o.salt = salt
}

func (o *oAuth) setEncryptedSK(sk driverCommon.B1Array) {
	o.encryptedSK = sk
}

func (o *oAuth) setVerifierType(verifierType int) error {
	if verifierType != ZtvtOrcl7 &&
		verifierType != ZtvtMd5 &&
		verifierType != ZtvtSmd5 &&
		verifierType != ZtvtSh1 &&
		verifierType != ZtvtSSH1 &&
		verifierType != ZtvtSha512 {
		common.Odl.Debug("Unsupported verifier type", "verifier type", verifierType)
		return common.NewOracleError(oracleErrors.AuthenticatorError, nil, nil)

	}
	o.verifierType = verifierType
	return nil
}

func (o *oAuth) setPBKDF2Salt(salt driverCommon.B1Array) {
	o.PBKDF2Salt = salt
}

func (o *oAuth) setPBKDF2VgenCount(count int) {
	o.PBKDF2VgenCount = count
}

func (o *oAuth) setPBKDF2SderCount(count int) {
	o.PBKDF2SderCount = count
}

func (o *oAuth) setUser(username driverCommon.B1Array) {
	o.user = username
}

func (o *oAuth) setConnectString(connectString string) {
	o.connectString = driverCommon.StringToB1Array(connectString)
}

// setLogonMode sets the logon mode of this oauth
// see KpzLogonSysdba KpzLogonSysoper
func (o *oAuth) setLogonMode(mode int64) {
	o.logonMode = mode
}

// ---- oauth Function Reply (TTIRPA) Message ----

// OAuthRPA represents an Oracle Authentication Reply (TTIRPA) TTC message.
// It handles the server's response to an authentication request, containing
// connection properties and authentication results.
type OAuthRPA struct {
	outNbPairs       driverCommon.UB2
	sessionTimeZone  string
	connectionValues *driverCommon.Properties[string]
}

// NewOAuthRPA creates a new Oracle Authentication Reply (TTIRPA) TTC message.
// It initializes the OAuthRPA struct and returns it as a Message interface,
// ready to unmarshal server authentication responses.
func NewOAuthRPA() driverCommon.Message[driverCommon.MessageType] {
	return &OAuthRPA{}
}

// GetMsgCode returns the TTC message code for this OAuthRPA message.
// It implements the common.Message interface and returns TTIRPA.
func (rpa *OAuthRPA) GetMsgCode() driverCommon.MessageType {
	return TTIRPA
}

// UnMarshalFrom deserializes the OAuthRPA message from the provided marshaller engine.
// It implements the common.UnMarshallable interface and reads the TTC protocol
// data for the authentication reply, extracting connection properties and session information.
func (rpa *OAuthRPA) UnMarshalFrom(ctx context.Context, engine driverCommon.Marshaller) error {
	common.Odl.Debug("unmarshal OAUTH RPA starts")
	var err error
	rpa.outNbPairs, err = engine.UnmarshalUB2(ctx)
	if err != nil {
		common.Odl.Warn("Unable to unmarshal oAuth RPA, cant' get pairs count", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, nil)
	}
	if err = validateAuthKeyValueLength("pairs", driverCommon.SB4(rpa.outNbPairs), maxAuthKeyValuePairs); err != nil {
		return err
	}

	//for o5logon, the flag is the verifier type
	keyValueList := newPreallocatedKeyValueList(int(rpa.outNbPairs))
	err = ((driverCommon.UnMarshallable)(keyValueList)).UnMarshalFrom(ctx, engine)
	if err != nil {
		common.Odl.Warn("Unable to unmarshal oAuth RPA, cant' unmarshal key/value pairs", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, nil)
	}

	rpa.connectionValues = driverCommon.NewProperties[string]()
	for kv := keyValueList.Front(); kv != nil; kv = kv.Next() {
		rpa.connectionValues.SetProperty(
			strings.TrimSpace(driverCommon.B1ArrayToString(kv.Value.(*driverCommon.KeyValue).Key)),
			kv.Value.(*driverCommon.KeyValue).Value)
	}

	if !rpa.connectionValues.ContainsKey(authDbMountID) {
		common.Odl.Warn("Unable to unmarshal oAuth RPA, missing property", "property", authDbMountID)
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, nil)
	}

	authDBMountID, err := rpa.connectionValues.GetTrimmedString(authDbMountID)
	if err != nil {
		common.Odl.Warn("Unable to get authDbMountID while unmarshalling oAuth RPA", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, nil)

	}

	parseLong, err := strconv.ParseInt(authDBMountID, 10, 64)
	if err != nil {
		common.Odl.Warn("Unable to parse int while unmarshalling oAuth RPA", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, nil)
	}

	if authDBMountID != "" &&
		parseLong == 0 {
		common.Odl.Warn("Database is in NOMOUNT state")
		return common.NewOracleError(oracleErrors.DatabaseMountStateError, nil, nil)
	}

	// Store FCF connection properties:
	rpa.connectionValues.SetProperty(serverHost,
		rpa.connectionValues.GetProperty(
			authScServerHost))

	rpa.connectionValues.SetProperty(instanceName,
		rpa.connectionValues.GetProperty(
			authScInstanceName))

	rpa.connectionValues.SetProperty(databaseName,
		rpa.connectionValues.GetProperty(
			authScDbuniqueName))

	rpa.connectionValues.SetProperty(serviceName,
		rpa.connectionValues.GetProperty(
			authScServiceName))

	rpa.connectionValues.SetProperty(SessionTimeZone, rpa.sessionTimeZone)
	common.Odl.Debug("unmarshal OAUTH RPA complete")
	return nil
}
