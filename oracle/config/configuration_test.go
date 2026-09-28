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

import "testing"

// TestQueryStringToMapTrimsValues verifies that the public query-string helper
// trims whitespace around keys and values while preserving each pair.
func TestQueryStringToMapTrimsValues(t *testing.T) {
	got, err := QueryStringToMap(" key = value &other=two")
	if err != nil {
		t.Fatalf("QueryStringToMap returned error: %v", err)
	}
	if got["key"] != "value" || got["other"] != "two" {
		t.Fatalf("unexpected query map: %#v", got)
	}
}

// TestQueryStringToMapRejectsMissingValue verifies that malformed query
// parameters return an error instead of being silently accepted.
func TestQueryStringToMapRejectsMissingValue(t *testing.T) {
	if _, err := QueryStringToMap("key"); err == nil {
		t.Fatal("QueryStringToMap accepted a key without a value")
	}
}

// TestLoggingConfigPublicAPI verifies the documented default logging settings
// and confirms that the configuration implements a non-empty Stringer value.
func TestLoggingConfigPublicAPI(t *testing.T) {
	config := NewOracleLoggingConfig()
	if config.GetLevel() != "ERROR" {
		t.Fatalf("default logging level = %q, want ERROR", config.GetLevel())
	}
	if config.GetDestination() != "NULL" {
		t.Fatalf("default logging destination = %q, want NULL", config.GetDestination())
	}
	if config.GetIncludeSensitive() || config.GetTruncate() {
		t.Fatal("sensitive logging and truncation should be disabled by default")
	}
	if rendered := config.String(); rendered == "" {
		t.Fatal("logging configuration String() returned an empty value")
	}
}

// TestDriverConfigValidateRejectsNegativeTimeout verifies that the configured
// zero-or-positive validator rejects a negative Oracle Net connect timeout.
func TestDriverConfigValidateRejectsNegativeTimeout(t *testing.T) {
	config := NewOracleDriverConfig()
	config.ConnectionProperties.ConnectTimeout = -1
	if err := config.Validate(); err == nil {
		t.Fatal("Validate accepted a negative connection timeout")
	}
}
