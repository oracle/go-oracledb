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
	"errors"
	"fmt"
	"testing"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"golang.org/x/text/language"
)

// tests behavior in case of unknown error.
func TestErrorUnknownCode(t *testing.T) {
	t.Parallel()
	ms := NewLocalizationService(language.AmericanEnglish)
	err := ms.LocalizeError(NewOracleError("ORA-01234", nil, nil)).(oracleErrors.SQLError)
	expected := fmt.Sprintf("%s - %s", "ORA-01234", "Unknown error code, message not found")
	if err.Error() != expected {
		t.Errorf("unexpected error message [%s], should be [%s]", err.Error(), expected)
	}
	if err.ErrorCode() != "ORA-01234" {
		t.Errorf("unexpected error code %s, expected ORA-01234", err.ErrorCode())
	}
}

// tests basic 3113 error message format
func TestErrorBasic3113(t *testing.T) {
	t.Parallel()
	ms := NewLocalizationService(language.AmericanEnglish)
	err := ms.LocalizeError(NewOracleError(oracleErrors.ConnectionLost, nil, nil)).(oracleErrors.SQLError)
	expected := fmt.Sprintf("%s - %s", oracleErrors.ConnectionLost, "database connection closed by peer")
	if err.Error() != expected {
		t.Errorf("unexpected error message [%s], should be [%s]", err.Error(), expected)
	}
}

// tests basic 3113 error message format using Canadian French language
func TestErrorBasic3113Fr(t *testing.T) {
	t.Parallel()
	ms := NewLocalizationService(language.CanadianFrench)
	err := ms.LocalizeError(NewOracleError(oracleErrors.ConnectionLost, nil, nil)).(oracleErrors.SQLError)
	expected := fmt.Sprintf("%s - %s", oracleErrors.ConnectionLost, "connexion de base de donn\u00E9es ferm\u00E9e par l'homologue")
	if err.Error() != expected {
		t.Errorf("unexpected error message [%s], should be [%s]", err.Error(), expected)
	}
}

// tests message of 3113 error with a cause
func TestError3113(t *testing.T) {
	t.Parallel()
	c := errors.New("my db is gone")
	ms := NewLocalizationService(language.English)
	err := ms.LocalizeError(NewOracleError(oracleErrors.ConnectionLost, c, nil)).(oracleErrors.SQLError)
	expected := fmt.Sprintf("%s - %s: %s", oracleErrors.ConnectionLost, "database connection closed by peer",
		c.Error())
	if err.Error() != expected {
		t.Errorf("unexpected error message [%s], should be [%s]", err.Error(), expected)
	}
}

// tests message of 3113 error with default and a cause
func TestError3113default(t *testing.T) {
	t.Parallel()
	c := errors.New("my db is gone")
	err := NewOracleError(oracleErrors.ConnectionLost, c, nil)
	expected := fmt.Sprintf("%s - %s: %s", oracleErrors.ConnectionLost, "database connection closed by peer",
		c.Error())
	if err.Error() != expected {
		t.Errorf("unexpected error message [%s], should be [%s]", err.Error(), expected)
	}
}

// tests message of 3113 error with a cause in French
func TestError3113Fr(t *testing.T) {
	t.Parallel()
	c := errors.New("my db is gone")
	ms := NewLocalizationService(language.French)
	err := ms.LocalizeError(NewOracleError(oracleErrors.ConnectionLost, c, nil)).(oracleErrors.SQLError)
	expected := fmt.Sprintf("%s - %s: %s", oracleErrors.ConnectionLost, "connexion de base de donn\u00E9es ferm\u00E9e par l'homologue",
		c.Error())
	if err.Error() != expected {
		t.Errorf("unexpected error message [%s], should be [%s]", err.Error(), expected)
	}
}

// tests message of 3113 error with missing language, fallback to English
func TestError3113pt(t *testing.T) {
	t.Parallel()
	c := errors.New("my db is gone")
	ms := NewLocalizationService(language.EuropeanPortuguese)
	err := ms.LocalizeError(NewOracleError(oracleErrors.ConnectionLost, c, nil)).(oracleErrors.SQLError)
	expected := fmt.Sprintf("%s - %s: %s", oracleErrors.ConnectionLost, "database connection closed by peer",
		c.Error())
	if err.Error() != expected {
		t.Errorf("unexpected error message [%s], should be [%s]", err.Error(), expected)
	}
}

// tests message of 3113 error with a cause with no language, fallback to English
func TestError3113NoLanguage(t *testing.T) {
	t.Parallel()
	c := errors.New("my db is gone")
	ms := NewLocalizationService(language.Tag{})
	err := ms.LocalizeError(NewOracleError(oracleErrors.ConnectionLost, c, nil)).(oracleErrors.SQLError)
	expected := fmt.Sprintf("%s - %s: %s", oracleErrors.ConnectionLost, "database connection closed by peer",
		c.Error())
	if err.Error() != expected {
		t.Errorf("unexpected error message [%s], should be [%s]", err.Error(), expected)
	}
}

// Tests unwrap of error with cause
func TestErrorUnwrap(t *testing.T) {
	t.Parallel()
	c := errors.New("my db is gone")
	ms := NewLocalizationService(language.Tag{})
	err := ms.LocalizeError(NewOracleError(oracleErrors.ConnectionLost, c, nil)).(oracleErrors.SQLError)
	expected := fmt.Sprintf("%s - %s: %s", oracleErrors.ConnectionLost, "database connection closed by peer",
		c.Error())
	if err.Error() != expected {
		t.Errorf("unexpected error message [%s], should be [%s]", err.Error(), expected)
	}
	unwraped := errors.Unwrap(err)
	if unwraped != c {
		t.Errorf("Unwrapped error should be cause expected %v, but was %v", c, unwraped)
	}
	if !errors.Is(c, unwraped) {
		t.Error("errors.Is should return true")
	}
}

func TestError3113InvalidLanguage(t *testing.T) {
	t.Parallel()
	c := errors.New("my db is gone")
	lang := language.Make("123")
	ms := NewLocalizationService(lang)
	err := ms.LocalizeError(NewOracleError(oracleErrors.ConnectionLost, c, nil)).(oracleErrors.SQLError)
	expected := fmt.Sprintf("%s - %s: %s", oracleErrors.ConnectionLost, "database connection closed by peer",
		c.Error())
	if err.Error() != expected {
		t.Errorf("unexpected error message [%s], should be [%s]", err.Error(), expected)
	}
}

// TestNewOERMessageError tests that errors created using NewOERMessageError
// implement the oracleErrors.SQLError interface and return then correct values for each
// function in the interface
func TestNewOERMessageError(t *testing.T) {
	t.Parallel()
	var newError error
	newError = NewOERMessageError(fmt.Sprintf("ORA-%05d", 3113), "error message goes here")
	if newError.Error() != "error message goes here" {
		t.Fatalf("err - wrong error message expected [error message goes here], but was [%s]", newError.Error())
	}
	err, ok := newError.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("oerMessageError should implement oracleErrors.SQLError")
	}
	if err.Error() != "error message goes here" {
		t.Fatalf("oracleErrors.SQLError - wrong error message expected [error message goes here], but was [%s]", err.Error())
	}
	if err.ErrorCode() != "ORA-03113" {
		t.Fatalf("oracleErrors.SQLError - error code expected [ORA-03113], but was [%s]", err.ErrorCode())
	}
	if errors.Unwrap(err) != nil {
		t.Fatal("Unwrap - should return nil, no cause")
	}
}
