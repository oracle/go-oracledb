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
	"io"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
)

type LoggingConfig interface {
	AssignFromFlags() error
	AssignFromEnv() error
	GetDestination() string
	GetLevel() string
	GetIncludeSensitive() bool
	GetTruncate() bool
}

// Odl Common reference to driver logger
var Odl = slog.New(slog.DiscardHandler)

// Osl Common reference to driver sensitive information logger
var Osl = slog.New(slog.DiscardHandler)

// Opl logger used for packet dump
var Opl = slog.New(slog.DiscardHandler)

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

	Odl = slog.New(slog.DiscardHandler)
	Osl = Odl
	Opl = Odl

	if strings.EqualFold(config.GetDestination(), "NULL") {
		return
	}

	var level slog.Level
	level.UnmarshalText([]byte(config.GetLevel()))

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
		AddSource: level == slog.LevelDebug,
		Level:     level,
	})

	Odl = slog.New(handler)

	if config.GetIncludeSensitive() {
		Osl = Odl
	}

	v, p := os.LookupEnv("ORACLE_GO_DRIVER_DEBUG_PACKETS")
	if p == true && v == "true" && config.GetIncludeSensitive() {
		Opl = slog.New(handler)
	}

}
