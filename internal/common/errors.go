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

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

var messageLanguages = []language.Tag{
	language.English, // en: First language will be the default if match fails
	language.French,  // fr
}

var defaultPrinter = message.NewPrinter(language.English)

// localizationService is the concrete implementation of LocalizationService.
type localizationService struct {
	printer *message.Printer
}

// LocalizationService is a sealed interface that formats Oracle errors and attaches
// localized printers to errors created or wrapped by the driver.
type LocalizationService interface {
	// format renders a localized message for the given error code and arguments.
	Format(code oracleErrors.ErrorCode, args ...any) string
	// LocalizeError attaches the service printer to an existing error chain.
	LocalizeError(err error) error
}

// NewLocalizationService returns a localization service for the provided user
// language tag. When the tag does not match a supported language, English is
// used as the fallback.
func NewLocalizationService(userLanguage language.Tag) LocalizationService {
	matcher := language.NewMatcher(messageLanguages)
	matchedLanguage, _, confidence := matcher.Match(userLanguage)
	Odl.Debug(fmt.Sprintf("Matched language %s with confidence %v", matchedLanguage, confidence))
	return &localizationService{
		printer: message.NewPrinter(matchedLanguage),
	}
}

// format renders a localized message for the given error code and arguments.
func (ms *localizationService) Format(code oracleErrors.ErrorCode, args ...any) string {
	return ms.printer.Sprintf(string(code), args...)
}

// LocalizeError attaches the service printer to an existing error chain.
func (ms *localizationService) LocalizeError(err error) error {
	deepSetLocalizationService(err, ms)
	return err
}

// deepSetLocalizationService walks an error chain and attaches the provided
// localization service to each OracleError it finds.
func deepSetLocalizationService(err error, ms *localizationService) {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if oErr, ok := e.(OracleError); ok {
			oErr.setLocalizationService(ms)
		}
	}
}

type OracleError interface {
	// setLocalizationService stores the printer used to render the error text.
	setLocalizationService(LocalizationService)
}

// oracleError is the concrete SQLError implementation used for driver-side
// failures.
type oracleError struct {
	code                oracleErrors.ErrorCode
	cause               error
	args                []any
	localizationService LocalizationService
}

// setLocalizationService stores the printer used to render the error text.
func (e *oracleError) setLocalizationService(localizationService LocalizationService) {
	e.localizationService = localizationService
}

// NewOracleError returns a new instance of the error with the given error code, cause and arguments.
//
// Parameters:
//   - code: the error code
//   - cause: the error that caused this error or nil if there is no cause
//   - args: the arguments to add to the error code
//
// Returns: a new instance of OracleError
func NewOracleError(code oracleErrors.ErrorCode, cause error, args ...any) oracleErrors.SQLError {
	return &oracleError{
		code:  code,
		cause: cause,
		args:  args,
	}
}

// ErrorCode returns the error code
func (e *oracleError) ErrorCode() string {
	return string(e.code)
}

// Error returns an error message prefixed with error code.
func (e *oracleError) Error() string {
	msg := defaultPrinter.Sprintf(string(e.code), e.args...)
	if e.localizationService != nil {
		msg = e.localizationService.Format(e.code, e.args...)
	}
	if len(msg) != 0 && msg != string(e.code) {
		if e.cause != nil {
			return fmt.Sprintf("%s - %s: %v", e.code, msg, e.cause.Error())
		}
		return fmt.Sprintf("%s - %s", e.code, msg)
	}
	return fmt.Sprintf("%s - Unknown error code, message not found", string(e.code))
}

// Unwrap unwraps the cause of this error
func (e *oracleError) Unwrap() error {
	return e.cause
}

// oerMessageError this struct should only be used by errors returned by the
// database on the OER message
type oerMessageError struct {
	code    string
	message string
}

// NewOERMessageError create a new oerOracleError. This function should only be
// used for errors returned by the database
func NewOERMessageError(code string, message string) oracleErrors.SQLError {
	return &oerMessageError{
		code:    code,
		message: message,
	}
}

// ErrorCode returns the error code
func (e *oerMessageError) ErrorCode() string {
	return e.code
}

// Error returns an error message prefixed with error code.
func (e *oerMessageError) Error() string {
	return e.message
}

// Unwrap Database errors do not wrap any other error, returns nil
func (e *oerMessageError) Unwrap() error {
	return nil
}

type CtxTimeoutCauseError interface {
	error
	GetSource() string
	GetValue() uint
	GetEmitterID() string
}
type _ctxTimeoutCauseError struct {
	source         string
	timeoutValueMS uint
	connectionID   string
}

func NewCtxTimeoutCauseError(timeoutSource string, timeoutValue uint, id string) CtxTimeoutCauseError {
	return &_ctxTimeoutCauseError{
		source:         timeoutSource,
		timeoutValueMS: timeoutValue,
		connectionID:   id,
	}
}

func (e _ctxTimeoutCauseError) GetEmitterID() string {
	return e.connectionID
}

func (e _ctxTimeoutCauseError) GetSource() string {
	return e.source
}
func (e _ctxTimeoutCauseError) GetValue() uint {
	return e.timeoutValueMS
}
func (e _ctxTimeoutCauseError) Error() string {
	return fmt.Sprintf("timeout of %d set by %s", e.timeoutValueMS, e.source)
}
