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
	"crypto/rand"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// splitOnSpace keeps quoted segments together and strips the wrapping quotes.
func splitOnSpace(input string) []string {
	var (
		tokens    []string
		current   strings.Builder
		inQuote   bool
		quoteChar rune
	)

	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
	}

	for _, r := range input {
		switch {
		case unicode.IsSpace(r) && !inQuote:
			flush()
		case r == '"' || r == '\'':
			if inQuote && r == quoteChar {
				inQuote = false
			} else if !inQuote {
				inQuote = true
				quoteChar = r
			} else {
				current.WriteRune(r)
			}
		default:
			current.WriteRune(r)
		}
	}
	flush()

	return tokens
}

// StripSpacesOutsideQuotes removes ASCII spaces that appear outside of double-quoted
// strings. Spaces inside quotes are preserved.
//
// Example:
//
//	input:  host = "my host"  :1521
//	output: host="my host":1521
func StripSpacesOutsideQuotes(s string) string {
	var result strings.Builder
	inQuotes := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '"' {
			inQuotes = !inQuotes
			result.WriteByte(ch)
			continue
		}
		if ch == ' ' && !inQuotes {
			// skip spaces outside quotes
			continue
		}
		result.WriteByte(ch)
	}
	return result.String()
}

// StringToB1Array converts a string into a B1 array.
// The given string is taken as unicode string. The string
// is then converted to UTF8 as B1 array.
// Returns the B1 array result
func StringToB1Array(key string) B1Array {
	runes := []rune(key)
	// UTF-8 needs up to four bytes for each rune, including non-BMP characters.
	maxNbBytes := len(runes) * utf8.UTFMax
	bytearr := make(B1Array, maxNbBytes)
	byteLen := _runesToAL32UTF8(runes, bytearr)
	rbytearr := make(B1Array, byteLen)
	copy(rbytearr, bytearr[:byteLen])
	return rbytearr
}

// _runesToAL32UTF8 converts rune (unicode) array to UTF8
// parameters:
//   - runes : the runes to be encoded
//   - out : the byte array to write the convertion to.
//
// returns:
//
//	the length of the converted value.
func _runesToAL32UTF8(runes []rune, out []byte) int {
	n := 0
	for _, r := range runes {
		size := utf8.EncodeRune(out[n:], r)
		n += size
	}
	return n
}

// B1ArrayToString converts a B1 array to string
// parameters: bytes the array to be converted
// returns: the converted string. if the given array is nil or zero length, an empty string is returned.
func B1ArrayToString(bytes B1Array) string {

	if bytes == nil || len(bytes) == 0 {
		return ""
	}
	runes := _decodeUTF8WithReplacement(bytes)

	return string(runes)
}

// _decodeUTF8WithReplacement decodes UTF8 characters to unicode.
// Any character that cannot be converted is replaced by '�'
// parameters: the UTF8 character array
// returns: the converted array
func _decodeUTF8WithReplacement(b []byte) []rune {
	var runes = make([]rune, len(b)*utf8.UTFMax)
	var idx = 0
	var stop = len(b)
	var zeroCount = 0
	for idx < stop {
		r, size := utf8.DecodeRune(b[idx:])
		if r == utf8.RuneError && size == 1 {
			runes[idx] = '�' // Unicode replacement character
		} else {
			if r != 0 { // skip the null character
				runes[idx] = r
			} else {
				zeroCount++
			}
		}
		idx += size
	}
	return runes[:idx-zeroCount]
}

// ldiMaxTimeField Added to ensure array values are positive
const ldiMaxTimeField int = 60

var _tzBytes []byte = nil

// GetTimeZoneBytes returns the timezone bytes for the current location.
func GetTimeZoneBytes() []byte {
	if _tzBytes != nil {
		return _tzBytes
	}
	loc := time.Now().Location()
	now := time.Now().In(loc)

	_, offsetSeconds := now.Zone()
	hr := offsetSeconds / 3600
	mi := (offsetSeconds / 60) % 60

	_tzBytes := []byte{
		0x80, 0x00, 0x00, 0x00,
		byte((hr + ldiMaxTimeField) & 0xFF),
		byte((mi + ldiMaxTimeField) & 0xFF),
		0x80, 0x00, 0x00, 0x00, 0x00,
	}

	return _tzBytes
}

// GenerateRandomBytes fills the provided byte slice with cryptographically secure random bytes.
func GenerateRandomBytes(bytes []byte) error {
	_, err := rand.Read(bytes)
	return err
}

// NibbleToHex nibble To hex.
// This will take a nibble (4 low bits of the byte passed) and convert it into
// the Hex value. If the value of nibble is greater than 4 bits, the
// 4 high bits will be filtered.
func NibbleToHex(nibble byte) byte {
	nibble = nibble & byte(0x0F) // Filtering High byte

	if nibble < 10 {
		return nibble + '0'
	}

	return byte(nibble - 10 + 'A')
}

// BArray2Nibbles Byte array to nibbles.
// Takes an array of bytes and converts it into an array of nibbles. The
// nibbles array must be at least double the size of array.
func BArray2Nibbles(array []byte, nibbles []byte) int {
	var i int
	for i = 0; i < len(array); i++ {
		nibbles[i*2] = NibbleToHex((array[i] & 0xF0) >> 4)
		nibbles[(i*2)+1] = NibbleToHex(array[i] & 0x0F)
	}
	return (i * 2)
}

// ToBinArray converts a hex string to a byte array.
func ToBinArray(hexStr string) []byte {
	bArray := make([]byte, len(hexStr)/2)
	for i := 0; i < (len(hexStr) / 2); i++ {
		nibble, err := strconv.ParseUint(hexStr[2*i:2*i+1], 16, 8)
		if err != nil {
		}

		firstNibble := byte(nibble) // [x,y)
		nibble, err = strconv.ParseUint(hexStr[2*i+1:2*i+2], 16, 8)
		if err != nil {
		}

		secondNibble := byte(nibble)
		finalByte := (secondNibble) | (firstNibble << 4)
		bArray[i] = finalByte
	}

	return bArray
}
