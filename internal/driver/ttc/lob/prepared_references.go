/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** See the LICENSE file for license terms.
 */

package lob

import (
	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// PreparedReferences owns the local release obligations associated with one
// prepared operation, such as a statement's streamed LOB binds.
type PreparedReferences struct {
	registry ReferenceRegistry
	onUnsafe func()
	values   []Reference
	released bool
}

// NewPreparedReferences creates an owner for references prepared by registry.
// onUnsafe is called when release fails or the owner is explicitly abandoned.
func NewPreparedReferences(registry ReferenceRegistry, onUnsafe func()) (*PreparedReferences, error) {
	if registry == nil || onUnsafe == nil {
		return nil, errInvalidPreparedReferenceOwner()
	}
	return &PreparedReferences{registry: registry, onUnsafe: onUnsafe}, nil
}

// Add retains locator and transfers its release obligation to this owner.
func (owner *PreparedReferences) Add(locator Locator) error {
	if owner == nil || owner.released {
		return errInvalidPreparedReferenceOwner()
	}
	reference, err := owner.registry.Retain(locator)
	if err != nil {
		return err
	}
	owner.values = append(owner.values, reference)
	return nil
}

// AddReference transfers an existing opaque reference obligation to this owner.
func (owner *PreparedReferences) AddReference(reference Reference) error {
	if owner == nil || owner.released || reference == nil {
		return errInvalidPreparedReferenceOwner()
	}
	owner.values = append(owner.values, reference)
	return nil
}

// ReleaseAll releases references in reverse creation order. On the first
// failure it abandons the owning session because later cleanup is unsafe.
func (owner *PreparedReferences) ReleaseAll() error {
	if owner == nil || owner.released || len(owner.values) == 0 {
		return nil
	}
	owner.released = true
	for index := len(owner.values) - 1; index >= 0; index-- {
		if err := owner.registry.Release(owner.values[index]); err != nil {
			owner.onUnsafe()
			return err
		}
	}
	return nil
}

// Abandon transfers all cleanup obligations to physical-session teardown.
func (owner *PreparedReferences) Abandon() {
	if owner == nil || owner.released {
		return
	}
	owner.released = true
	owner.onUnsafe()
}

// Len returns the number of references owned by this set.
func (owner *PreparedReferences) Len() int {
	if owner == nil {
		return 0
	}
	return len(owner.values)
}

// Released reports whether cleanup or abandonment has already been performed.
func (owner *PreparedReferences) Released() bool {
	return owner == nil || owner.released
}

func errInvalidPreparedReferenceOwner() error {
	return common.NewOracleError(oracleErrors.InvalidLobInput, nil, "prepared reference owner")
}
