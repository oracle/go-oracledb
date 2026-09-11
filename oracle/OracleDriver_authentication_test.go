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

package oracle

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	oracleProviders "github.com/oracle/go-oracledb/v26/oracle/providers"
)

const (
	testOCITokenFileName      = "token"
	testOCIPrivateKeyFileName = "oci_db_key.pem"
)

// testOCITokenProvider implements SignedTokenAuthenticationProvider with token
// and private key material read from files configured by the integration test.
type testOCITokenProvider struct {
	tokenPath      string
	privateKeyPath string
}

// Token returns the OCI IAM token read from tokenPath.
//
// Parameters:
//   - _ : the caller context, unused by this file-backed provider.
//
// Returns:
//   - the trimmed token content.
//   - an error if tokenPath cannot be read.
func (p testOCITokenProvider) Token(_ context.Context) (string, error) {
	return readTrimmedFile(p.tokenPath)
}

// PrivateKeyForToken returns the PEM private key associated with token.
//
// Parameters:
//   - _ : the caller context, unused by this file-backed provider.
//   - token: the token being authenticated.
//
// Returns:
//   - the PEM-encoded private key bytes.
//   - an error if privateKeyPath cannot be read.
func (p testOCITokenProvider) PrivateKeyForToken(_ context.Context, token string) ([]byte, error) {
	return os.ReadFile(p.privateKeyPath)
}

// testOAuthTokenProvider implements TokenAuthenticationProvider with a token
// read from a file configured by the integration test.
type testOAuthTokenProvider struct {
	tokenPath string
}

// Token returns the OAuth access token read from tokenPath.
//
// Parameters:
//   - _ : the caller context, unused by this file-backed provider.
//
// Returns:
//   - the trimmed token content.
//   - an error if tokenPath cannot be read.
func (p testOAuthTokenProvider) Token(_ context.Context) (string, error) {
	return readTrimmedFile(p.tokenPath)
}

// TestDriver_Authentication_TTIWRN verifies that an Oracle warning emitted
// during authentication is consumed without failing the connection and is
// reported through the driver logger.
//
// The configured database user must be allowed to create profiles and users.
func TestDriver_Authentication_TTIWRN(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	adminDB, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open administrative connection: %v", err)
	}
	defer adminDB.Close()

	namePrefix := "PGD"
	var containerName string
	if err = adminDB.QueryRowContext(ctx, "SELECT SYS_CONTEXT('USERENV', 'CON_NAME') FROM DUAL").Scan(&containerName); err != nil {
		t.Fatalf("failed to determine the current database container: %v", err)
	}
	if strings.EqualFold(containerName, "CDB$ROOT") {
		namePrefix = "C##PGD"
	}
	t.Log("containerName:", containerName)
	suffix := time.Now().UnixNano() % 1_000_000_000
	profileName := fmt.Sprintf("%s_WRN_P_%09d", namePrefix, suffix)
	userName := fmt.Sprintf("%s_WRN_U_%09d", namePrefix, suffix)
	const password = "TtiWrnTest42"

	t.Log("userName:", userName)
	t.Log("profileName:", profileName)

	createProfile := fmt.Sprintf(`CREATE PROFILE %s LIMIT
		PASSWORD_LIFE_TIME 1/86400
		PASSWORD_GRACE_TIME 1
		PASSWORD_VERIFY_FUNCTION NULL`, profileName)
	if _, err = adminDB.ExecContext(ctx, createProfile); err != nil {
		if isInsufficientPrivilegeError(err) {
			t.Skipf("configured user cannot create profiles: %v", err)
		}
		t.Fatalf("failed to create warning test profile: %v", err)
	}
	defer func() {
		if _, dropErr := adminDB.ExecContext(context.Background(), "DROP PROFILE "+profileName+" CASCADE"); dropErr != nil {
			t.Logf("failed to drop warning test profile %s: %v", profileName, dropErr)
		}
	}()

	createUser := fmt.Sprintf(`CREATE USER %s IDENTIFIED BY "%s" PROFILE %s`, userName, password, profileName)
	t.Log("createUser:", createUser)
	if _, err = adminDB.ExecContext(ctx, createUser); err != nil {
		if isInsufficientPrivilegeError(err) {
			t.Skipf("configured user cannot create users: %v", err)
		}
		t.Fatalf("failed to create warning test user: %v", err)
	}
	defer func() {
		if _, dropErr := adminDB.ExecContext(context.Background(), "DROP USER "+userName+" CASCADE"); dropErr != nil {
			t.Logf("failed to drop warning test user %s: %v", userName, dropErr)
		}
	}()

	if _, err = adminDB.ExecContext(ctx, "GRANT CREATE SESSION TO "+userName); err != nil {
		t.Fatalf("failed to grant CREATE SESSION to warning test user: %v", err)
	}

	// PASSWORD_LIFE_TIME has one-second granularity. Allow an additional second
	// so the next authentication occurs reliably after the expiry timestamp.
	time.Sleep(2 * time.Second)

	warningConfig := TestingConfig.Clone()
	warningConfig.Credentials.Username = userName
	warningConfig.Credentials.Password = password
	warningConfig.Credentials.LogonMode = ""

	var logOutput bytes.Buffer
	previousLogger := common.Odl
	common.Odl = slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelWarn}))
	defer func() { common.Odl = previousLogger }()

	warningDB, err := sql.Open(warningConfig.Driver.Name, warningConfig.GetConnectionString())
	if err != nil {
		t.Fatalf("failed to create warning test database handle: %v", err)
	}
	defer warningDB.Close()

	t.Log("warningDB:", warningConfig.GetConnectionString())
	if err = warningDB.PingContext(ctx); err != nil {
		t.Fatalf("authentication with a password-expiry warning failed: %v", err)
	}

	if output := logOutput.String(); !strings.Contains(output, "Authentication warning received") {
		t.Fatalf("authentication succeeded but ORA-28002 was not logged: %s", output)
	}
}

func isInsufficientPrivilegeError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), string(oracleErrors.InsufficientPrivilege)) ||
		strings.Contains(err.Error(), "ORA-01031"))
}

// TestDriver_Authentication_OCIToken verifies OCI IAM token authentication by
// connecting to an Autonomous Database and querying DUAL. Set
// ORACLE_GO_OCI_TOKEN_CONNECT_DESCRIPTOR, ORACLE_GO_OCI_TOKEN_LOCATION (optional)
// and ORACLE_GO_OCI_TOKEN_EXPECTED_USER to run this integration test.
func TestDriver_Authentication_OCIToken(t *testing.T) {
	connectDescriptor := os.Getenv("ORACLE_GO_OCI_TOKEN_CONNECT_DESCRIPTOR")
	tokenLocation := os.Getenv("ORACLE_GO_OCI_TOKEN_LOCATION")
	expectedUser := os.Getenv("ORACLE_GO_OCI_TOKEN_EXPECTED_USER")

	if connectDescriptor == "" || expectedUser == "" || tokenLocation == "" {
		t.Skip("OCI token authentication requires ORACLE_GO_OCI_TOKEN_CONNECT_DESCRIPTOR, ORACLE_GO_OCI_TOKEN_LOCATION and ORACLE_GO_OCI_TOKEN_EXPECTED_USER")
	}

	cfg := NewOracleDriverConfig()
	cfg.ConnectDescriptor = connectDescriptor

	connector, err := NewOracleConnector(cfg)
	if err != nil {
		t.Fatalf("failed to create OCI token connector: %v", err)
	}
	registrar, ok := connector.(oracleProviders.ProviderRegistrar)
	if !ok {
		t.Fatal("connector does not support provider registration")
	}
	registrar.RegisterProvider(testOCITokenProvider{
		tokenPath:      filepath.Join(tokenLocation, testOCITokenFileName),
		privateKeyPath: filepath.Join(tokenLocation, testOCIPrivateKeyFileName),
	})

	db := sql.OpenDB(connector)
	defer db.Close()

	ctx := context.Background()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("OCI token authentication ping failed: %v", err)
	}

	var result string
	if err := db.QueryRowContext(ctx, "SELECT USER FROM SYS.DUAL").Scan(&result); err != nil {
		t.Fatalf("OCI token authentication query failed: %v", err)
	}
	if result != expectedUser {
		t.Fatalf("unexpected OCI token authentication query result: got %q, want %q", result, expectedUser)
	}
}

// TestDriver_Authentication_OAuth verifies OAuth token authentication by
// connecting to a Database and querying DUAL. Set
// ORACLE_GO_OAUTH_CONNECT_DESCRIPTOR, ORACLE_GO_OAUTH_TOKEN_LOCATION and
// ORACLE_GO_OAUTH_EXPECTED_USER to run this integration test.
func TestDriver_Authentication_OAuth(t *testing.T) {
	connectDescriptor := os.Getenv("ORACLE_GO_OAUTH_CONNECT_DESCRIPTOR")
	tokenLocation := os.Getenv("ORACLE_GO_OAUTH_TOKEN_LOCATION")
	expectedUser := os.Getenv("ORACLE_GO_OAUTH_EXPECTED_USER")

	if connectDescriptor == "" || tokenLocation == "" || expectedUser == "" {
		t.Skip("OAuth token authentication requires ORACLE_GO_OAUTH_CONNECT_DESCRIPTOR, ORACLE_GO_OAUTH_TOKEN_LOCATION and ORACLE_GO_OAUTH_EXPECTED_USER")
	}

	cfg := NewOracleDriverConfig()
	cfg.ConnectDescriptor = connectDescriptor

	connector, err := NewOracleConnector(cfg)
	if err != nil {
		t.Fatalf("failed to create OAuth token connector: %v", err)
	}
	registrar, ok := connector.(oracleProviders.ProviderRegistrar)
	if !ok {
		t.Fatal("connector does not support provider registration")
	}
	registrar.RegisterProvider(testOAuthTokenProvider{tokenPath: tokenLocation})

	db := sql.OpenDB(connector)
	defer db.Close()

	ctx := context.Background()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("OAuth token authentication ping failed: %v", err)
	}

	var result string
	if err := db.QueryRowContext(ctx, "SELECT USER FROM SYS.DUAL").Scan(&result); err != nil {
		t.Fatalf("OAuth token authentication query failed: %v", err)
	}
	if result != expectedUser {
		t.Fatalf("unexpected OAuth token authentication query result: got %q, want %q", result, expectedUser)
	}
}

// readTrimmedFile returns the contents of path without surrounding whitespace.
//
// Parameters:
//   - path: the file to read.
//
// Returns:
//   - the trimmed file contents.
//   - an error if path cannot be read.
func readTrimmedFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}
