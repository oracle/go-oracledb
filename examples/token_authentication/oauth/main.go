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

// Package main shows OAuth token authentication using a file-backed
// provider registered on an Oracle connector.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/oracle/go-oracledb/v26/oracle"
	oracleProviders "github.com/oracle/go-oracledb/v26/oracle/providers"
)

// fileOAuthTokenProvider implements TokenAuthenticationProvider interface.
type fileOAuthTokenProvider struct {
	tokenPath string
}

// Token returns the token used for token authentication
func (p *fileOAuthTokenProvider) Token(context.Context) (string, error) {
	return readTrimmedFile(p.tokenPath)
}

func main() {
	connectDescriptor := requiredEnv("ORACLE_GO_OAUTH_CONNECT_DESCRIPTOR")
	tokenPath := requiredEnv("ORACLE_GO_OAUTH_TOKEN_FILE")

	cfg := oracle.NewOracleDriverConfig()
	cfg.ConnectDescriptor = connectDescriptor

	connector, err := oracle.NewOracleConnector(cfg)
	if err != nil {
		log.Fatal(err)
	}

	// Check that the connector implements ProviderRegistrar
	registrar, ok := connector.(oracleProviders.ProviderRegistrar)
	if !ok {
		log.Fatal("connector does not support provider registration")
	}
	// register the provider, the provider methods will be called by
	// the driver during token-based authentication
	registrar.RegisterProvider(&fileOAuthTokenProvider{tokenPath: tokenPath})

	db := sql.OpenDB(connector)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatal(err)
	}

	var current_user, authenticated_identity, identification_type, enterprise_identity, proxy_user sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT "+
		"SYS_CONTEXT('userenv', 'current_user') AS current_user,"+
		" SYS_CONTEXT('userenv', 'authenticated_identity') AS authenticated_identity,"+
		" SYS_CONTEXT('userenv', 'IDENTIFICATION_TYPE') AS identification_type,"+
		" SYS_CONTEXT('USERENV','ENTERPRISE_IDENTITY') AS enterprise_identity,"+
		" sys_context('userenv','proxy_user') as proxy_user FROM sys.dual").
		Scan(&current_user, &authenticated_identity, &identification_type, &enterprise_identity, &proxy_user); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("current_user: %s\n", current_user.String)
	fmt.Printf("authenticated_identity: %s\n", authenticated_identity.String)
	fmt.Printf("identification_type: %s\n", identification_type.String)
	fmt.Printf("enterprise_identity: %s\n", enterprise_identity.String)
	fmt.Printf("proxy_user: %s\n", proxy_user.String)
}

func requiredEnv(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		log.Fatalf("missing required environment variable %s", name)
	}
	return value
}

func readTrimmedFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}
