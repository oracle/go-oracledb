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

package config

import (
	"log/slog"
	"reflect"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	"golang.org/x/text/language"
)

// TestValidateLoggingLevel checks logging level validators.
// expectations level as string properly parsed
func TestValidateLoggingLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		want    any
		wantErr bool
	}{
		{
			name:  "debug",
			value: "DEBUG",
			want:  slog.LevelDebug.String(),
		},
		{
			name:  "inFo",
			value: "INFO",
			want:  slog.LevelInfo.String(),
		},
		{
			name:  "wARn",
			value: "WARN",
			want:  slog.LevelWarn.String(),
		},
		{
			name:  "error",
			value: "ERROR",
			want:  slog.LevelError.String(),
		},
		{name: "fine", value: "fine", want: slog.Level(-8).String()},
		{name: "finest", value: "finest", want: slog.Level(-16).String()},
		{name: "mixed case finest", value: "FiNeSt", want: slog.Level(-16).String()},
		{
			name:    "invalid level",
			value:   "TRACE",
			wantErr: true,
		},
		{
			name:    "non-string value",
			value:   123,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := validateLoggingLevel(reflect.ValueOf(tt.value), "Logging.Level")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

// TestValidateBooleanValue checks boolean validators.
// expectations boolean and string values are properly parsed.
func TestValidateBooleanValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		want    any
		wantErr bool
	}{
		{name: "bool true", value: true, want: true},
		{name: "string true", value: " true ", want: true},
		{name: "string false", value: "FALSE", want: false},
		{name: "empty string", value: " ", want: false},
		{name: "invalid string", value: "yes", wantErr: true},
		{name: "non-boolean value", value: 1, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := validateBooleanValue(reflect.ValueOf(tt.value), "BooleanValue")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

// TestValidateLogonModeValue checks logon mode validators.
// expectations logon mode values are normalized.
func TestValidateLogonModeValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		want    any
		wantErr bool
	}{
		{name: "sysdba", value: " sysdba ", want: common.KpzLogonSysdba.String()},
		{name: "sysoper", value: "SYSOPER", want: common.KpzLogonSysoper.String()},
		{name: "empty string", value: "", want: ""},
		{name: "invalid string", value: "NORMAL", wantErr: true},
		{name: "non-string value", value: 1, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := validateLogonModeValue(reflect.ValueOf(tt.value), "Credentials.LogonMode")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

// TestValidateLanguage checks language validators.
// expectations language strings and tags are properly parsed.
func TestValidateLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		want    any
		wantErr bool
	}{
		{name: "string language", value: "fr", want: language.French},
		{name: "language tag", value: language.English, want: language.English},
		{name: "empty string", value: "", wantErr: true},
		{name: "invalid string", value: "en-@", wantErr: true},
		{name: "non-string value", value: 1, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := validateLanguage(reflect.ValueOf(tt.value), "Locale.ClientLanguage")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

// TestValidateZeroOrPositive checks zero-or-positive integer validators.
// expectations integer and string values are properly parsed.
func TestValidateZeroOrPositive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		want    any
		wantErr bool
	}{
		{name: "zero string", value: "0", want: int64(0)},
		{name: "positive string", value: " 42 ", want: int64(42)},
		{name: "positive int", value: 42, want: int64(42)},
		{name: "negative string", value: "-1", wantErr: true},
		{name: "invalid string", value: "not-int", wantErr: true},
		{name: "negative int", value: -1, wantErr: true},
		{name: "unsupported type", value: true, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := validateZeroOrPositive(reflect.ValueOf(tt.value), "ZeroOrPositive")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

// TestValidateDRCPPurity checks DRCP purity validators.
// expectations purity values are properly parsed.
func TestValidateDRCPPurity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		want    any
		wantErr bool
	}{
		{name: "self", value: " self ", want: common.DrcpPuritySelf},
		{name: "new", value: "NEW", want: common.DrcpPurityNew},
		{name: "empty string", value: "", want: ""},
		{name: "invalid string", value: "STATEMENT", wantErr: true},
		{name: "non-string value", value: 1, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := validateDRCPPurity(reflect.ValueOf(tt.value), "ConnectionProperties.Drcp.Purity")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

// TestValidateDRCPBoundary checks DRCP boundary validators.
// expectations boundary values are properly parsed.
func TestValidateDRCPBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		want    any
		wantErr bool
	}{
		{name: "statement", value: " statement ", want: common.DrcpBoundaryStatement},
		{name: "transaction", value: "TRANSACTION", want: common.DrcpBoundaryTransaction},
		{name: "empty string", value: "", want: ""},
		{name: "invalid string", value: "SELF", wantErr: true},
		{name: "non-string value", value: 1, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := validateDRCPBoundary(reflect.ValueOf(tt.value), "ConnectionProperties.Drcp.Boundary")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}
