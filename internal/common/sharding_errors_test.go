package common

import (
	"strings"
	"testing"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"golang.org/x/text/language"
)

// TestShardingErrorCatalog verifies all sharding error codes have usable
// messages and correctly formatted arguments in every supported language.
func TestShardingErrorCatalog(t *testing.T) {
	cases := []struct {
		code oracleErrors.ErrorCode
		args []interface{}
	}{
		{oracleErrors.ShardingKeyEmpty, nil},
		{oracleErrors.ShardingSubkeyInvalid, []interface{}{99}},
		{oracleErrors.ShardingSubkeyEncoding, []interface{}{0, 99}},
		{oracleErrors.ShardingKeyTooLong, []interface{}{65536, 65535}},
		{oracleErrors.ShardingContextRequired, nil},
		{oracleErrors.ShardingEncodingInvalid, []interface{}{"invalid header"}},
		{oracleErrors.ShardingKeyConflict, []interface{}{"SHARDING_KEY_B64"}},
		{oracleErrors.ConnectionDataTooLong, []interface{}{65536, 65535}},
	}
	for _, lang := range []language.Tag{language.English, language.French, language.Spanish, language.Portuguese, language.Italian} {
		service := NewLocalizationService(lang)
		for _, tc := range cases {
			err := service.LocalizeError(NewOracleError(tc.code, nil, tc.args...))
			text := err.Error()
			if strings.Contains(text, "%!") || strings.Contains(text, "Unknown error") || !strings.HasPrefix(text, string(tc.code)+" - ") {
				t.Errorf("%s %s: %s", lang, tc.code, text)
			}
		}
	}
}
