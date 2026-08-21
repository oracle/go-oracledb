/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package ttc

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleConfig "github.com/oracle/go-oracledb/v26/oracle/config"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	tokenFileName         = "token"
	ociPrivateKeyFileName = "oci_db_key.pem"
)

type tokenProviderContext struct {
	connectString  string
	sessionContext *driverCommon.SessionContext
	tokenLocation  string
}

type tokenProvider interface {
	logonMode() int64
	resolveTokenPath(tokenLocation string) (string, error)
	applyAuthData(oauthPacket *oAuth, token string, ctx tokenProviderContext) error
}

type tokenAuthenticator struct {
	tokenAuthentication oracleConfig.TokenAuthenticationType
	accessToken         string
	tokenLocation       string
	connectString       string
	provider            tokenProvider
	shelf               ttiShelf[driverCommon.MessageType]
	sessionContext      *driverCommon.SessionContext
}

type ociTokenProvider struct{}
type oauthTokenProvider struct{}

func NewTokenAuthenticator(tokenAuthentication oracleConfig.TokenAuthenticationType, accessToken, tokenLocation, connectString string) *tokenAuthenticator {
	normalizedAuth := oracleConfig.TokenAuthenticationType(strings.ToUpper(strings.TrimSpace(tokenAuthentication.String())))
	return &tokenAuthenticator{
		tokenAuthentication: normalizedAuth,
		accessToken:         strings.TrimSpace(accessToken),
		tokenLocation:       strings.TrimSpace(tokenLocation),
		connectString:       connectString,
		provider:            newTokenProvider(normalizedAuth),
	}
}

func newTokenProvider(tokenAuthentication oracleConfig.TokenAuthenticationType) tokenProvider {
	switch oracleConfig.TokenAuthenticationType(strings.ToUpper(strings.TrimSpace(tokenAuthentication.String()))) {
	case oracleConfig.TokenAuthenticationOCI:
		return ociTokenProvider{}
	case oracleConfig.TokenAuthenticationOAuth:
		return oauthTokenProvider{}
	default:
		return nil
	}
}

func (ta *tokenAuthenticator) SetShelf(shelf *ttiShelf[driverCommon.MessageType]) {
	ta.shelf = *shelf
}

func (ta *tokenAuthenticator) SetSessionContext(sessCtx *driverCommon.SessionContext) {
	ta.sessionContext = sessCtx
}

func (ta *tokenAuthenticator) Authenticate(ctx context.Context) error {
	common.Odl.Debug("Start TOKEN authentication")
	if ta.provider == nil {
		return common.NewOracleError(oracleErrors.NoAuthenticatorError, nil, ta.tokenAuthentication)
	}
	if ta.sessionContext == nil {
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}

	token, err := ta.resolveAccessToken()
	if err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}
	if err := validateJWTExpiration(token); err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	return ta.doOAuth(ctx, token)
}

func (ta *tokenAuthenticator) doOAuth(ctx context.Context, token string) error {
	shelf := ta.shelf
	streamer := shelf.GetMessageStreamer().(MessageStreamerInterface)

	msg, err := shelf.GetMessageFactory().(Factory).GetMessageForFunction(TTIFUN, oauth)
	if err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	oauthMsg := msg.(*oAuth)
	oauthMsg.setConnectString(ta.connectString)
	oauthMsg.setLogonMode(ta.provider.logonMode())
	if err := oauthMsg.prepareForTokenOAUTH(driverCommon.B1Array{}); err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}
	if err := ta.provider.applyAuthData(oauthMsg, token, tokenProviderContext{
		connectString:  ta.connectString,
		sessionContext: ta.sessionContext,
		tokenLocation:  ta.tokenLocation,
	}); err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	if err := streamer.Push(ctx, msg); err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}
	if err := streamer.Flush(ctx); err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	oauthRPACallBack := func(t *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		return shelf.GetMessageFactory().(Factory).GetMessageForFunction(TTIRPA, oauth)
	}
	streamer.RegisterPreUnmarshallCallback(TTIRPA, oauthRPACallBack)
	defer streamer.UnRegisterPreUnmarshallCallback(TTIRPA)

	var oauthrpa *OAuthRPA
	for {
		msg, err := streamer.Pull(ctx, TTIRPA, TTIOER, TTIWRN)
		if err != nil {
			return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
		}
		switch msg.GetMsgCode() {
		case TTIRPA:
			oauthrpa = msg.(*OAuthRPA)
		case TTIOER:
			ttioer := msg.(tTIOerIface)
			if err := ttioer.getError(); err != nil {
				return err
			}
			if oauthrpa == nil {
				return common.NewOracleError(oracleErrors.InternalError, nil, nil)
			}
			ta.sessionContext.UpdateSessionProperties(oauthrpa.connectionValues)
			return nil
		case TTIWRN:
			logAuthenticationWarning(msg.(*tTIwrn))
		default:
			return common.NewOracleError(oracleErrors.InternalError, nil, nil)
		}
	}
}

func (ta *tokenAuthenticator) resolveAccessToken() (string, error) {
	if ta.accessToken != "" {
		return ta.accessToken, nil
	}

	tokenPath, err := ta.provider.resolveTokenPath(ta.tokenLocation)
	if err != nil {
		return "", err
	}

	tokenBytes, err := os.ReadFile(tokenPath)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(tokenBytes))
	if token == "" {
		return "", common.NewOracleError(oracleErrors.EmptyTokenError, nil, ta.String())
	}
	return token, nil
}

func (provider ociTokenProvider) logonMode() int64 {
	return common.KpzLogon.Value() | common.KpzLogonToken.Value()
}

func (oauthTokenProvider) logonMode() int64 {
	return common.KpzLogon.Value()
}

func (provider ociTokenProvider) resolveTokenPath(tokenLocation string) (string, error) {
	tokenDir, err := resolveTokenDirectory(tokenLocation, filepath.Join(".oci", "db-token"))
	if err != nil {
		return "", err
	}
	return filepath.Join(tokenDir, tokenFileName), nil
}

func (oauthTokenProvider) resolveTokenPath(tokenLocation string) (string, error) {
	return resolveTokenLocation(tokenLocation)
}

func (provider ociTokenProvider) applyAuthData(oauthPacket *oAuth, token string, ctx tokenProviderContext) error {
	header, err := provider.generateTokenHeader(ctx)
	if err != nil {
		return err
	}
	keyPath, err := resolveOCIPrivateKeyPath(ctx.tokenLocation)
	if err != nil {
		return err
	}
	signer, err := readOCIPrivateKey(keyPath)
	if err != nil {
		return err
	}
	return oauthPacket.setTokenKeyValsForOAUTH(token, header, signer)
}

func (oauthTokenProvider) applyAuthData(oauthPacket *oAuth, token string, _ tokenProviderContext) error {
	return oauthPacket.setTokenKeyValsForOAUTH(token, "", nil)
}

func (provider ociTokenProvider) generateTokenHeader(ctx tokenProviderContext) (string, error) {
	serviceName, err := extractServiceName(ctx.connectString)
	if err != nil {
		return "", err
	}
	remoteAddr, err := getRequiredSessionProperty(ctx.sessionContext, "REMOTE_ADDRESS")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"date: %s\n(request-target): %s\nhost: %s",
		time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"),
		serviceName,
		remoteAddr,
	), nil
}

func getRequiredSessionProperty(sessionContext *driverCommon.SessionContext, key string) (string, error) {
	value, ok := sessionContext.GetSessionProperties().GetProperty(key).(string)
	if !ok {
		return "", common.NewOracleError(oracleErrors.ValueRetrievalError, nil, key)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", common.NewOracleError(oracleErrors.ValueRetrievalError, nil, key)
	}
	return value, nil
}

func extractServiceName(connectString string) (string, error) {
	common.Odl.Debug("ConnectionString", "connectString", connectString)
	return extractAddressValue(connectString, "SERVICE_NAME")
}

func extractAddressValue(connectString, key string) (string, error) {
	upper := strings.ToUpper(connectString)
	idx := strings.Index(upper, key+"=")
	if idx == -1 {
		return "", common.NewOracleError(oracleErrors.ValueRetrievalError, nil, key)
	}
	start := idx + len(key) + 1
	end := strings.IndexAny(connectString[start:], ")")
	if end == -1 {
		return "", common.NewOracleError(oracleErrors.ValueRetrievalError, nil, key)
	}
	return strings.TrimSpace(connectString[start : start+end]), nil
}

func resolveTokenDirectory(tokenLocation, defaultRelativePath string) (string, error) {
	if tokenLocation != "" {
		info, err := os.Stat(tokenLocation)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			return tokenLocation, nil
		}
		return filepath.Dir(tokenLocation), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, defaultRelativePath), nil
}

func resolveTokenLocation(tokenLocation string) (string, error) {
	if strings.TrimSpace(tokenLocation) == "" {
		return "", common.NewOracleError(oracleErrors.MissingTokenLocationError, nil)
	}
	info, err := os.Stat(tokenLocation)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return filepath.Join(tokenLocation, tokenFileName), nil
	}
	return tokenLocation, nil
}

func resolveOCIPrivateKeyPath(tokenLocation string) (string, error) {
	tokenDir, err := resolveTokenDirectory(tokenLocation, filepath.Join(".oci", "db-token"))
	if err != nil {
		return "", err
	}
	return filepath.Join(tokenDir, ociPrivateKeyFileName), nil
}

func (ta *tokenAuthenticator) String() string {
	return fmt.Sprintf("TokenAuthenticator{tokenAuthentication=%s, tokenLocation=%s}", ta.tokenAuthentication, ta.tokenLocation)
}

func readOCIPrivateKey(path string) (crypto.Signer, error) {
	keyPEM, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, common.NewOracleError(oracleErrors.InvalidPrivateKey, nil)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, common.NewOracleError(oracleErrors.InvalidPrivateKey, nil)
	}
	if _, ok := signer.(*rsa.PrivateKey); !ok {
		return nil, common.NewOracleError(oracleErrors.InvalidPrivateKey, nil)
	}
	return signer, nil
}

func validateJWTExpiration(token string) error {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}

	common.Odl.Debug("JWTToken", "token", token, "payload", payload)
	var claims struct {
		Exp *int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == nil {
		return nil
	}
	if time.Unix(*claims.Exp, 0).Before(time.Now()) {
		return common.NewOracleError(oracleErrors.ExpiredToken, nil)
	}
	return nil
}

func signTokenHeader(header string, signer crypto.Signer) (string, error) {
	sum := sha256.Sum256([]byte(header))
	signature, err := signer.Sign(rand.Reader, sum[:], crypto.SHA256)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}
