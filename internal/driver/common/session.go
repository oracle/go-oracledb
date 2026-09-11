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

const (
	TimeZoneVersionNumber    = "timeZoneVersionNumber"
	DriverCharacterSet       = "driverCharacterSet"
	SessionNCharCharacterSet = "sessionNCharCharacterSet"
	RemoteAddress            = "remoteAddress"
	RemotePort               = "remotePort"
	ConnectDescriptor        = "connectDescriptor"
)

// SessionContext connection session context. This holds database session information
type SessionContext struct {
	sessionProperties *Properties[string] // sessionProperties represents the negotiated properties during OAuth
	clientProperties  *Properties[string] // clientProperties contains properties needed by the client
}

// SetTimeZoneVersionNumber sets the time zone version number in the session context.
func (s *SessionContext) SetTimeZoneVersionNumber(v byte) {
	s.clientProperties.SetProperty(TimeZoneVersionNumber, v)
}

// SetSessionCharacterSets records the negotiated driver and NCHAR character sets for the session.
// The driver character set mirrors cliRIN/cliROUT (AL32UTF8 today) so downstream components can
// derive TTC lobAmt policies. The server database character set is intentionally not retained because
// the driver only supports AL32UTF8 pairings at present; capturing the client character set avoids
// implying wider charset coverage that is not yet implemented.
func (s *SessionContext) SetSessionCharacterSets(driverCS, ncharCS UB2) {
	s.clientProperties.SetProperty(DriverCharacterSet, driverCS)
	s.clientProperties.SetProperty(SessionNCharCharacterSet, ncharCS)
}

// DriverCharacterSet returns the negotiated driver character set identifier (cliRIN/cliROUT).
func (s *SessionContext) DriverCharacterSet() UB2 {
	if s.clientProperties != nil && s.clientProperties.ContainsKey(DriverCharacterSet) {
		if ub2value, ok := s.clientProperties.GetProperty(DriverCharacterSet).(UB2); ok {
			return ub2value
		}
	}
	return 0
}

// SessionNCharCharacterSet returns the negotiated NCHAR character set identifier (TTIPRO.NCharCharset).
func (s *SessionContext) SessionNCharCharacterSet() UB2 {
	if s.clientProperties != nil && s.clientProperties.ContainsKey(SessionNCharCharacterSet) {
		if ub2value, ok := s.clientProperties.GetProperty(SessionNCharCharacterSet).(UB2); ok {
			return ub2value
		}
	}
	return 0
}

// UpdateSessionProperties updates/adds the session properties with new values
func (s *SessionContext) UpdateSessionProperties(props *Properties[string]) {
	s.sessionProperties.PutAll(props)
}

// GetSessionProperties gets properetis of the session
func (s *SessionContext) GetSessionProperties() *Properties[string] {
	return s.sessionProperties
}

// UpdateClientProperties updates/adds the client properties with new values
func (s *SessionContext) UpdateClientProperties(props *Properties[string]) {
	s.clientProperties.PutAll(props)
}

// GetClientProperties gets the client properties
func (s *SessionContext) GetClientProperties() *Properties[string] {
	return s.clientProperties
}

// NewSessionContext creates a new context
// User is retrieved form current system user (see user.Current()).
// sessionProgram  is retrieved form current system user (see os.Executable()).
// terminal is initialized to "unknown"
func NewSessionContext() *SessionContext {
	return &SessionContext{
		sessionProperties: NewProperties[string](),
		clientProperties:  NewProperties[string](),
	}
}
