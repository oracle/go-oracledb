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
	"context"
	"database/sql"
	"testing"
	"time"
)

// ************************** NOTE ************************** //
// Tests rely on cyclops tools for delaying the network traffic
// see "Running test with cyclops" section in our CONTRIBUTING.md guide.
// Example of configuration for these tests to run are
//  - "cyclops-delayed-traffic" in configurations/driver_test_config.json
//  - cyclops_config.json file in configurations directory
// please adjust remote_host and remote_port configuration in cyclops config
// ********************************************************** //

// checkAcceptableTimeShift checks that elapsed time did not exceed
//
//		some variance due to network/OS
//		args:
//		  - expected expected time lapse
//	   - d1 duration start
//	   - d2 duration end
func checkAcceptableTimeShift(t *testing.T, expected time.Duration, d1, d2 time.Time) {
	laps := d2.Sub(d1).Milliseconds()
	shift := float64(laps-expected.Milliseconds()) / float64(expected.Milliseconds()) * 100.0
	if shift > 2.0 {
		t.Errorf("duration %v excceded tolerance of 2%% on %v", d2.Sub(d1),
			expected)
	}
}

// Test connection timeout using transport_connect_timeout property
//
//	Expectation:
//	 connection fails on ConnectTimeout error
func TestTimeoutConnectWithTransportConnectTimeout(t *testing.T) {
	t.Parallel()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	// we always use the same one for this test case.
	config, err := TestEnvironement.GetConfig("unreachable-host")
	if err != nil {
		t.Skip("No unreachable-host configuration available")
	}

	dsnProps := map[string]string{
		"transport_connect_timeout": "5s",
	}
	dsn := config.GetConnectionStringWithProperties(dsnProps)

	db, err := sql.Open(config.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("Failed to open connection: %v", err)
	}
	defer db.Close()
	start := time.Now()
	err = db.PingContext(context.Background())
	//calculate duration
	stop := time.Now()
	if err == nil {
		t.Fatalf("Expected timeout error, but got no error")
	}
	t.Logf("transport connect tm error received: [%v]", err.Error())
	checkErrorRaised(t, err, ConnectFailed, ConnectTimeout)
	checkAcceptableTimeShift(t, 5*time.Second, start, stop)

}

// Test connection timeout using transport_connect_timeout and a context
// the property will be less than the value in the context
//
//	Expectation:
//	 connection fails on ConnectTimeout
func TestTimeoutConnectWithTransportConnectTimeoutAndContext(t *testing.T) {
	t.Parallel()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	// we always use the same one for this test case.
	config, err := TestEnvironement.GetConfig("unreachable-host")
	if err != nil {
		t.Skip("No unreachable-host configuration available")
	}
	dsnProps := map[string]string{
		"transport_connect_timeout": "5s",
	}
	dsn := config.GetConnectionStringWithProperties(dsnProps)

	db, err := sql.Open(config.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("Failed to open connection: %v", err)
	}
	defer db.Close()

	// context timeout is explicitly longer
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	//record time
	start := time.Now()
	err = db.PingContext(ctx)
	//calculate duration
	stop := time.Now()
	if err == nil {
		t.Fatalf("Expected timeout error, but got no error")
	}

	t.Logf("transport connect tm error received after [%v]ms: [%v]",
		stop.Sub(start).Milliseconds(), err.Error())

	checkErrorRaised(t, err, ConnectFailed, ConnectTimeout)
	checkAcceptableTimeShift(t, 5*time.Second, start, stop)
}

// Test connection timeout using transport_connect_timeout and a context
// the property will be greater than the value in the context
//
//	Expectation:
//	 connection fails on a timeout value set in the context
func TestTimeoutConnectWithTransportConnectTimeoutAndContextGreater(t *testing.T) {
	t.Parallel()

	config, err := TestEnvironement.GetConfig("cyclops-delayed-traffic")
	if err != nil {
		t.Skip("No cyclops configuration available")
	}

	if !config.Enabled {
		t.Skip("No cyclops configuration not enabled")
	}

	dsnProps := map[string]string{
		"transport_connect_timeout": "30s",
	}
	dsn := config.GetConnectionStringWithProperties(dsnProps)

	db, err := sql.Open(config.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("Failed to open connection: %v", err)
	}
	defer db.Close()

	// context timeout is explicitly shorter
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	//record time
	start := time.Now()
	err = db.PingContext(ctx)
	//calculate duration
	stop := time.Now()
	if err == nil {
		t.Fatalf("Expected timeout error, but got no error")
	}

	t.Logf("transport connect tm error received: [%v]", err.Error())

	checkErrorRaised(t, err, ConnectFailed, CtxTimeout)
	checkAcceptableTimeShift(t, 5*time.Second, start, stop)
}

// Test connection timeout using recv_timeout property
//
//	Expectation:
//	 connection fails on timeout
func TestTimeoutConnectWithRecvTimeout(t *testing.T) {
	t.Parallel()

	config, err := TestEnvironement.GetConfig("cyclops-delayed-traffic")
	if err != nil {
		t.Skip("No cyclops configuration available")
	}

	if !config.Enabled {
		t.Skip("No cyclops configuration not enabled")
	}

	dsnProps := map[string]string{
		"recv_timeout": "5s",
	}
	dsn := config.GetConnectionStringWithProperties(dsnProps)

	db, err := sql.Open(config.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("Failed to open connection: %v", err)
	}
	defer db.Close()
	//record time
	start := time.Now()
	err = db.PingContext(context.Background())
	stop := time.Now()
	if err == nil {
		t.Fatalf("Expected timeout error, but got no error")
	}

	t.Logf("recv_timeout connect tm error received: [%v]", err.Error())

	checkErrorRaised(t, err, ConnectFailed, ConnectTimeout)
	checkAcceptableTimeShift(t, 5*time.Second, start, stop)
}

// Test connection timeout using recv_timeout property
//
//	Expectation:
//	 connection fails on timeout
func TestTimeoutConnectWithConnectTimeout(t *testing.T) {
	t.Parallel()

	config, err := TestEnvironement.GetConfig("cyclops-delayed-traffic")
	if err != nil {
		t.Skip("No cyclops configuration available")
	}

	if !config.Enabled {
		t.Skip("No cyclops configuration not enabled")
	}

	dsnProps := map[string]string{
		"connect_timeout": "5s",
	}
	dsn := config.GetConnectionStringWithProperties(dsnProps)

	db, err := sql.Open(config.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("Failed to open connection: %v", err)
	}
	defer db.Close()
	//record time
	start := time.Now()
	err = db.PingContext(context.Background())
	stop := time.Now()
	if err == nil {
		t.Fatalf("Expected timeout error, but got no error")
	}

	t.Logf("connect_timeout connect tm error received: [%v]", err.Error())

	checkErrorRaised(t, err, ConnectFailed, ConnectTimeout)
	checkAcceptableTimeShift(t, 5*time.Second, start, stop)
}

// Test connection timeout using recv_timeout and a context
// the property will be less than the value in the context
//
//	Expectation:
//	 connection fails on a timeout value set in the property
func TestTimeoutConnectWithRecvConnectTimeoutAndContext(t *testing.T) {
	t.Parallel()
	config, err := TestEnvironement.GetConfig("cyclops-delayed-traffic")
	if err != nil {
		t.Skip("No cyclops configuration available")
	}

	if !config.Enabled {
		t.Skip("No cyclops configuration not enabled")
	}

	dsnProps := map[string]string{
		"recv_timeout": "5s",
	}
	dsn := config.GetConnectionStringWithProperties(dsnProps)

	db, err := sql.Open(config.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("Failed to open connection: %v", err)
	}
	defer db.Close()

	// context timeout is explicitly longer
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	//record time
	start := time.Now()
	err = db.PingContext(ctx)
	stop := time.Now()
	if err == nil {
		t.Fatalf("Expected timeout error, but got no error")
	}

	t.Logf("recv_timeout/ctx connect tm error received: [%v]", err.Error())

	checkErrorRaised(t, err, ConnectFailed, ConnectTimeout)

	checkAcceptableTimeShift(t, 5*time.Second, start, stop)
}

// Test connection timeout using recv_timeout and a context
// the property will be greater than the value in the context
//
//	Expectation:
//	 connection fails on a timeout value set in the context
func TestTimeoutConnectWithRecvConnectTimeoutAndContextGreater(t *testing.T) {
	t.Parallel()
	config, err := TestEnvironement.GetConfig("cyclops-delayed-traffic")
	if err != nil {
		t.Skip("No cyclops configuration available")
	}

	if !config.Enabled {
		t.Skip("No cyclops configuration not enabled")
	}

	dsnProps := map[string]string{
		"recv_timeout": "60s",
	}
	dsn := config.GetConnectionStringWithProperties(dsnProps)

	db, err := sql.Open(config.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("Failed to open connection: %v", err)
	}
	defer db.Close()

	// context timeout is explicitly shorter
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	//record time
	start := time.Now()
	err = db.PingContext(ctx)
	stop := time.Now()
	if err == nil {
		t.Fatalf("Expected timeout error, but got no error")
	}

	t.Logf("ctx connect tm error received: [%v]", err.Error())

	checkErrorRaised(t, err, ConnectFailed, CtxTimeout)
	checkAcceptableTimeShift(t, 5*time.Second, start, stop)
}

// Test connection timeout using all possible ways
// transport_connect_timeout, context, recv_timout.
//
//		Expectation:
//		 connection fails on timeout according to precedence of the durations
//	  in that case the connect_timeout
func TestTimeoutConnectTimeoutPrecedence1(t *testing.T) {
	t.Parallel()
	config, err := TestEnvironement.GetConfig("cyclops-delayed-traffic")
	if err != nil {
		t.Skip("No cyclops configuration available")
	}

	if !config.Enabled {
		t.Skip("No cyclops configuration not enabled")
	}

	dsnProps := map[string]string{
		"recv_timeout":              "5s",
		"transport_connect_timeout": "10s",
		"connect_timeout":           "2s",
	}

	dsn := config.GetConnectionStringWithProperties(dsnProps)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	db, err := sql.Open(config.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("Failed to open connection: %v", err)
	}
	defer db.Close()

	start := time.Now()
	err = db.PingContext(ctx)
	stop := time.Now()
	if err == nil {
		t.Fatalf("Expected timeout error, but got no error")
	}

	t.Logf("TestTimeoutConnectTimeoutPrecedence1 error received: [%v]", err.Error())

	checkErrorRaised(t, err, ConnectFailed, ConnectTimeout)
	checkAcceptableTimeShift(t, 2*time.Second, start, stop)

}

// Test connection timeout using all possible ways
// transport_connect_timeout, context, recv_timout.
//
//		Expectation:
//		 connection fails on timeout according to precedence of the durations
//	  in that case the user context
func TestTimeoutConnectTimeoutPrecedence2(t *testing.T) {
	t.Parallel()
	config, err := TestEnvironement.GetConfig("cyclops-delayed-traffic")
	if err != nil {
		t.Skip("No cyclops configuration available")
	}

	if !config.Enabled {
		t.Skip("No cyclops configuration not enabled")
	}

	dsnProps := map[string]string{
		"connect_timeout":           "5s",
		"recv_timeout":              "15s",
		"transport_connect_timeout": "10s",
	}

	dsn := config.GetConnectionStringWithProperties(dsnProps)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	db, err := sql.Open(config.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("Failed to open connection: %v", err)
	}
	defer db.Close()

	start := time.Now()
	err = db.PingContext(ctx)
	stop := time.Now()
	if err == nil {
		t.Fatalf("Expected timeout error, but got no error")
	}

	t.Logf("TestTimeoutConnectTimeoutPrecedence2 error received: [%v]", err.Error())

	checkErrorRaised(t, err, ConnectFailed, ConnectTimeout)
	checkAcceptableTimeShift(t, 2*time.Second, start, stop)

}

// Test connection timeout using all possible ways
// transport_connect_timeout, context, recv_timout.
//
//		Expectation:
//		 connection fails on timeout according to precedence of the durations
//	    in that case, the recv_timeout
func TestTimeoutConnectTimeoutPrecedence3(t *testing.T) {
	t.Parallel()
	config, err := TestEnvironement.GetConfig("cyclops-delayed-traffic")
	if err != nil {
		t.Skip("No cyclops configuration available")
	}

	if !config.Enabled {
		t.Skip("No cyclops configuration not enabled")
	}

	dsnProps := map[string]string{
		"connect_timeout":           "5s",
		"recv_timeout":              "2s",
		"transport_connect_timeout": "10s",
	}

	dsn := config.GetConnectionStringWithProperties(dsnProps)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	db, err := sql.Open(config.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("Failed to open connection: %v", err)
	}
	defer db.Close()

	start := time.Now()
	err = db.PingContext(ctx)
	stop := time.Now()
	if err == nil {
		t.Fatalf("Expected timeout error, but got no error")
	}
	t.Logf("TestTimeoutConnectTimeoutPrecedence3 error received: [%v]", err.Error())
	checkErrorRaised(t, err, ConnectFailed, ConnectTimeout)
	checkAcceptableTimeShift(t, 2*time.Second, start, stop)

}

// Test connection timeout using all possible ways
// transport_connect_timeout, context, recv_timout.
//
//		Expectation:
//		 connection fails on timeout according to precedence of the durations
//	    in that case, the transport_connect_timeout
func TestTimeoutConnectTimeoutPrecedence4(t *testing.T) {
	t.Parallel()

	// we always use the same one for this test case.
	config, err := TestEnvironement.GetConfig("unreachable-host")
	if err != nil {
		t.Skip("No unreachable-host configuration available")
	}

	dsnProps := map[string]string{
		"connect_timeout":           "5s",
		"recv_timeout":              "50s",
		"transport_connect_timeout": "2s",
	}

	dsn := config.GetConnectionStringWithProperties(dsnProps)
	t.Logf("connecting to %s", dsn)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	db, err := sql.Open(TestingConfig.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("Failed to open connection: %v", err)
	}
	defer db.Close()

	start := time.Now()
	err = db.PingContext(ctx)
	stop := time.Now()
	if err == nil {
		t.Fatalf("Expected timeout error, but got no error")
	}

	t.Logf("TestTimeoutConnectTimeoutPrecedence4 error received: [%v]", err.Error())
	checkErrorRaised(t, err, ConnectFailed, ConnectTimeout)
	checkAcceptableTimeShift(t, 2*time.Second, start, stop)
}
