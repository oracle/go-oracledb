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
	"slices"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
)

const ttiCookieVersion byte = 1
const zeroByte = 0

// LRU cache for TTC cookies
var ttcCookieCache = common.NewSafeLRUCache[*ttiCookie](50)

type ttiCookie struct {
	connectionProtocolVersion       driverCommon.UB2
	databaseCharSet                 driverCommon.UB2
	databaseCharSetFlag             byte
	databaseNCharSet                driverCommon.UB2
	databaseRuntimeCapabilities     []byte
	databaseCompileTimeCapabilities []byte
	databasePortage                 driverCommon.B1Array
}

// newTTICookieFromTTIpro copies negotiated server values into an independent cookie.
func newTTICookieFromTTIpro(pro *tTIpro) *ttiCookie {
	return &ttiCookie{
		connectionProtocolVersion:       pro.proSvrVer,
		databaseCharSet:                 pro.svrCharSet,
		databaseCharSetFlag:             pro.svrFlags,
		databaseNCharSet:                pro.nCharCharset,
		databaseRuntimeCapabilities:     slices.Clone(pro.serverCaps.runTimeCapabilities),
		databaseCompileTimeCapabilities: slices.Clone(pro.serverCaps.compileTimeCapabilities),
		databasePortage:                 slices.Clone(pro.svrPortDescription),
	}
}

// GetMsgCode identifies the TTC cookie message.
func (c *ttiCookie) GetMsgCode() driverCommon.MessageType {
	return TTICOOKIE
}

// MarshalTo writes the cookie payload and returns any marshalling error.
func (c *ttiCookie) MarshalTo(ctx context.Context, mar driverCommon.Marshaller) error {
	if err := mar.MarshalUB1(ctx, driverCommon.UB1(ttiCookieVersion)); err != nil {
		return err
	}

	if err := mar.MarshalUB1(ctx, driverCommon.UB1(c.connectionProtocolVersion)); err != nil {
		return err
	}
	databaseCharSetE := (uint16(c.databaseCharSet)&0xFF)<<8 | (uint16(c.databaseCharSet) >> 8)
	if err := mar.MarshalUB2(ctx, driverCommon.UB2(databaseCharSetE)); err != nil {
		return err
	}

	if err := mar.MarshalUB1(ctx, driverCommon.UB1(c.databaseCharSetFlag)); err != nil {
		return err
	}

	databaseNCharSetE := (uint16(c.databaseNCharSet)&0xFF)<<8 | (uint16(c.databaseNCharSet) >> 8)
	if err := mar.MarshalUB2(ctx, driverCommon.UB2(databaseNCharSetE)); err != nil {
		return err
	}

	portageLength := len(c.databasePortage) + 1
	if err := mar.MarshalUB1(ctx, driverCommon.UB1(portageLength)); err != nil {
		return err
	}
	if err := mar.MarshalB1Array(ctx, c.databasePortage); err != nil {
		return err
	}
	if err := mar.MarshalUB1(ctx, zeroByte); err != nil {
		return err
	}

	compileTimeCapabilitiesLength := len(c.databaseCompileTimeCapabilities)
	if err := mar.MarshalUB1(ctx, driverCommon.UB1(compileTimeCapabilitiesLength)); err != nil {
		return err
	}
	if err := mar.MarshalB1Array(ctx, c.databaseCompileTimeCapabilities); err != nil {
		return err
	}

	runtimeCapabilitiesLength := len(c.databaseRuntimeCapabilities)
	if err := mar.MarshalUB1(ctx, driverCommon.UB1(runtimeCapabilitiesLength)); err != nil {
		return err
	}
	return mar.MarshalB1Array(ctx, c.databaseRuntimeCapabilities)
}

// GetCachedTTCCookie returns the cookie for the database UUID and whether it exists.
func GetCachedTTCCookie(databaseUUID string) (*ttiCookie, bool) {
	cookie, found := ttcCookieCache.Get(databaseUUID)
	return cookie, found && cookie != nil
}

// newProtocolMessage reconstructs PRO and its client capabilities from cached values.
func (c *ttiCookie) newProtocolMessage() *tTIpro {
	pro := newTTIpro().(*tTIpro)
	pro.proSvrVer = c.connectionProtocolVersion
	pro.svrCharSet = c.databaseCharSet
	pro.svrFlags = c.databaseCharSetFlag
	pro.nCharCharset = c.databaseNCharSet
	pro.svrPortDescription = slices.Clone(c.databasePortage)
	pro.serverCaps.runTimeCapabilities = slices.Clone(c.databaseRuntimeCapabilities)
	pro.serverCaps.compileTimeCapabilities = slices.Clone(c.databaseCompileTimeCapabilities)
	pro.clientCaps.adjustCapabilityFrom(pro.serverCaps)
	return pro
}

// IsTTCCookieEligible requires optimization to be enabled and a nonblank database UUID.
func IsTTCCookieEligible(
	properties oracleconfig.OracleDriverProperties,
	databaseUUID string,
) bool {
	return properties.IsTTCCookieOptimizationEnabled() &&
		strings.TrimSpace(databaseUUID) != ""
}

// SeedTTCCookieForTest inserts a cookie for factory-selection tests and returns
// a cleanup function that removes the entry. The cookie is not intended
// for a real negotiation. Tests must use a unique database UUID.
func SeedTTCCookieForTest(databaseUUID string) func() {
	ttcCookieCache.Put(
		databaseUUID,
		&ttiCookie{connectionProtocolVersion: 6},
	)
	return func() {
		ttcCookieCache.Remove(databaseUUID)
	}
}

// ClearTTCCookieCacheForTest resets cached TTC negotiation data for integration tests.
func ClearTTCCookieCacheForTest() {
	ttcCookieCache.Clear()
}

// CorruptTTCCookieForTest replaces only the specified database's cached cookie
// with a stale copy, preserving the original value for existing users of it.
func CorruptTTCCookieForTest(databaseUUID string) (driverCommon.UB2, bool) {
	cookie, found := GetCachedTTCCookie(databaseUUID)
	if !found {
		return 0, false
	}
	stale := *cookie
	stale.connectionProtocolVersion = 0
	ttcCookieCache.Put(databaseUUID, &stale)
	return cookie.connectionProtocolVersion, true
}

// TTCCookieProtocolVersionForTest reports the version cached for this database.
func TTCCookieProtocolVersionForTest(databaseUUID string) (driverCommon.UB2, bool) {
	cookie, found := GetCachedTTCCookie(databaseUUID)
	if !found {
		return 0, false
	}
	return cookie.connectionProtocolVersion, true
}
