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
	"regexp"
	"strings"
	"unicode/utf8"
)

var simpleIdentifierPattern = regexp.MustCompile(`^[\p{L}][\p{L}\p{N}_]*$`)

// EnquoteLiteral returns a string enclosed in single quotes. Any occurrence of a single
// quote within the string will be replaced by two single quotes.
//
// Parameters:
//   - val: Input text to quote.
//
// Returns:
//   - string: A string enclosed by single quotes with every single quote converted
//     to two single quotes.
func EnquoteLiteral(val string) string {
	return "'" + strings.ReplaceAll(val, "'", "''") + "'"
}

// EnquoteNCharLiteral returns a string enclosed in single quotes and prefixed with 'N'.
// Any occurrence of a single quote within the string will be replaced by two single quotes.
//
// Parameters:
//   - val: Input text to quote as an NCHAR literal.
//
// Returns:
//   - string: An N-prefixed string enclosed by single quotes with every single
//     quote converted to two single quotes.
func EnquoteNCharLiteral(val string) string {
	return "N'" + strings.ReplaceAll(val, "'", "''") + "'"
}

// IsSimpleIdentifier retrieves whether identifier is a simple SQL identifier.
// A simple SQL identifier must match the following criteria:
//   - The first character is a Unicode letter.
//   - The name only contains Unicode letters, Unicode digits, or underscores.
//   - The length of the identifier must be between 1 and 128 characters.
//
// Parameters:
//   - identifier: the SQL identifier text to validate.
//
// Returns:
//   - bool: true if the identifier is a simple identifier, otherwise false.
func IsSimpleIdentifier(identifier string) bool {
	length := utf8.RuneCountInString(identifier)
	if length < 1 || length > MaxIdentifierLength {
		return false
	}
	return simpleIdentifierPattern.MatchString(identifier)
}

// EnquoteIdentifier returns identifier as a delimited SQL identifier enclosed in
// double quotes.
//
// If identifier is a simple SQL identifier, the returned value is that identifier
// enclosed in double quotes.
//
// If identifier is not a simple SQL identifier, it is enclosed in double quotes
// unless it is already enclosed in double quotes. In that case the existing outer
// quotes are preserved and the inner text is validated.
//
// An error is returned when identifier is empty, longer than 128 characters, or when the
// delimited form would contain a null character or an embedded double quote.
//
// Examples:
//   - Hello -> "Hello"
//   - G'Day -> "G'Day"
//   - "Bruce Wayne" -> "Bruce Wayne"
//   - GoodDay$ -> "GoodDay$"
//   - Hello"World -> error
func EnquoteIdentifier(identifier string) (string, error) {
	length := utf8.RuneCountInString(identifier)
	if length < 1 || length > MaxIdentifierLength {
		return "", NewOracleError(InvalidIdentifier, nil)
	}
	if IsSimpleIdentifier(identifier) {
		return `"` + identifier + `"`, nil
	}

	unquoted := identifier
	if len(unquoted) >= 2 && strings.HasPrefix(unquoted, `"`) && strings.HasSuffix(unquoted, `"`) {
		unquoted = unquoted[1 : len(unquoted)-1]
	}

	if strings.ContainsRune(unquoted, '"') || strings.ContainsRune(unquoted, '\x00') {
		return "", NewOracleError(InvalidIdentifier, nil)
	}

	return `"` + unquoted + `"`, nil
}
