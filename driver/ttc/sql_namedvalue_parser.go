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
	"database/sql/driver"
	"log/slog"
	"math"
	"unicode"

	"github.com/oracle/go-driver/driver/common"
)

// bindDetails holds metadata about SQL bind placeholders extracted by parsePlaceholders.
//
// Fields:
//   - bindNames: placeholders in order of appearance; repeated for each occurrence.
//     The index in this slice is the zero-based occurrence position.
//   - bindMap: maps each unique placeholder name to all of its zero-based occurrence
//     indices in bindNames; used to fill all occurrences in one step.
//
// Usage:
// extractInputBindValues consumes bindDetails to align driver.NamedValue inputs (by
// Name or by Ordinal resolved to a name) to the positional slice expected by the
// wire protocol, ensuring all occurrences are filled and reporting duplicates or
// missing arguments.
type bindDetails struct {
	bindNames   []string
	bindMap     map[string][]uint16
	uniqueNames map[string]uint16
}

const maxBindIndex = math.MaxUint16

// parsePlaceholders scans the SQL text and returns a slice of placeholder
// identifiers (without the leading colon) in order of appearance.
//
// Rules:
//   - Any ':' followed by a letter or a digit starts a placeholder.
//   - Identifiers started by a letter continue with letters or digits.
//   - Numeric identifiers continue with digits (e.g., :1, :23).
//   - Text inside comments (-- ... , /* ... */) and quoted strings ('...' or "...")
//     is ignored.
//   - Constructs like ':=' are ignored as non-placeholders.
//   - A dangling ':' at the end of the string is an error.
func parsePlaceholders(sql string) (*bindDetails, error) {
	var binds []string
	uniqueNames := make(map[string]uint16)
	occ := make(map[string][]uint16)
	var occIndex uint16 = 0
	runes := []rune(sql)
	n := len(runes)

	for i := 0; i < n; i++ {
		// Skip over comments or quoted strings starting at position i.
		if newI, skipped := skipCommentOrString(runes, i); skipped {
			// Adjust for the loop's i++ increment.
			i = newI - 1
			continue
		}

		ch := runes[i]

		// Detect placeholder
		if ch == ':' {
			start := i + 1
			if start >= n {
				common.Odl.Error("parsePlaceholders: dangling colon at end of query")
				return nil, common.NewOracleError(common.StatementParsingDanglingColon, nil)
			}
			// Not a valid placeholder (e.g., ':=' used in Pl/SQL)
			if runes[start] == '=' {
				continue
			}

			j := start
			// the namedValue "name" placeholder starts with a letter/number then can be followed
			// by another letter, a digit, an underscore. eg. "nn", "n1" "n_n"
			for j < n &&
				(unicode.IsLetter(runes[j]) ||
					unicode.IsDigit(runes[j]) ||
					runes[j] == '_' ||
					runes[j] == '$' ||
					runes[j] == '#' ||
					runes[j] == '@') {
				j++
			}
			name := string(runes[start:j])
			binds = append(binds, name)
			if _, found := occ[name]; !found {
				if len(uniqueNames) > maxBindIndex {
					return nil, common.NewOracleError(common.StatementParsingInvalidArgCount, nil, len(uniqueNames)+1, maxBindIndex+1)
				}
				uniqueNames[name] = uint16(len(uniqueNames))
			}
			if len(binds) > maxBindIndex+1 {
				return nil, common.NewOracleError(common.StatementParsingPlaceholdersArgsMismatch, nil, len(binds), maxBindIndex+1)
			}
			occ[name] = append(occ[name], occIndex)
			occIndex++
			i = j - 1
		}
	}
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("parsePlaceholders: placeholders parsed:", "binds", binds, "bindMap", occ)
	}
	return &bindDetails{bindNames: binds, bindMap: occ, uniqueNames: uniqueNames}, nil
}

// skipCommentOrString checks if a comment or a quoted string starts at index i,
// and if so, returns the index just after the construct and true. Otherwise it
// returns the original index and false.
func skipCommentOrString(runes []rune, i int) (int, bool) {
	n := len(runes)
	if i < 0 || i >= n {
		return i, false
	}

	// Line comment: -- until newline or end
	if runes[i] == '-' && i+1 < n && runes[i+1] == '-' {
		j := i + 2
		for j < n && runes[j] != '\n' {
			j++
		}
		return j, true
	}

	// Block comment: /* ... */
	if runes[i] == '/' && i+1 < n && runes[i+1] == '*' {
		j := i + 2
		for j < n-1 {
			if runes[j] == '*' && runes[j+1] == '/' {
				j += 2
				return j, true
			}
			j++
		}
		// Unclosed block comment: skip to end
		return n, true
	}

	// Single-quoted string: ' ... ' with '' escapes
	if runes[i] == '\'' {
		j := i + 1
		for j < n {
			if runes[j] == '\'' {
				if j+1 < n && runes[j+1] == '\'' {
					j += 2
					continue
				}
				j++ // consume closing quote
				return j, true
			}
			j++
		}
		// Unclosed string: skip to end
		return n, true
	}

	// Double-quoted identifier: " ... " with "" escapes
	if runes[i] == '"' {
		j := i + 1
		for j < n {
			if runes[j] == '"' {
				if j+1 < n && runes[j+1] == '"' {
					j += 2
					continue
				}
				j++ // consume closing quote
				return j, true
			}
			j++
		}
		// Unclosed identifier: skip to end
		return n, true
	}

	return i, false
}

/*
extractInputBindValues aligns the supplied NamedValue slice to match the
placeholder occurrences discovered by parsePlaceholders.

New semantics:
  - Build a map from each placeholder name to all its occurrence indices.
  - Applying a value by Name fills all occurrences of that name.
  - Applying a value by Ordinal resolves Ordinal -> bindNames[ordinal-1] (i.e., the placeholder name
    at that position) and then behaves exactly like Name (fills all occurrences of that name).
  - If setting a value would overwrite a position that's already filled, return a duplicate error.
  - After processing all inputs, every placeholder position must be filled; otherwise return a
    missing-arg error.
  - Extra values that do not map to any placeholder name (by Name or by Ordinal resolution) cause an error.
*/
func extractInputBindValues(bindDetails *bindDetails, in []driver.NamedValue) ([]driver.Value, error) {

	// numberOfBinds: total placeholder occurrences in SQL (including repeated names)
	numberOfBinds := len(bindDetails.bindNames)

	// allPlaceholders holds the final positional values aligned with each placeholder occurrence
	allPlaceholders := make([]driver.Value, numberOfBinds)
	// filled marks which occurrence positions have been assigned a value
	filled := make([]bool, numberOfBinds)
	// total number of filledBindValues
	total := 0

	// place a value for all occurrences of a given key
	placeByKey := func(key string, src driver.NamedValue) error {
		indices, ok := bindDetails.bindMap[key]
		if !ok {
			// Provided name does not exist among placeholders
			common.Odl.Error("extractInputBindValues: provided name not in placeholders", "name", src.Name)
			return common.NewOracleError(common.StatementParsingNameNotFound, nil, src.Name)
		}
		for _, idx := range indices {
			if filled[idx] {
				// Attempt to overwrite an already filled placeholder occurrence
				common.Odl.Error("extractInputBindValues: duplicate value for placeholder", "placeholder", bindDetails.bindNames[idx], "position", idx+1)
				return common.NewOracleError(common.StatementParsingDuplicateArg, nil, bindDetails.bindNames[idx], int(idx)+1)
			}
			allPlaceholders[idx] = src.Value
			filled[idx] = true
			total++
		}
		return nil
	}

	// Process inputs in order
	for _, nv := range in {
		// If Name is provided, assign its value to all occurrences of that placeholder name.
		// Otherwise, resolve Ordinal to the placeholder name at that position and assign by name.
		if nv.Name != "" {
			if err := placeByKey(nv.Name, nv); err != nil {
				return nil, err
			}
			continue
		} else {
			// Resolve by Ordinal to its placeholder name, then place by that name
			if nv.Ordinal <= 0 || nv.Ordinal > numberOfBinds {
				common.Odl.Error("extractInputBindValues: argument without name or valid ordinal", "ordinal", nv.Ordinal)
				return nil, common.NewOracleError(common.StatementParsingInvalidOrdinal, nil, nv.Ordinal, numberOfBinds)
				//"Invalid ordinal provided %d, number of placeholder found %d"
			}
			if err := placeByKey(bindDetails.bindNames[nv.Ordinal-1], nv); err != nil {
				return nil, err
			}
		}
	}

	// Our parser is too simple so we may have found placeholders that are not
	// placeholders, therefore we only send the placeholders for which the user
	// has set values
	filledPlaceholders := make([]driver.Value, total)
	currentPosition := 0
	for i, isFilled := range filled {
		if isFilled {
			filledPlaceholders[currentPosition] = allPlaceholders[i]
			currentPosition++
		}
	}
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("extractInputBindValues: validated input NamedValues:", "out", filledPlaceholders)
	}
	return filledPlaceholders, nil
}

// extractInputBindValuesForPlSql special implementation for PL/SQL that aligns
// the supplied NamedValue slice to match the order of occurence if the
// placeholders in the PL/SQL statement.
func extractInputBindValuesForPlSql(bindDetails *bindDetails, in []driver.NamedValue) ([]driver.Value, error) {

	numberOfBinds := len(bindDetails.uniqueNames)
	// allPlaceholders holds the final positional values aligned with each placeholder occurrence
	placeholders := make([]driver.Value, numberOfBinds)
	filled := make([]bool, numberOfBinds)
	// total number of filledBindValues
	total := 0

	// Process inputs in order
	for _, nv := range in {
		// If Name is provided, assign its value to all occurrences of that placeholder name.
		// Otherwise, resolve Ordinal to the placeholder name at that position and assign by name.
		if nv.Name != "" {
			index, ok := bindDetails.uniqueNames[nv.Name]
			if !ok {
				common.Odl.Error("extractInputBindValuesForPlSql: provided name not in placeholders", "name", nv.Name)
				return nil, common.NewOracleError(common.StatementParsingNameNotFound, nil, nv.Name)
			}
			if filled[index] {
				common.Odl.Error("extractInputBindValuesForPlSql: duplicate value for placeholder", "placeholder", nv.Name, "position", index+1)
				return nil, common.NewOracleError(common.StatementParsingDuplicateArg, nil, nv.Name, int(index)+1)
			}
			placeholders[index] = nv.Value
			filled[index] = true
			total++
		} else {
			// Resolve by Ordinal to its placeholder name, then place by that name
			if nv.Ordinal <= 0 || nv.Ordinal > numberOfBinds {
				common.Odl.Error("extractInputBindValues: argument without name or valid ordinal", "ordinal", nv.Ordinal)
				return nil, common.NewOracleError(common.StatementParsingInvalidOrdinal, nil, nv.Ordinal, numberOfBinds)
				//"Invalid ordinal provided %d, number of placeholder found %d"
			}
			index := nv.Ordinal - 1
			if filled[index] {
				common.Odl.Error("extractInputBindValuesForPlSql: duplicate value for ordinal", "ordinal", nv.Ordinal)
				return nil, common.NewOracleError(common.StatementParsingDuplicateArg, nil, bindDetails.bindNames[index], nv.Ordinal)
			}
			placeholders[nv.Ordinal-1] = nv.Value
			filled[nv.Ordinal-1] = true
			total++
		}
	}

	// Our parser is too simple so we may have found placeholders that are not
	// placeholders, therefore we only send the placeholders for which the user
	// has set values
	filledPlaceholders := make([]driver.Value, total)
	currentPosition := 0
	for i, isFilled := range filled {
		if isFilled {
			filledPlaceholders[currentPosition] = placeholders[i]
			currentPosition++
		}
	}

	common.Odl.Debug("extractInputBindValues: validated input NamedValues:", "out", filledPlaceholders)
	return filledPlaceholders, nil
}
