/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively, the "Software"), free of charge and under any and all copyright
** and patent rights owned or freely licensable by each licensor hereunder covering
** either (i) the unmodified Software as contributed to or provided by such licensor,
** or (ii) the Larger Works (as defined below), to deal in both the Software and
** any Larger Work without restriction, including the rights to copy, create
** derivative works of, display, perform, and distribute the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 */

package ttc

import (
	"context"
	"errors"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestRunOTxEnSetupAndTransportFailures verifies the OTXEN setup, write,
// flush, and read failures returned by runOTxEn.
func TestRunOTxEnSetupAndTransportFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation txStateChangeOperation
		setup     func(*connection, *mockStreamer)
		wantError oracleErrors.ErrorCode
	}{
		{
			name:      "factory error",
			operation: otxenAbort,
			setup: func(conn *connection, _ *mockStreamer) {
				conn.shelf.RegisterMessageFactory(&mockFactory{returnErr: errors.New("factory failed")})
			},
			wantError: oracleErrors.InternalError,
		},
		{
			name:      "unsupported operation",
			operation: txStateChangeOperation(99),
			setup:     func(*connection, *mockStreamer) {},
			wantError: oracleErrors.InternalError,
		},
		{
			name:      "push error",
			operation: otxenAbort,
			setup: func(_ *connection, streamer *mockStreamer) {
				streamer.pushErr = errors.New("push failed")
			},
			wantError: oracleErrors.StreamerWriteError,
		},
		{
			name:      "flush error",
			operation: otxenAbort,
			setup: func(_ *connection, streamer *mockStreamer) {
				streamer.flushErr = errors.New("flush failed")
			},
			wantError: oracleErrors.StreamerWriteError,
		},
		{
			name:      "pull error",
			operation: otxenAbort,
			setup: func(_ *connection, streamer *mockStreamer) {
				streamer.pullErr = errors.New("pull failed")
			},
			wantError: oracleErrors.StreamerReadError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streamer := &mockStreamer{pullMsg: &mockOer{}}
			conn := newTransactionTestConnection(streamer)
			tt.setup(conn, streamer)

			tx := newTransaction(conn, context.Background())
			if got := transactionErrorCode(t, conn.runOTxEn(context.Background(), tt.operation, tx)); got != tt.wantError {
				t.Fatalf("runOTxEn error code = %s, want %s", got, tt.wantError)
			}
		})
	}
}

// TestRunOTxEnAdditionalResponsePaths verifies that runOTxEn continues after
// an unexpected TTIRPA type and completes when TTISTA is the terminal response.
func TestRunOTxEnAdditionalResponsePaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pullMsgs []common.Message[common.MessageType]
	}{
		{name: "TTISTA terminal response", pullMsgs: []common.Message[common.MessageType]{newTTISTA()}},
		{
			name: "unexpected TTIRPA type followed by TTISTA",
			pullMsgs: []common.Message[common.MessageType]{
				newOTxSeRPA(),
				newTTISTA(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streamer := &mockStreamer{pullMsgs: tt.pullMsgs}
			conn := newTransactionTestConnection(streamer)
			tx := newTransaction(conn, context.Background())

			if err := conn.runOTxEn(context.Background(), otxenAbort, tx); err != nil {
				t.Fatalf("runOTxEn returned error: %v", err)
			}
		})
	}
}
