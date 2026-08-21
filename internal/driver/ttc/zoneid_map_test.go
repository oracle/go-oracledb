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

import "testing"

func TestGetIDFromZone_Found(t *testing.T) {
	t.Parallel()
	// Select a zone that is present in the seeded map
	const zone = "America/Los_Angeles"
	id, ok := getIDFromZone(zone)
	if !ok {
		t.Fatalf("expected zone %q to be found", zone)
	}
	if id != 103 {
		t.Fatalf("expected id 103 for %q, got %d", zone, id)
	}
}

func TestGetIDFromZone_NotFound(t *testing.T) {
	t.Parallel()
	const zone = "Etc/NotAZone"
	if id, ok := getIDFromZone(zone); ok {
		t.Fatalf("expected zone %q to be not found, got id=%d", zone, id)
	}
}

func TestGetZoneFromID_Found(t *testing.T) {
	t.Parallel()
	// ID 103 is mapped to "America/Los_Angeles" in zoneIDFromName.
	// Reverse map keeps the first encountered name for an ID.
	const id = 103
	name, ok := getZoneFromID(id)
	if !ok {
		t.Fatalf("expected id %d to be found", id)
	}
	if name == "" {
		t.Fatalf("expected a non-empty zone name for id %d", id)
	}
}

func TestGetZoneFromID_NotFound(t *testing.T) {
	t.Parallel()
	const id = 999999
	if name, ok := getZoneFromID(id); ok {
		t.Fatalf("expected id %d to be not found, got name=%q", id, name)
	}
}

// Sanity check: ensure at least a few core zones exist in the forward map,
// which indirectly exercises the global map initialization path.
func TestZoneMaps_Sanity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		zone string
		want int
	}{
		{"UTC", 5121},
		{"GMT", 513},
		{"America/New_York", 100},
		{"Asia/Tokyo", 267},
		{"Europe/London", 369},
	}
	for _, tc := range cases {
		got, ok := getIDFromZone(tc.zone)
		if !ok {
			t.Fatalf("expected zone %q to be found", tc.zone)
		}
		if got != tc.want {
			t.Fatalf("zone %q expected id %d, got %d", tc.zone, tc.want, got)
		}
	}
}
