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

// package main demonstrates explicit end-user security contexts in an HTTP service
package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	"github.com/oracle/go-oracledb/v26/oracle"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	osql "github.com/oracle/go-oracledb/v26/oracle/sql"
)

// main starts the HTTP example
func main() {
	if len(os.Args) != 1 {
		log.Fatal("this example takes no command-line arguments")
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run opens the database pool and starts the HTTP server
func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	db := sql.OpenDB(cfg.connector)
	defer db.Close()

	api := &employeeAPI{cfg: cfg, db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /employees/username", api.byUsername)
	if cfg.appScope != "" {
		mux.HandleFunc("POST /employees/token", api.byToken)
		log.Print("serving POST /employees/token")
	}

	server := &http.Server{
		Addr:         "localhost:8080",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 50 * time.Second,
	}
	log.Print("serving POST /employees/username")
	return server.ListenAndServe()
}

// employeeAPI uses an explicit security context for each request
type employeeAPI struct {
	cfg *sampleConfig
	db  *sql.DB
}

// byUsername uses a database-managed end-user name supplied in the JSON body
func (a *employeeAPI) byUsername(w http.ResponseWriter, r *http.Request) {
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "send a JSON request body")
		return
	}
	var input struct {
		Username string `json:"username"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	if err := decoder.Decode(&input); err != nil || input.Username == "" {
		writeError(w, http.StatusBadRequest, "provide a database end-user name in the username field")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	a.employees(w, ctx, osql.EndUserSecurityContext{NamedUser: osql.NamedEndUser{Username: input.Username}})
}

// byToken exchanges the demo user's OCI IAM credentials for an end-user token
func (a *employeeAPI) byToken(w http.ResponseWriter, r *http.Request) {
	const challenge = `Basic realm="DDS demo"`
	username, password, ok := r.BasicAuth()
	if !ok || username == "" || password == "" {
		w.Header().Set("WWW-Authenticate", challenge)
		writeError(w, http.StatusUnauthorized, "provide an OCI IAM username and password using Basic authentication")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	token, err := a.cfg.requestToken(ctx, a.cfg.appClientID, a.cfg.appClientSecret, url.Values{
		"grant_type": {"password"},
		"scope":      {a.cfg.appScope},
		"username":   {username},
		"password":   {password},
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not obtain an OCI IAM end-user token")
		return
	}
	a.employees(w, ctx, osql.EndUserSecurityContext{EndUserToken: token})
}

// employees obtains the application token and executes the query on the connection with the explicit context
func (a *employeeAPI) employees(w http.ResponseWriter, ctx context.Context, secCtx osql.EndUserSecurityContext) {
	token, err := a.cfg.databaseToken(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not obtain an IAM database-access token")
		return
	}
	secCtx.DatabaseAccessToken = token
	employees, err := employeesForUser(ctx, a.db, secCtx)
	if err != nil {
		log.Print(err)
		writeError(w, http.StatusBadGateway, "could not query employee records")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(employees); err != nil {
		log.Print(err)
	}
}

// employeesForUser reads the records visible to one end user on a dedicated connection
func employeesForUser(ctx context.Context, db *sql.DB, secCtx osql.EndUserSecurityContext) ([]employee, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	security, err := oracle.NewConnectionWrapper(conn)
	if err != nil {
		return nil, err
	}
	if err := security.SetEndUserSecurityContext(secCtx); err != nil {
		return nil, err
	}
	defer security.ClearEndUserSecurityContext()

	return readEmployees(ctx, conn)
}

// writeError sends a JSON error message without exposing upstream details
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// sampleConfig holds the Oracle connector and OCI IAM settings for the HTTP example
type sampleConfig struct {
	connector       driver.Connector
	appScope        string
	appClientID     string
	appClientSecret string

	oauthURL             string
	databaseClientID     string
	databaseClientSecret string
	databaseScope        string
	client               *http.Client
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
		appScope:             os.Getenv("DDS_APP_SCOPE"),
		appClientID:          os.Getenv("DDS_APP_CLIENT_ID"),
		appClientSecret:      os.Getenv("DDS_APP_CLIENT_SECRET"),
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
			return nil, common.NewOracleError(oracleErrors.InvalidConnectionParameter, nil, "empty", setting.name, "a value from SETUP.md")
		}
	}

	if c.appScope != "" {
		if c.appClientID == "" {
			return nil, common.NewOracleError(oracleErrors.InvalidConnectionParameter, nil, "empty", "DDS_APP_CLIENT_ID", "the HR Application client ID")
		}
		if c.appClientSecret == "" {
			return nil, common.NewOracleError(oracleErrors.InvalidConnectionParameter, nil, "empty", "DDS_APP_CLIENT_SECRET", "the HR Application client secret")
		}
	}

	u, err := url.Parse(strings.TrimRight(domain, "/"))
	if err != nil {
		return nil, common.NewOracleError(oracleErrors.InvalidConnectionParameter, err, "invalid", "DDS_IAM_DOMAIN_URL", "an HTTPS identity domain URL")
	}
	if u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, common.NewOracleError(oracleErrors.InvalidConnectionParameter, nil, "invalid", "DDS_IAM_DOMAIN_URL", "an HTTPS identity domain URL")
	}
	c.oauthURL = u.JoinPath("oauth2/v1").String()
	c.connector, err = oracle.NewOracleConnector(cfg)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// databaseToken requests a database-access token for the current request
func (c *sampleConfig) databaseToken(ctx context.Context) (string, error) {
	form := url.Values{"grant_type": {"client_credentials"}, "scope": {c.databaseScope}}
	return c.requestToken(ctx, c.databaseClientID, c.databaseClientSecret, form)
}

// requestToken sends a credential grant to OCI IAM and returns its access token
func (c *sampleConfig) requestToken(ctx context.Context, clientID, clientSecret string, form url.Values) (string, error) {
	if common.Odl.Enabled(context.Background(), common.OlDebug) {
		common.Odl.Debug("Deep Data Security sample requests an IAM token", "grant type", form.Get("grant_type"))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.oauthURL+"/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", common.NewOracleError(oracleErrors.InternalError, err)
	}
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", common.NewOracleError(oracleErrors.InternalError, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("IAM token request returned HTTP %d", resp.StatusCode)
		return "", common.NewOracleError(oracleErrors.InternalError, nil)
	}
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", common.NewOracleError(oracleErrors.InternalError, err)
	}
	if result.AccessToken == "" {
		return "", common.NewOracleError(oracleErrors.InternalError, nil)
	}
	return result.AccessToken, nil
}

// employee represents a sample record with a JSON null for a masked SSN
type employee struct {
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	SSN       *string `json:"ssn"`
}

// readEmployees reads the rows visible to the current end-user security context
func readEmployees(ctx context.Context, conn *sql.Conn) ([]employee, error) {
	rows, err := conn.QueryContext(ctx, "SELECT first_name, last_name, ssn FROM hr_app.employees ORDER BY employee_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	employees := []employee{}
	for rows.Next() {
		var record employee
		var ssn sql.NullString
		if err := rows.Scan(&record.FirstName, &record.LastName, &ssn); err != nil {
			return nil, err
		}
		if ssn.Valid {
			record.SSN = &ssn.String
		}
		employees = append(employees, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return employees, nil
}
