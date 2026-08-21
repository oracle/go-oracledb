package config

import (
	"slices"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

type TokenAuthenticationType string

const (
	// TokenAuthenticationOCI selects OCI IAM token authentication using a token
	// and OCI database private key bundle.
	TokenAuthenticationOCI TokenAuthenticationType = "OCI_TOKEN"
	// TokenAuthenticationOAuth selects generic OAuth bearer token
	// authentication without an OCI database private key.
	TokenAuthenticationOAuth TokenAuthenticationType = "OAUTH"
)

// String returns the Oracle connection-property string value for the token
// authentication type.
func (t TokenAuthenticationType) String() string {
	return string(t)
}

var tokenAuthenticationTypeValues = map[string]TokenAuthenticationType{
	TokenAuthenticationOCI.String():   TokenAuthenticationOCI,
	TokenAuthenticationOAuth.String(): TokenAuthenticationOAuth,
}

var AllTokenAuthenticationTypeNames = slices.Collect(func(yield func(string) bool) {
	for k := range tokenAuthenticationTypeValues {
		if !yield(k) {
			return
		}
	}
})

// ParseTokenAuthenticationType normalizes and validates a token
// authentication type string and returns the corresponding typed constant.
func ParseTokenAuthenticationType(value, valueName string) (TokenAuthenticationType, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" {
		return "", nil
	}

	if tokenAuthenticationType, ok := tokenAuthenticationTypeValues[normalized]; ok {
		return tokenAuthenticationType, nil
	}

	return "", common.NewOracleError(
		oracleErrors.InvalidConnectionParameter,
		nil,
		value,
		valueName,
		AllTokenAuthenticationTypeNames,
	)
}

// IsValid reports whether the token authentication type is one of the
// supported token authentication constants.
func (t TokenAuthenticationType) IsValid() bool {
	_, ok := tokenAuthenticationTypeValues[t.String()]
	return ok
}
