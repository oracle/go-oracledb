/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** See the LICENSE file for license terms.
 */

package ttc

import (
	"github.com/oracle/go-oracledb/v26/internal/common"
	ttcLob "github.com/oracle/go-oracledb/v26/internal/driver/ttc/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// ttcLobLocator adapts TTC's mutable locator state to the opaque reference
// registry contract. The locator and its protocol fields remain private to TTC.
type ttcLobLocator struct {
	locator *locator
}

var _ ttcLob.Locator = ttcLobLocator{}

func (locator ttcLobLocator) Snapshot() []byte {
	if locator.locator == nil {
		return nil
	}
	return append([]byte(nil), locator.locator.locatorBytes...)
}

func (locator ttcLobLocator) MarkReleased() {
	if locator.locator != nil {
		locator.locator.markReleased()
	}
}

func (locator ttcLobLocator) ReferenceIdentity() (ttcLob.ReferenceID, error) {
	loc := locator.locator
	if loc == nil || len(loc.locatorBytes) < kolbLobIDOffset+kolbLobIDLength {
		return "", common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "lob-reference", "lob", "missing complete LOB ID")
	}
	if loc.isQuasiLocator() || loc.isValueBasedLocator() {
		return "", common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "lob-reference", "lob", "value-based locator")
	}
	if loc.locatorBytes[koll2FlagOffset]&kolblInitializedFlag == 0 {
		return "", common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "lob-reference", "lob", "uninitialized locator")
	}
	if !loc.isTemporaryLocator() && !loc.isAbstractLocator() {
		return "", common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "lob-reference", "lob", "persistent locator")
	}
	return ttcLob.ReferenceID(string(loc.locatorBytes[kolbLobIDOffset : kolbLobIDOffset+kolbLobIDLength])), nil
}
