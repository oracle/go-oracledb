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

// package main demonstrates supplying a DDS context for each query through a provider
package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	"github.com/oracle/go-oracledb/v26/oracle"
	"github.com/oracle/go-oracledb/v26/oracle/providers"
	osql "github.com/oracle/go-oracledb/v26/oracle/sql"
)

// main loads the settings and serves the provider example
func main() {
	if len(os.Args) != 1 {
		log.Fatal("this example takes no command-line arguments")
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	if err := serveEmployees(cfg); err != nil {
		log.Fatal(err)
	}
}

// serveEmployees registers the provider before opening the pool and serving the endpoint
func serveEmployees(cfg *sampleConfig) error {
	provider := &securityProvider{cfg: cfg}
	registrar := cfg.connector.(providers.ProviderRegistrar)
	registrar.RegisterProvider(provider)

	db := sql.OpenDB(cfg.connector)
	defer db.Close()
	// let requests for different users reuse the same physical connection
	db.SetMaxOpenConns(1)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /employees/username", func(w http.ResponseWriter, r *http.Request) {
		queryEmployees(w, r, db)
	})
	server := &http.Server{
		Addr:         "localhost:8081",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 35 * time.Second,
	}
	log.Print("serving POST /employees/username")
	return server.ListenAndServe()
}

// userKey identifies the request's end user in its context
type userKey struct{}

// queryEmployees passes the selected local end user to the query in its Go context
func queryEmployees(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	var input struct {
		Username string `json:"username"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	if err := decoder.Decode(&input); err != nil || input.Username == "" {
		http.Error(w, "send a JSON body with a username", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	// store this request's username in the context passed to readEmployees
	ctx = context.WithValue(ctx, userKey{}, input.Username)
	employees, err := readEmployees(ctx, db)
	if err != nil {
		log.Print(err)
		http.Error(w, "could not query employee records", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(employees); err != nil {
		log.Print(err)
	}
}

// employee represents a sample record with a JSON null for a masked SSN
type employee struct {
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	SSN       *string `json:"ssn"`
}

// readEmployees reads the rows visible to the current end-user security context
func readEmployees(ctx context.Context, db *sql.DB) ([]employee, error) {
	// pass the same context to QueryContext so the driver can give it to the provider
	rows, err := db.QueryContext(ctx, "SELECT first_name, last_name, ssn FROM hr_app.employees ORDER BY employee_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	employees := []employee{}
	for rows.Next() {
		var record employee
		if err := rows.Scan(&record.FirstName, &record.LastName, &record.SSN); err != nil {
			return nil, err
		}
		employees = append(employees, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return employees, nil
}

// securityProvider combines the application's token with the user of each query
type securityProvider struct {
	cfg *sampleConfig
}

var _ providers.DeepDataSecurityProvider = (*securityProvider)(nil)

// read the username that the handler passed to QueryContext
func (p *securityProvider) EndUserSecurityContext(ctx context.Context) (osql.EndUserSecurityContext, error) {
	username, _ := ctx.Value(userKey{}).(string)
	if username == "" {
		return osql.EndUserSecurityContext{}, errors.New("missing end-user name in query context")
	}
	// reuse the application's token or renew it when it is close to expiring
	token, err := p.cfg.databaseToken(ctx)
	if err != nil {
		return osql.EndUserSecurityContext{}, err
	}
	return osql.EndUserSecurityContext{
		DatabaseAccessToken: token,
		NamedUser:           osql.NamedEndUser{Username: username},
	}, nil
}

// accessToken holds an IAM access token and its refresh time
type accessToken struct {
	value     string
	refreshAt time.Time
}

// databaseToken reuses the application's client-credentials token and renews it
// on demand shortly before it expires
func (c *sampleConfig) databaseToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if time.Now().Before(c.cachedToken.refreshAt) {
		return c.cachedToken.value, nil
	}
	requestedAt := time.Now()
	form := url.Values{"grant_type": {"client_credentials"}, "scope": {c.databaseScope}}
	if common.Odl.Enabled(context.Background(), common.OlDebug) {
		common.Odl.Debug("Deep Data Security provider requests a database-access token")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.oauthURL+"/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(c.databaseClientID, c.databaseClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("IAM token request returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.AccessToken == "" || result.ExpiresIn <= 0 || result.ExpiresIn > math.MaxInt64/int64(time.Second) {
		return "", errors.New("IAM response requires an access token and a valid lifetime")
	}
	lifetime := time.Duration(result.ExpiresIn) * time.Second
	expiresAt := requestedAt.Add(lifetime)
	c.cachedToken = accessToken{
		value:     result.AccessToken,
		refreshAt: expiresAt.Add(-min(30*time.Second, lifetime/10)),
	}
	log.Printf("Obtained a database-access token (expires at %s)", expiresAt.Format(time.RFC3339))
	return c.cachedToken.value, nil
}

// sampleConfig holds the Oracle connector and OCI IAM settings for the provider example
type sampleConfig struct {
	connector driver.Connector

	oauthURL             string
	databaseClientID     string
	databaseClientSecret string
	databaseScope        string
	client               *http.Client

	tokenMu     sync.Mutex
	cachedToken accessToken
}

// loadConfig reads the database and OCI IAM environment variables
func loadConfig() (*sampleConfig, error) {
	cfg := oracle.NewOracleDriverConfig()
	cfg.ConnectDescriptor = os.Getenv("DDS_CONNECT_DESCRIPTOR")
	cfg.Credentials.User = os.Getenv("DDS_DB_USER")
	cfg.Credentials.Password = os.Getenv("DDS_DB_PASSWORD")
	domain := os.Getenv("DDS_IAM_DOMAIN_URL")
	c := &sampleConfig{
		databaseClientID:     os.Getenv("DDS_DATABASE_CLIENT_ID"),
		databaseClientSecret: os.Getenv("DDS_DATABASE_CLIENT_SECRET"),
		databaseScope:        os.Getenv("DDS_DATABASE_SCOPE"),
		client: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
	for _, setting := range []struct{ name, value string }{
		{"DDS_CONNECT_DESCRIPTOR", cfg.ConnectDescriptor},
		{"DDS_DB_USER", cfg.Credentials.User},
		{"DDS_DB_PASSWORD", cfg.Credentials.Password},
		{"DDS_IAM_DOMAIN_URL", domain},
		{"DDS_DATABASE_CLIENT_ID", c.databaseClientID},
		{"DDS_DATABASE_CLIENT_SECRET", c.databaseClientSecret},
		{"DDS_DATABASE_SCOPE", c.databaseScope},
	} {
		if setting.value == "" {
			return nil, fmt.Errorf("%s is required", setting.name)
		}
	}

	u, err := url.Parse(strings.TrimRight(domain, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse DDS_IAM_DOMAIN_URL: %w", err)
	}
	if u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("DDS_IAM_DOMAIN_URL must be an HTTPS identity domain URL")
	}
	c.oauthURL = u.JoinPath("oauth2/v1").String()
	c.connector, err = oracle.NewOracleConnector(cfg)
	if err != nil {
		return nil, err
	}
	return c, nil
}
