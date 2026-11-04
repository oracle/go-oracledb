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
	"io"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestReadCookieAuthenticationResponse_Success verifies password and token response
// sequences containing PRO, DTY, a warning, RPA, and successful OER.
func TestReadCookieAuthenticationResponse_Success(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		function driverCommon.FunctionType
		rpa      driverCommon.Message[driverCommon.MessageType]
	}{
		{name: "password", function: oSesskey, rpa: &oSesskeyRPA{}},
		{name: "token", function: oauth, rpa: &OAuthRPA{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shelf := newShelf[driverCommon.MessageType]()
			streamer := &mockStreamer{
				pullMsgs: []driverCommon.Message[driverCommon.MessageType]{
					newTTIpro(), NewTTIdty(), &tTIwrn{}, tc.rpa, &mockOer{},
				},
				pullErr: io.EOF,
			}

			response, err := readCookieAuthenticationResponse(context.Background(), shelf, streamer, tc.function)

			if err != nil {
				t.Fatalf("read authentication response: %v", err)
			}
			if response != tc.rpa {
				t.Fatalf("got RPA %p, want %p", response, tc.rpa)
			}
			if len(streamer.pullMsgs) != 0 {
				t.Error("reader returned before consuming the successful OER")
			}
		})
	}
}

// TestReadCookieAuthenticationResponse_TTIRENEG verifies that renegotiation
// returns the fallback signal immediately without an authentication RPA.
func TestReadCookieAuthenticationResponse_TTIRENEG(t *testing.T) {
	t.Parallel()
	shelf := newShelf[driverCommon.MessageType]()
	streamer := &mockStreamer{
		pullMsgs: []driverCommon.Message[driverCommon.MessageType]{&tTIReneg{}, &mockOer{}},
		pullErr:  io.EOF,
	}

	response, err := readCookieAuthenticationResponse(context.Background(), shelf, streamer, oSesskey)

	if !errors.Is(err, driverCommon.ErrTTIRENEG) {
		t.Fatalf("got error %v, want ErrTTIRENEG", err)
	}
	if response != nil {
		t.Errorf("got response %v, want nil", response)
	}
	if len(streamer.pullMsgs) != 1 {
		t.Error("reader continued reading after TTIRENEG")
	}
}

// TestReadCookieAuthenticationResponse_Failure verifies server rejection,
// successful OER without RPA, and response read failures.
func TestReadCookieAuthenticationResponse_Failure(t *testing.T) {
	t.Parallel()

	t.Run("server rejects authentication after RPA", func(t *testing.T) {
		shelf := newShelf[driverCommon.MessageType]()
		serverErr := errors.New("authentication rejected")
		streamer := &mockStreamer{
			pullMsgs: []driverCommon.Message[driverCommon.MessageType]{&OAuthRPA{}, &mockOer{err: serverErr}},
			pullErr:  io.EOF,
		}

		response, err := readCookieAuthenticationResponse(context.Background(), shelf, streamer, oauth)

		if !errors.Is(err, serverErr) {
			t.Fatalf("got error %v, want server rejection %v", err, serverErr)
		}
		if response != nil {
			t.Errorf("got response %v, want nil on rejection", response)
		}
	})

	t.Run("successful OER without RPA", func(t *testing.T) {
		shelf := newShelf[driverCommon.MessageType]()
		streamer := &mockStreamer{
			pullMsgs: []driverCommon.Message[driverCommon.MessageType]{&mockOer{}},
			pullErr:  io.EOF,
		}

		response, err := readCookieAuthenticationResponse(context.Background(), shelf, streamer, oauth)

		sqlErr, ok := err.(oracleErrors.SQLError)
		if !ok || sqlErr.ErrorCode() != string(oracleErrors.InternalError) {
			t.Fatalf("got error %v, want InternalError for missing RPA", err)
		}
		if response != nil {
			t.Errorf("got response %v, want nil without RPA", response)
		}
	})

	t.Run("response read fails", func(t *testing.T) {
		shelf := newShelf[driverCommon.MessageType]()
		streamer := &mockStreamer{pullErr: io.EOF}

		response, err := readCookieAuthenticationResponse(context.Background(), shelf, streamer, oauth)

		if !errors.Is(err, io.EOF) {
			t.Fatalf("got error %v, want underlying EOF", err)
		}
		sqlErr, ok := err.(oracleErrors.SQLError)
		if !ok || sqlErr.ErrorCode() != string(oracleErrors.AuthenticatorError) {
			t.Fatalf("got error %v, want AuthenticatorError", err)
		}
		if response != nil {
			t.Errorf("got response %v, want nil on read failure", response)
		}
	})
}
