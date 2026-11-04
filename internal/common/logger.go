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

// Package logging package to define common logging usage in the Oracle driver
package common

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

type LoggingConfig interface {
	AssignFromFlags() error
	AssignFromEnv() error
	GetDestination() string
	GetLevel() string
	GetIncludeSensitive() bool
	GetTruncate() bool
}

// OracleLogger Oracle driver logger type
type OracleLogger struct {
	slog.Logger
	sensitiveEnabled bool // do we allow sensitive information to be logged
}

// With return a sub logger for given attributes
// see Logger.With()
func (l *OracleLogger) With(args ...any) *OracleLogger {
	return &OracleLogger{
		Logger:           *l.Logger.With(args...),
		sensitiveEnabled: l.sensitiveEnabled,
	}
}

// custom logging levels
const (
	OlPacketDump = slog.Level(-32) // private level to log packet dumps.
	OlFinest     = slog.Level(-16)
	OlFine       = slog.Level(-8)
	OlDebug      = slog.LevelDebug
	OlInfo       = slog.LevelInfo
	OlWarning    = slog.LevelWarn
	OlError      = slog.LevelError
)

// Fine logs a message with OlFine level
// parameters:
//   - msg message to be written
//   - args attributes
func (l *OracleLogger) Fine(msg string, args ...any) {
	if !l.Enabled(context.Background(), OlFine) {
		return
	}
	// not goign back in the stack will alwasy display Fine() as source
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:])
	record := slog.NewRecord(time.Now(), OlFine, msg, pcs[0])
	record.Add(args...)

	_ = l.Handler().Handle(context.Background(), record)
}

// Finest logs a message with OlFinest level
// parameters:
//   - msg message to be written
//   - args attributes
func (l *OracleLogger) Finest(msg string, args ...any) {
	if !l.Enabled(context.Background(), OlFinest) {
		return
	}
	// not goign back in the stack will alwasy display Finest() as source
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // skip runtime.Callers, log, Fine/Finest
	record := slog.NewRecord(time.Now(), OlFinest, msg, pcs[0])
	record.Add(args...)

	_ = l.Handler().Handle(context.Background(), record)
}

// PacketDump dumps a packet to the logging handler
// parameters:
//   - packetBytes : the byte of the network packet
func (l *OracleLogger) PacketDump(packetBytes []byte) {
	l.LogAttrs(context.Background(),
		OlPacketDump, "PacketDump", slog.Any(packetDumpAttrKey, packetBytes))
}

// OdlT gets a tagged logger.
// argument :
//   - tag, the tag for the returned sub logger
//
// returns:
//   - a previously allocated sub looger or a new one if one is not already available
func OdlT(tag string) *OracleLogger {
	if len(tag) != 0 {
		return Odl.With("ID", tag)
	}
	return &Odl
}

const packetDumpAttrKey = "packet"

// logging handler to handle packet dumps
type packetDumpHandler struct {
	*filteredHandler
	writer io.Writer
}

// logging handler that is enabled only for a given set of levels
// This handler will discard messages that are not with allowed levels.
// whatever the current logger level.
type filteredHandler struct {
	backend slog.Handler
	levels  []slog.Level // list of level allowed
}

// newFilteredHandler creates a new filteredHandler
// arguments :
//
//	backend : the backend handler where to log messages
//	levels: level white list.
//
// returns:
//
//	a new handler
func newFilteredHandler(backend slog.Handler, levels ...slog.Level) *filteredHandler {
	return &filteredHandler{levels: levels, backend: backend}
}

// newPacketDumpHandler creates a new packetDumpHandler
// arguments :
//
//	out : the writer to write dumps to (using raw format)
//	next: the actual handler to be used
//
// returns:
//
//	a new handler
func newPacketDumpHandler(out io.Writer, next slog.Handler) *packetDumpHandler {
	return &packetDumpHandler{filteredHandler: newFilteredHandler(next, OlPacketDump), writer: out}
}

// Enabled see slog.Logger.Enabled()
func (h *filteredHandler) Enabled(_ context.Context, level slog.Level) bool {
	for _, l := range h.levels {
		if l == level {
			return true
		}
	}
	return false
}

// WithAttrs see slog.Logger.WithAttrs()
func (h *filteredHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &filteredHandler{backend: h.backend.WithAttrs(attrs), levels: h.levels}
}

// WithGroup see slog.Logger.WithGroup()
func (h *filteredHandler) WithGroup(name string) slog.Handler {
	return &filteredHandler{backend: h.backend.WithGroup(name), levels: h.levels}
}

// Handle see slog.Logger.Handle()
func (h *filteredHandler) Handle(ctx context.Context, record slog.Record) error {
	return h.backend.Handle(ctx, record)
}

// Handle see slog.Logger.Handle()
func (h *packetDumpHandler) Handle(ctx context.Context, record slog.Record) error {
	var packet []byte
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key != packetDumpAttrKey {
			return true
		}
		if value, ok := attr.Value.Any().([]byte); ok {
			packet = value
		}
		return false
	})
	if packet == nil {
		return h.backend.Handle(ctx, record)
	}
	return h.dump(ctx, record.Level, packet)
}

// WithAttrs see slog.Logger.WithAttrs()
func (h *packetDumpHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &packetDumpHandler{filteredHandler: newFilteredHandler(h.backend.WithAttrs(attrs)), writer: h.writer}
}

// WithGroup see slog.Logger.WithGroup()
func (h *packetDumpHandler) WithGroup(name string) slog.Handler {
	return &packetDumpHandler{filteredHandler: newFilteredHandler(h.backend.WithGroup(name)),
		writer: h.writer}
}

// dump dumps packet bytes to the underlying IO writer
func (h *packetDumpHandler) dump(ctx context.Context, level slog.Level, buf []byte) error {
	header := slog.NewRecord(time.Now(), level, "packet dump", 0)
	header.AddAttrs(slog.Int("Data Length", len(buf)))
	if err := h.backend.Handle(ctx, header); err != nil {
		return err
	}

	var line bytes.Buffer
	var lineL bytes.Buffer
	var final bytes.Buffer
	for i, b := range buf {
		hexByte := fmt.Sprintf("%02X", b)
		if line.Len() != 0 {
			line.WriteString(" ")
		}
		line.WriteString(hexByte)
		if b >= 33 && b <= 126 {
			// Printable ASCII range
			lineL.WriteString(fmt.Sprintf("%c", b))
		} else {
			// Non-printable, replace with dot
			lineL.WriteString(".")
		}

		if (i+1)%8 == 0 || i == len(buf)-1 {
			final.WriteString(fmt.Sprintf("%-8s %s\n", lineL.String(), line.String()))
			lineL.Reset()
			line.Reset()
		}
	}
	_, err := h.writer.Write(final.Bytes())
	if err != nil {
		return err
	}

	return nil
}

var Odl = OracleLogger{
	Logger:           *slog.New(slog.DiscardHandler),
	sensitiveEnabled: false,
}

// keep track if InitLoggingWithConfig has been called once.
var ready atomic.Bool
var currentLogCloser io.Closer

func InitLoggingWithConfig(config LoggingConfig) {
	if config == nil {
		if !ready.CompareAndSwap(false, true) {
			return
		}
		return
	}

	ready.Store(true)

	config.AssignFromFlags()
	config.AssignFromEnv()

	if currentLogCloser != nil {
		_ = currentLogCloser.Close()
		currentLogCloser = nil
	}

	Odl.Logger = *slog.New(slog.DiscardHandler)
	Odl.sensitiveEnabled = false

	if strings.EqualFold(config.GetDestination(), "NULL") {
		return
	}

	level := parseOracleLogLevel(config.GetLevel())

	var logOut io.Writer

	if strings.EqualFold(config.GetDestination(), "STDOUT") {
		logOut = os.Stdout
	} else if strings.EqualFold(config.GetDestination(), "STDERR") {
		logOut = os.Stderr
	} else {
		// assume a file
		var oFlags = os.O_CREATE | os.O_WRONLY
		if config.GetTruncate() {
			oFlags |= os.O_TRUNC
		} else {
			oFlags |= os.O_APPEND
		}
		var err error
		logOut, err = os.OpenFile(config.GetDestination(), oFlags, 0644)
		if err != nil {
			// nothing we can print then
			return
		}
		currentLogCloser = logOut.(io.Closer)
	}

	var handler = slog.NewTextHandler(logOut, &slog.HandlerOptions{
		AddSource: level <= slog.LevelDebug,
		Level:     level,
	})

	if config.GetIncludeSensitive() {
		Odl.sensitiveEnabled = true
	}

	v, p := os.LookupEnv("ORACLE_GO_DRIVER_DEBUG_PACKETS")
	if p == true && v == "true" && config.GetIncludeSensitive() {
		multiHandler := slog.NewMultiHandler(
			newFilteredHandler(handler, OlFinest, OlFine, OlDebug, OlInfo, OlWarning, OlError),
			newPacketDumpHandler(logOut, handler))
		Odl.Logger = *slog.New(multiHandler)
	} else {
		Odl.Logger = *slog.New(handler)
	}

}

func parseOracleLogLevel(value string) slog.Level {
	switch strings.ToUpper(value) {
	case "FINEST":
		return OlFinest
	case "FINE":
		return OlFine
	default:
		var level slog.Level
		_ = level.UnmarshalText([]byte(value))
		return level
	}
}
