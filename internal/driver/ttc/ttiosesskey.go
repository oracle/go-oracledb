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
	"fmt"
	"strconv"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	minimumPBKDF2VgenCount = 4096
	minimumPBKDF2SderCount = 3
	maximumPBKDF2VgenCount = 100000000 // see kztv.h
	maximumPBKDF2SderCount = 100000000 // see kztv.h
	maxAuthKeyValuePairs   = 128
)

var _minimumPBKDF2VgenCountStr = driverCommon.B1Array(strconv.Itoa(minimumPBKDF2VgenCount))

func isValidPBKDF2VgenCount(count int) bool {
	return count >= minimumPBKDF2VgenCount && count <= maximumPBKDF2VgenCount
}

func isValidPBKDF2SderCount(count int) bool {
	return count >= minimumPBKDF2SderCount && count <= maximumPBKDF2SderCount
}

// oSessionKey represents an Oracle Session Key (OSESSKEY) TTC message used for session establishment.
// It implements the common.Message interface and handles session initialization and authentication.
type oSessionKey struct {
	// header is the TTC-compatible envelope that emits the oSesskey function metadata.
	header              driverCommon.Marshallable
	keyValList          *keyValueList
	currentTTCSeqNumber uint8
	sessionTimeZone     string

	// caller needs to set these fields
	user      driverCommon.B1Array
	sysUser   string
	logonMode uint8
}

// Equals implements Comparable interface
func (o *oSessionKey) Equals(c *oSessionKey) bool {
	if c == nil {
		return false
	}

	if o.currentTTCSeqNumber != c.currentTTCSeqNumber {
		return false
	}
	if strings.Compare(o.sessionTimeZone, c.sessionTimeZone) != 0 {
		return false
	}
	if !o.user.Equals(c.user) {
		return false
	}
	if strings.Compare(o.sysUser, c.sysUser) != 0 {
		return false
	}
	if o.logonMode != c.logonMode {
		return false
	}
	return o.keyValList.Equals(c.keyValList)
}

// NewOSesskey creates a new Oracle Session Key (OSESSKEY) TTC message.
// It initializes the oSesskey struct with default session information and returns it as a Message interface.
// The returned oSesskey message is ready to be configured for session establishment.
func NewOSesskey() driverCommon.Message[driverCommon.MessageType] {
	obj := &oSessionKey{
		header: &ttiFunHeader{_funcType: oSesskey},
	}
	return obj
}

// NewOSesskey18 creates a new Oracle Session Key (OSESSKEY) TTC message for protocol 18 and above.
// It initializes the oSesskey struct with a TTC 18 header and default session information, returning it as a Message interface.
// The returned oSesskey message is ready to be configured for session establishment using the 18c header format.
func NewOSesskey18() driverCommon.Message[driverCommon.MessageType] {
	obj := &oSessionKey{
		header: &ttiFunHeader18{ttiFunHeader: &ttiFunHeader{_funcType: oSesskey}},
	}
	return obj
}

// GetMsgCode returns the TTC message code for this oSesskey message.
// It implements the common.Message interface and returns TTIFUN.
func (sess *oSessionKey) GetMsgCode() driverCommon.MessageType {
	return TTIFUN
}

func (sess *oSessionKey) GetFuncCode() driverCommon.FunctionType {
	return oSesskey
}

// MarshalTo serializes the oSesskey message to the provided marshaller engine.
// It implements the common.Marshallable interface and writes the TTC protocol
// data for the session key establishment request, including user and session information.
func (sess *oSessionKey) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	sess.buildKeyValueList()
	err := sess.header.MarshalTo(ctx, engine)
	if err != nil {
		common.Odl.Error("oAuth.MarshalTo: header marshal failed",
			"error", err,
			"msgCode", sess.GetMsgCode(),
			"FunCode", sess.GetFuncCode(),
		)
		return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[sess.GetMsgCode()])
	}

	if len(sess.user) > 0 {
		err = engine.MarshalPTR(ctx) // authusr*
		if err != nil {
			common.Odl.Warn("Error marshalling Osesskey authusr", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
		err := engine.MarshalSB4(ctx, driverCommon.SB4(len(sess.user))) // authusrl
		if err != nil {
			common.Odl.Warn("Error marshalling Osesskey authusrl", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	} else {
		err = engine.MarshalNullPTR(ctx) // authusr*
		if err != nil {
			common.Odl.Warn("Error marshalling Osesskey authusr", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
		err := engine.MarshalSB4(ctx, 0)
		if err != nil {
			common.Odl.Warn("Error marshalling Osesskey authusrl", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	}
	err = engine.MarshalUB4(ctx, driverCommon.UB4(sess.logonMode)) // authflg
	if err != nil {
		common.Odl.Warn("Error marshalling Osesskey authflg", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	err = engine.MarshalPTR(ctx) // authivl
	if err != nil {
		common.Odl.Warn("Error marshalling Osesskey authivl", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	err = engine.MarshalUB4(ctx, driverCommon.UB4(sess.keyValList.Len())) // authivln
	if err != nil {
		common.Odl.Warn("Error marshalling Osesskey authivln", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	err = engine.MarshalPTR(ctx) // authovl
	if err != nil {
		common.Odl.Warn("Error marshalling Osesskey authovl", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	err = engine.MarshalPTR(ctx) // authovln
	if err != nil {
		common.Odl.Warn("Error marshalling Osesskey authovln", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	// --------pisdef finished. Now send data

	if len(sess.user) != 0 {
		err = engine.MarshalChar(ctx, sess.user)
		if err != nil {
			common.Odl.Warn("Error marshalling Osesskey", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	}

	err = sess.keyValList.MarshalTo(ctx, engine)
	if err != nil {
		common.Odl.Warn("Error marshalling Osesskey", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	return nil
}

func (sess *oSessionKey) setUser(user driverCommon.B1Array) {
	sess.user = user
}

func (sess *oSessionKey) buildKeyValueList() {

	sess.logonMode = sess.logonMode | KpzLogon

	sess.keyValList = newKeyValueList()
	sess.keyValList.PushBackList(_keyValStaticInfoForOsesskey)

}

// oSesskeyRPA represents an Oracle Session Key Reply (TTIRPA) TTC message.
// It handles the server's response to a session key establishment request,
// containing authentication and session parameters returned by the Oracle server.
type oSesskeyRPA struct {
	sessionTimeZone  string
	connectionValues *driverCommon.Properties[string]
}

// NewOSesskeyRPA creates a new Oracle Session Key Reply (TTIRPA) TTC message.
// It initializes the oSesskeyRPA struct and returns it as a Message interface,
// ready to unmarshal server responses to session key establishment requests.
func NewOSesskeyRPA() driverCommon.Message[driverCommon.MessageType] {
	return &oSesskeyRPA{}
}

// GetMsgCode returns the TTC message code for this oSesskeyRPA message.
// It implements the common.Message interface and returns TTIRPA.
func (rpa *oSesskeyRPA) GetMsgCode() driverCommon.MessageType {
	return TTIRPA
}

// UnMarshalFrom deserializes the oSesskeyRPA message from the provided marshaller engine.
// It implements the common.UnMarshallable interface and reads the TTC protocol
// data for the session key establishment reply, extracting authentication parameters.
func (rpa *oSesskeyRPA) UnMarshalFrom(ctx context.Context, engine driverCommon.Marshaller) error {

	outNbPairs, err := engine.UnmarshalUB2(ctx)
	if err != nil {
		common.Odl.Debug("Error Unmarshalling Osesskey RPA nbParis", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, nil)
	}
	if err = validateAuthKeyValueLength("pairs", driverCommon.SB4(outNbPairs), maxAuthKeyValuePairs); err != nil {
		return err
	}
	keyValueList := newPreallocatedKeyValueList(int(outNbPairs))

	err = keyValueList.UnMarshalFrom(ctx, engine)
	if err != nil {
		common.Odl.Debug("error unmarshalling Osesskey RPA key value list", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, nil)
	}

	rpa.connectionValues = driverCommon.NewProperties[string]()
	for kv := keyValueList.Front(); kv != nil; kv = kv.Next() {
		rpa.connectionValues.SetProperty(
			driverCommon.B1ArrayToString(kv.Value.(*driverCommon.KeyValue).Key),
			kv.Value.(*driverCommon.KeyValue))
	}

	// TODO change to variables
	if !rpa.connectionValues.ContainsKey(authSesskey) {
		common.Odl.Warn("Missing key from OSession Keys", "key", authSesskey)
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, nil)
	}
	if !rpa.connectionValues.ContainsKey(authVFRData) {
		common.Odl.Warn("Missing key from OSession Keys", "key", authVFRData)
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, nil)
	}
	if !rpa.connectionValues.ContainsKey(authPDKDF2CSKSalt) {
		common.Odl.Warn("Missing key from OSession Keys", "key", authPDKDF2CSKSalt)
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, nil)
	}
	if !rpa.connectionValues.ContainsKey(authPbkdf2VgenCount) {
		common.Odl.Warn("Missing key from OSession Keys", "key", authPbkdf2VgenCount)
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, nil)
	}
	if !rpa.connectionValues.ContainsKey(authPbkdf2SderCount) {
		common.Odl.Warn("Missing key from OSession Keys", "key", authPbkdf2SderCount)
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, nil)
	}

	PBKDF2Vgen, _ := rpa.connectionValues.GetProperty(authPbkdf2VgenCount).(*driverCommon.KeyValue)
	PBKDF2VgenCount, err := strconv.Atoi(driverCommon.B1ArrayToString(PBKDF2Vgen.Value))
	if err != nil {
		return err
	}
	if !isValidPBKDF2VgenCount(PBKDF2VgenCount) {
		// minimumPBKDF2VgenCount is the minimum agreed count.
		// If its less then most likely its a dirty server.
		common.Odl.Warn(fmt.Sprintf("PBKDF2VgenCount outside allowed range [%d, %d] received: [%d]",
			minimumPBKDF2VgenCount,
			maximumPBKDF2VgenCount,
			PBKDF2VgenCount))
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, "invalid PBKDF2VgenCount")
	}

	PBKDF2Sder, _ := rpa.connectionValues.GetProperty(authPbkdf2SderCount).(*driverCommon.KeyValue)
	PBKDF2SderCount, err := strconv.Atoi(driverCommon.B1ArrayToString(PBKDF2Sder.Value))
	if err != nil {
		common.Odl.Warn("Error unmarshalling Osesskey RPA, wrong value for authPbkdf2SderCount", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, nil)
	}

	if !isValidPBKDF2SderCount(PBKDF2SderCount) {
		// 3 is the minimum agreed limit
		common.Odl.Warn(fmt.Sprintf("PBKDF2SderCount outside allowed range [%d, %d] received: [%d]",
			minimumPBKDF2SderCount,
			maximumPBKDF2SderCount,
			PBKDF2SderCount))
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, "invalid PBKDF2SderCount")
	}

	return nil

}
