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

package common

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type recordingSlogHandler struct {
	enabled bool
	records []slog.Record
}

func (h *recordingSlogHandler) Enabled(context.Context, slog.Level) bool {
	return h.enabled
}

func (h *recordingSlogHandler) Handle(_ context.Context, record slog.Record) error {
	h.records = append(h.records, record.Clone())
	return nil
}

func (h *recordingSlogHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h *recordingSlogHandler) WithGroup(string) slog.Handler {
	return h
}

// TestPacketDumpHandlerFormatsPacket verifies that packet dump records are
// forwarded to the wrapped handler while their byte payload is formatted as a
// hex and ASCII dump in the configured writer.
func TestPacketDumpHandlerFormatsPacket(t *testing.T) {
	t.Parallel()

	recorder := &recordingSlogHandler{enabled: true}
	var out bytes.Buffer
	handler := newPacketDumpHandler(&out, recorder)
	record := slog.NewRecord(time.Unix(0, 0), OlPacketDump, "packet dump", 0)
	record.AddAttrs(slog.Any(packetDumpAttrKey, []byte("ABCDEFGHI")))

	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("packet dump handler returned error: %v", err)
	}

	if got, want := len(recorder.records), 1; got != want {
		t.Fatalf("record count = %d, want %d", got, want)
	}
	if recorder.records[0].Message != "packet dump" {
		t.Fatalf("header message = %q, want packet dump", recorder.records[0].Message)
	}
	assertRecordAttr(t, recorder.records[0], "Data Length", int64(9))
	if got := out.String(); !strings.Contains(got, "ABCDEFGH") ||
		!strings.Contains(got, "41 42 43 44 45 46 47 48") {
		t.Fatalf("first dump line = %q", got)
	}
	if got := out.String(); !strings.Contains(got, "I") || !strings.Contains(got, "49") {
		t.Fatalf("second dump line = %q", got)
	}
}

func TestOracleLoggerWithReturnsLoggerWithAttrs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := &OracleLogger{
		Logger:           *slog.New(slog.NewTextHandler(&out, nil)),
		sensitiveEnabled: true,
	}

	child := logger.With("ID", "bench")
	child.Info("message")

	if child == logger {
		t.Fatal("With returned original logger")
	}
	if !child.sensitiveEnabled {
		t.Fatal("With did not preserve sensitive logging setting")
	}
	if got := out.String(); !strings.Contains(got, "msg=message") || !strings.Contains(got, "ID=bench") {
		t.Fatalf("log output = %q, want message with ID attribute", got)
	}
}

func TestOracleLoggerFineAndFinestLogCustomLevels(t *testing.T) {
	t.Parallel()

	recorder := &recordingSlogHandler{enabled: true}
	logger := &OracleLogger{Logger: *slog.New(recorder)}

	logger.Fine("fine message", "key", "fine")
	logger.Finest("finest message", "key", "finest")

	if got, want := len(recorder.records), 2; got != want {
		t.Fatalf("record count = %d, want %d", got, want)
	}

	tests := []struct {
		name    string
		record  slog.Record
		level   slog.Level
		message string
		attr    string
	}{
		{
			name:    "fine",
			record:  recorder.records[0],
			level:   OlFine,
			message: "fine message",
			attr:    "fine",
		},
		{
			name:    "finest",
			record:  recorder.records[1],
			level:   OlFinest,
			message: "finest message",
			attr:    "finest",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.record.Level != test.level {
				t.Fatalf("level = %v, want %v", test.record.Level, test.level)
			}
			if test.record.Message != test.message {
				t.Fatalf("message = %q, want %q", test.record.Message, test.message)
			}
			if test.record.PC == 0 {
				t.Fatal("record PC is not set")
			}
			assertRecordAttr(t, test.record, "key", test.attr)
		})
	}
}

func TestOdlTReturnsTaggedLogger(t *testing.T) {
	var out bytes.Buffer
	original := Odl
	Odl = OracleLogger{
		Logger:           *slog.New(slog.NewTextHandler(&out, nil)),
		sensitiveEnabled: true,
	}
	t.Cleanup(func() {
		Odl = original
	})

	if got := OdlT(""); got != &Odl {
		t.Fatal("OdlT with empty tag did not return global logger")
	}

	logger := OdlT("session-1")
	if logger == &Odl {
		t.Fatal("OdlT with tag returned global logger")
	}
	if !logger.sensitiveEnabled {
		t.Fatal("OdlT did not preserve sensitive logging setting")
	}

	logger.Info("message")
	if got := out.String(); !strings.Contains(got, "msg=message") ||
		!strings.Contains(got, "ID=session-1") {
		t.Fatalf("log output = %q, want message with ID attribute", got)
	}
}

func TestParseOracleLogLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  slog.Level
	}{
		{name: "finest", value: "FINEST", want: OlFinest},
		{name: "finest lower case", value: "finest", want: OlFinest},
		{name: "fine", value: "FINE", want: OlFine},
		{name: "fine lower case", value: "fine", want: OlFine},
		{name: "debug", value: "DEBUG", want: slog.LevelDebug},
		{name: "info", value: "INFO", want: slog.LevelInfo},
		{name: "warn", value: "WARN", want: slog.LevelWarn},
		{name: "error", value: "ERROR", want: slog.LevelError},
		{name: "bare numeric defaults to info", value: "-12", want: slog.LevelInfo},
		{name: "invalid defaults to info", value: "invalid", want: slog.LevelInfo},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := parseOracleLogLevel(test.value); got != test.want {
				t.Fatalf("parseOracleLogLevel(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

func assertRecordAttr(t *testing.T, record slog.Record, key string, want any) {
	t.Helper()

	found := false
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key != key {
			return true
		}
		found = true
		if got := attr.Value.Any(); got != want {
			t.Fatalf("attr %s = %v, want %v", key, got, want)
		}
		return false
	})
	if !found {
		t.Fatalf("missing attr %s", key)
	}
}
