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
	"database/sql"
	"fmt"
	"testing"
)

func TestServerError(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	t.Run("TestServerError_ORA-12514", func(t *testing.T) {
		wrongSrvCfg := TestingConfig.Clone()
		wrongSrvCfg.Database.ServiceName = "INVALID_SERVICE"

		dsn := wrongSrvCfg.GetConnectionString()

		fmt.Printf("connecting to %q\n", dsn)
		db, err := sql.Open(wrongSrvCfg.Driver.Name, dsn)
		if err != nil {
			t.Fatalf("failed to open connection: %v", err)
		}
		defer db.Close()
		err = db.Ping()
		checkErrorRaised(t, err, ConnectFailed, InvalidServiceName)
	})

	t.Run("TestServerError_ORA-12520", func(t *testing.T) {

		t.Skipf("setting shared on local DB may end in looping indefinitively, waiting for a fix of that test")

		noHandlerCfg := TestingConfig.Clone()
		noHandlerCfg.Database.ServerType = "shared"

		dsn := noHandlerCfg.GetConnectionString()
		db, err := sql.Open(noHandlerCfg.Driver.Name, dsn)
		if err != nil {
			t.Fatalf("failed to open connection: %v", err)
		}
		defer db.Close()
		err = db.Ping()
		checkErrorRaised(t, err, ConnectFailed, NoAvailableHandler) // ORA-12520
	})
	t.Run("TestServerError_ORA-12521", func(t *testing.T) {

		noInstanceCfg := TestingConfig.Clone()
		noInstanceCfg.Database.InstanceName = "nonexistent"
		dsn := noInstanceCfg.GetConnectionString()

		db, err := sql.Open(noInstanceCfg.Driver.Name, dsn)
		if err != nil {
			t.Fatalf("failed to open connection: %v", err)
		}
		defer db.Close()
		err = db.Ping()
		checkErrorRaised(t, err, ConnectFailed, UnknownInstance) // ORA-12521
	})

	t.Run("TestServerError_ORA-12505", func(t *testing.T) {

		noSIDCfg := TestingConfig.Clone()
		noSIDCfg.Database.ServiceName = ""
		noSIDCfg.Database.SIDName = "nonexistent"
		dsn := noSIDCfg.GetConnectionString()

		db, err := sql.Open(noSIDCfg.Driver.Name, dsn)
		if err != nil {
			t.Fatalf("failed to open connection: %v", err)
		}
		defer db.Close()
		err = db.Ping()
		checkErrorRaised(t, err, ConnectFailed, InvalidSID) // ORA-12505
	})
	t.Run("TestServerError_ORA_12541", func(t *testing.T) {
		wrongPortCfg := TestingConfig.Clone()
		wrongPortCfg.Database.Port = 1700

		dsn := wrongPortCfg.GetConnectionString()
		db, err := sql.Open(wrongPortCfg.Driver.Name, dsn)
		if err != nil {
			t.Fatalf("failed to open connection: %v", err)
		}
		defer db.Close()
		err = db.Ping()
		checkErrorRaised(t, err, ConnectFailed, NoListenerAvailable) // ORA-12541
	})
}
