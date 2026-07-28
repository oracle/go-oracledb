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
	"reflect"
	"strings"
	"testing"
)

// TestProperties_ContainsKey checks ContainsKey API.
// Build a Properties with source
// Expectation:
//   - ContainsKey on key from source return true
//   - ContainsKey on missing key return false
func TestProperties_ContainsKey(t *testing.T) {
	t.Parallel()
	var tmplMap map[string]any
	tmplMap = map[string]any{
		"bacon": 3.22,
	}

	newMap := NewPropertiesWithSource[string](tmplMap)
	if !newMap.ContainsKey("bacon") {
		t.Errorf("ContainsKey() should have return true")
	}

	newMap.SetProperty("foo", "bar")

	if !newMap.ContainsKey("foo") {
		t.Errorf("ContainsKey() should have return true for foo")
	}

	if newMap.ContainsKey("nothing") {
		t.Errorf("ContainsKey() should have return false")
	}
}

// TestProperties_GetProperty checks GetProperty API.
// Build a Properties, set a mapping
// Expectation:
//   - GetProperty on the key return the value
//   - GetProperty on missing key return nil (default value for type 'any')
func TestProperties_GetProperty(t *testing.T) {
	t.Parallel()
	newMap := NewProperties[string]()

	newMap.SetProperty("pasta", "carbonara")

	if strings.Compare(newMap.GetProperty("pasta").(string), "carbonara") != 0 {
		t.Errorf("GetProperty returned unexpected value wanted [%v] got [%v]",
			"carbonara",
			newMap.GetProperty("pasta").(string))
	}
	if newMap.GetProperty("pizza") != nil {
		t.Errorf("GetProperty returned unexpected value wanted [%v] got [%v]",
			nil,
			newMap.GetProperty("pizza"))
	}

}

// TestProperties_GetProperty checks GetTrimmedString API.
// Build a Properties, set a mapping for string containing blanks and integer
// Expectation:
//   - GetTrimmedString on dirty string return of a trimmed version of it
//   - GetTrimmedString on integer mapping return an error
//   - GetTrimmedString on missing mapping return nil
func TestProperties_GetTrimmedString(t *testing.T) {
	t.Parallel()
	newMap := NewProperties[string]()
	newMap.SetProperty("dirty", " hello my friends\t\n")
	newMap.SetProperty("dirty int", 12)

	trimed, err := newMap.GetTrimmedString("dirty")
	if err != nil {
		t.Errorf("GetTrimmedString should not have return an error, returned [%v]", err)
	}
	if strings.Compare(trimed, "hello my friends") != 0 {
		t.Errorf("ContainsKey() should have return false")
	}
	_, err = newMap.GetTrimmedString("dirty int")
	if err == nil {
		t.Errorf("GetTrimmedString() should have return an error ")
	}
	_, err = newMap.GetTrimmedString("not there")
	if err == nil {
		t.Errorf("GetTrimmedString() should have return an error for missing mapping")
	}
}

// TestProperties_PutAll checks PutAll API.
// 1. Build an empty Properties.
// 2. Call putAll for some mappings.
// 3. Call putAll for the same mappings plus a new one.
// 4. Call putAll with an empty Properties.
// Expectation:
//   - Properties properly updated on step #2
//   - Properties properly updated with new values plus the new mapping on step #3
//   - Properties mapping remain unchanged #4
func TestProperties_PutAll(t *testing.T) {
	t.Parallel()

	var tmpl map[string]any
	tmpl = map[string]any{
		"Apple":  1.75,
		"Banana": 3.22,
		"Orange": 1.89,
	}
	tmplMap := NewPropertiesWithSource[string](tmpl)

	newMap := NewProperties[string]()
	newMap.PutAll(tmplMap)
	if strings.Compare(newMap.String(), "[Apple=1.75,Banana=3.22,Orange=1.89]") != 0 {
		t.Errorf("Pull all failed wanted [%v], got [%v]",
			"[Apple=1.75,Banana=3.22,Orange=1.89]",
			newMap.String())
	}

	var tmpl2 map[string]any
	tmpl2 = map[string]any{
		"Apple":  2.75,
		"Banana": 4.22,
		"Kiwi":   5.00,
	}
	tmplMap2 := NewPropertiesWithSource(tmpl2)

	newMap.PutAll(tmplMap2)

	if newMap.GetProperty("Apple") != 2.75 {
		t.Errorf("GetProperty(\"Apple\") failed wanted [%v], got [%v]", 2.75, newMap.GetProperty("Apple"))
	}
	if newMap.GetProperty("Banana") != 4.22 {
		t.Errorf("GetProperty(\"Banana\") failed wanted [%v], got [%v]", 4.22, newMap.GetProperty("Banana"))
	}
	if newMap.GetProperty("Orange") != 1.89 {
		t.Errorf("GetProperty(\"Orange\") failed wanted [%v], got [%v]", 1.89, newMap.GetProperty("Orange"))
	}
	if newMap.GetProperty("Kiwi") != 5.00 {
		t.Errorf("GetProperty(\"Kiwi\") failed wanted [%v], got [%v]", 5.00, newMap.GetProperty("Kiwi"))
	}

	tmplMap3 := NewProperties[string]()

	newMap.PutAll(tmplMap3)
	if newMap.GetProperty("Apple") != 2.75 {
		t.Errorf("GetProperty(\"Apple\") failed wanted [%v], got [%v]", 2.75, newMap.GetProperty("Apple"))
	}
	if newMap.GetProperty("Banana") != 4.22 {
		t.Errorf("GetProperty(\"Banana\") failed wanted [%v], got [%v]", 4.22, newMap.GetProperty("Banana"))
	}
	if newMap.GetProperty("Orange") != 1.89 {
		t.Errorf("GetProperty(\"Orange\") failed wanted [%v], got [%v]", 1.89, newMap.GetProperty("Orange"))
	}
	if newMap.GetProperty("Kiwi") != 5.00 {
		t.Errorf("GetProperty(\"Kiwi\") failed wanted [%v], got [%v]", 5.00, newMap.GetProperty("Kiwi"))
	}

}

// TestProperties_Reset checks Reset API.
//  1. build a Property from source
//  2. call SetProperty to update a mapping
//  3. call SetProperty for a new mapping
//  4. call Reset()
//
// / Expectation:
//   - Properties back to original mapping after step #4
func TestProperties_Reset(t *testing.T) {
	t.Parallel()
	var tmplMap map[string]any
	tmplMap = map[string]any{
		"eggs":    1.75,
		"bacon":   3.22,
		"sausage": 1.89,
	}

	newMap := NewPropertiesWithSource[string](tmplMap)

	newMap.SetProperty("bacon", 15)
	newMap.SetProperty("pizza", "calzone")

	if strings.Compare(newMap.String(), "[bacon=15,eggs=1.75,pizza=calzone,sausage=1.89]") != 0 {
		t.Errorf("map not initialised correctly wanted [%v], got [%v]",
			"[bacon=15,eggs=1.75,pasta=carbonara,pizza=calzone,sausage=1.89]", newMap.String())
	}

	newMap.Reset()

	if newMap.ContainsKey("pizza") {
		t.Errorf("mapping should have been dsicarded")
	}

	if strings.Compare(newMap.String(), "[bacon=3.22,eggs=1.75,sausage=1.89]") != 0 {
		t.Fatalf("map not initialised correctly wanted [%v], got [%v]",
			"[bacon=15,eggs=1.75,pasta=carbonara,pizza=calzone,sausage=1.89]", newMap.String())
	}
}

// TestProperties_Snapshot checks Snapshot API.
//  1. build a Property from source
//  2. call SetProperty to update a mapping
//  3. call SetProperty for a new mapping
//  4. call Snapshot()
//  5. call SetProperty to update a mapping
//  6. call SetProperty for a new mapping
//  7. call Reset()
//
// / Expectation:
//   - Properties back to original mapping after step #4
func TestProperties_Snapshot(t *testing.T) {
	t.Parallel()
	var tmplMap map[string]any
	tmplMap = map[string]any{
		"eggs":    1.75,
		"bacon":   3.22,
		"sausage": 1.89,
	}

	newMap := NewPropertiesWithSource[string](tmplMap)

	newMap.SetProperty("bacon", 15)
	newMap.SetProperty("pizza", "calzone")

	if strings.Compare(newMap.String(), "[bacon=15,eggs=1.75,pizza=calzone,sausage=1.89]") != 0 {
		t.Fatalf("map not initialised correctly wanted [%v], got [%v]",
			"[bacon=15,eggs=1.75,pizza=calzone,sausage=1.89]", newMap.String())
	}

	newMap.Snapshot()

	newMap.SetProperty("bacon", 39)
	newMap.SetProperty("pizza", "regina")

	if got := newMap.GetProperty("bacon"); !reflect.DeepEqual(got, 39) {
		t.Errorf("GetProperty() return wrong value  have [%v], got [%v]", 39, got)
	}
	if got := newMap.GetProperty("pizza"); !reflect.DeepEqual(got, "regina") {
		t.Errorf("GetProperty() return wrong value  have [%v], got [%v]", "regina", got)
	}

	newMap.Reset()

	if got := newMap.GetProperty("bacon"); !reflect.DeepEqual(got, 15) {
		t.Errorf("GetProperty() return wrong value  have [%v], got [%v]", 15, got)
	}
	if got := newMap.GetProperty("eggs"); !reflect.DeepEqual(got, 1.75) {
		t.Errorf("GetProperty() return wrong value  have [%v], got [%v]", 1.75, got)
	}
	if got := newMap.GetProperty("sausage"); !reflect.DeepEqual(got, 1.89) {
		t.Errorf("GetProperty() return wrong value  have [%v], got [%v]", 1.89, got)
	}
	if got := newMap.GetProperty("pizza"); !reflect.DeepEqual(got, "calzone") {
		t.Errorf("GetProperty() return wrong value  have [%v], got [%v]", "calzone", got)
	}

}

// TestProperties_SetProperty checks SetProperty API
// Expectation: the value passed to SetProperty overwrite the value set at properties initialization
func TestProperties_SetProperty(t *testing.T) {
	t.Parallel()
	var tmplMap map[string]any
	tmplMap = map[string]any{
		"eggs":    1.75,
		"bacon":   3.22,
		"sausage": 1.89,
	}

	newMap := NewPropertiesWithSource[string](tmplMap)
	newMap.SetProperty("bacon", 12)
	if got := newMap.GetProperty("bacon"); !reflect.DeepEqual(got, 12) {
		t.Errorf("NewPropertiesWithSource() should have [%v], got [%v]", 12, got)
	}
}

// TestProperties_String checks String() implementations
// Expectation: returned string match expected format
func TestProperties_String(t *testing.T) {
	t.Parallel()
	type fields struct {
		_source  map[string]any
		_updated map[string]any
	}
	var p1 map[string]any
	p1 = map[string]any{"A1": "A1_val", "A3": "A3_val"}
	var p2 map[string]any
	p2 = map[string]any{"A2": "A2_val", "A3": "A3_val2"}

	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "String_empty_source",
			fields: fields{
				_updated: p2,
			},
			want: "[A2=A2_val,A3=A3_val2]",
		},
		{
			name: "String_empty_updatedd",
			fields: fields{
				_source: p1,
			},
			want: "[A1=A1_val,A3=A3_val]",
		},
		{
			name: "String_updated",
			fields: fields{
				_source:  p1,
				_updated: p2,
			},
			want: "[A1=A1_val,A2=A2_val,A3=A3_val2]",
		},
		{
			name:   "String_empty",
			fields: fields{},
			want:   "[]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Properties[string]{
				_source:  tt.fields._source,
				_updated: tt.fields._updated,
			}
			if got := p.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNewPropertiesWithSource verifies new Properties creation with template.
// Expectation:
//   - Successfully create the properties
//   - can get back a value by key form template
func TestNewPropertiesWithSource(t *testing.T) {
	t.Parallel()
	var tmplMap map[string]any
	tmplMap = map[string]any{
		"eggs":    1.75,
		"bacon":   3.22,
		"sausage": 1.89,
	}

	newMap := NewPropertiesWithSource[string](tmplMap)

	if got := newMap.GetProperty("bacon"); !reflect.DeepEqual(got, 3.22) {
		t.Errorf("NewPropertiesWithSource() should have [%v], got [%v]", 3.22, got)
	}
}

// TestNewProperties verifies new Properties creation.
// Expectation: Successfully create the properties
func TestNewProperties(t *testing.T) {
	t.Parallel()

	newMap := NewProperties[string]()
	if newMap.ContainsKey("not there") {
		t.Errorf("NewProperties() should not any key")
	}
}
