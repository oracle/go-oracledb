/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package ttc

import (
	"context"
	"time"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

const blobBindCleanupTimeout = 5 * time.Second

type oracleBlobValue interface {
	OracleBlobValue() []byte
}

type blobLocator []byte

type blobBindExecutor struct {
	*lobExecutor
}

func newBlobBindExecutor(shelf *driverCommon.Shelf[driverCommon.MessageType]) *blobBindExecutor {
	executor := &blobBindExecutor{lobExecutor: newLobExecutor()}
	executor.SetShelf(shelf)
	return executor
}

// createTemporary uses Oracle's fixed-width temporary BLOB locator form. The
// signed, length-prefixed locator used for CLOB creation is not accepted for a
// binary LOB; byte 1 identifies the legacy temporary locator and charset 1 is
// the TTC sentinel required even though BLOB contents have no character set.
func (e *blobBindExecutor) createTemporary(ctx context.Context) (driverCommon.B1Array, error) {
	definition := NewLobDefinitionForTemporaryCreate(
		kolllTemp,
		0,
		driverCommon.UB8(DtyBlob),
		driverCommon.UB4(durationSession),
		false,
		1,
	)
	definition.sourceLocator.locatorBytes[0] = 0
	definition.sourceLocator.locatorBytes[1] = 0x54
	definition.fixedTemporaryLocator = true
	if err := e.execute(ctx, definition); err != nil {
		return nil, err
	}
	return definition.sourceLocator.locatorBytes, nil
}

func (e *blobBindExecutor) freeTemporary(ctx context.Context, locatorBytes blobLocator) error {
	definition := &lobDefinition{
		sourceLocator: newLocator(driverCommon.B1Array(locatorBytes), 0),
		operation:     kplobTmpFree,
	}
	return e.execute(ctx, definition)
}

func (e *blobBindExecutor) materialize(ctx context.Context, data []byte) (blobLocator, error) {
	locatorBytes, err := e.createTemporary(ctx)
	if err != nil {
		return nil, err
	}
	locator := newLocator(locatorBytes, 1)
	if _, err := e.write(ctx, locator, driverCommon.B1Array(data), driverCommon.UB8(len(data))); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), blobBindCleanupTimeout)
		_ = e.freeTemporary(cleanupCtx, blobLocator(locatorBytes))
		cancel()
		return nil, err
	}
	return blobLocator(locatorBytes), nil
}

type blobBindCleanup struct {
	executor *blobBindExecutor
	locators []blobLocator
}

// free releases every session-duration locator after the DML has copied it into
// the target BLOB. Retaining these locators until connection close leaks server
// temporary-LOB state in long-lived pools.
func (c *blobBindCleanup) free(ctx context.Context) error {
	if c == nil {
		return nil
	}
	var firstErr error
	for index := len(c.locators) - 1; index >= 0; index-- {
		if err := c.executor.freeTemporary(ctx, c.locators[index]); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
