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

// Capability client/server capabilities
// used to maintain a map of capabilities enabled after negotiation
type Capability struct {
	Value byte
	IsSet bool
}

// Shelf as a global storage to share driver infrastructure
type Shelf[T any] struct {
	marshaller           Marshaller
	msgFactory           Factory
	msgStmr              Streamer[T]
	localizationService  LocalizationService
	capabilities         map[string]Capability
	connectionProperties *OracleDriverProperties // connectionProperties represents the connection properties set in dsn string
}

// NewShelf Creates a new Shelf
func NewShelf[T any]() *Shelf[T] {
	return &Shelf[T]{
		marshaller:           nil,
		msgFactory:           nil,
		msgStmr:              nil,
		localizationService:  nil,
		connectionProperties: nil,
	}
}

// RegisterMarshaller Registers a marshaler to the Shelf
// Returns the Shelf itself
// Existing instance os replaced by the one passed as parameter
func (s *Shelf[T]) RegisterMarshaller(mar Marshaller) *Shelf[T] {
	s.marshaller = mar
	return s
}

// RegisterMessageFactory Registers a message factory to the Shelf
// Returns the Shelf itself
// Existing instance os replaced by the one passed as parameter
func (s *Shelf[T]) RegisterMessageFactory(msgFactory Factory) *Shelf[T] {
	s.msgFactory = msgFactory
	return s
}

// RegisterMessageStreamer Registers a message streamer to the Shelf
// Returns the Shelf itself
// Existing instance os replaced by the one passed as parameter
func (s *Shelf[T]) RegisterMessageStreamer(msgStmr Streamer[T]) *Shelf[T] {
	s.msgStmr = msgStmr
	return s
}

// RegisterLocalizationService registers a localization service on the Shelf.
// Returns the Shelf itself.
// Existing instance is replaced by the one passed as parameter.
func (s *Shelf[T]) RegisterLocalizationService(localizationService LocalizationService) *Shelf[T] {
	s.localizationService = localizationService
	return s
}

// RegisterCapabilities Registers the negotiated capabilities
// Returns the Shelf itself
// Existing instance os replaced by the one passed as parameter
func (s *Shelf[T]) RegisterCapabilities(capabilities map[string]Capability) *Shelf[T] {
	s.capabilities = capabilities
	return s
}

// UpdateConnectionProperties adds the provided connection properties to the Shelf.
// Existing properties with the same keys will be overwritten by the new values.
func (s *Shelf[T]) UpdateConnectionProperties(props *OracleDriverProperties) *Shelf[T] {
	s.connectionProperties = props
	return s
}

// GetConnectionProperties retrieves the connection properties stored on the Shelf.
func (s *Shelf[T]) GetConnectionProperties() *OracleDriverProperties {
	return s.connectionProperties
}

// GetMessageStreamer Retrieves the streamer previously registered or nil
func (s *Shelf[T]) GetMessageStreamer() Streamer[T] {
	return s.msgStmr
}

// GetMarshaller Retrieves the marshaller previously registered or nil
func (s *Shelf[T]) GetMarshaller() Marshaller {
	return s.marshaller
}

// GetMessageFactory Retrieves the factory previously registered or nil
func (s *Shelf[T]) GetMessageFactory() Factory {
	return s.msgFactory
}

// GetLocalizationService retrieves the localization service previously registered or nil.
func (s *Shelf[T]) GetLocalizationService() LocalizationService {
	return s.localizationService
}

// LocalizeError localizes the provided error with the localization service
// registered on the shelf. If no localization service is registered, the error
// is returned unchanged.
func (s *Shelf[T]) LocalizeError(err error) error {
	if s.localizationService == nil {
		return err
	}
	return s.localizationService.LocalizeError(err)
}

// GetCapabilities Retrieves the previously registered capabilities or nil
func (s *Shelf[T]) GetCapabilities() map[string]Capability {
	return s.capabilities
}
