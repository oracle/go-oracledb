# Oracle `database/sql` compatibility reproductions

These tests isolate the two failures seen in the `go-sql-test` compatibility
suite:

- `TestManyQueryRow` repeats the same `QueryRow(...).Scan(...)` call 10,000
  times.
- `TestPreparedStmt` runs 10 goroutines, each reusing a prepared SELECT and
  INSERT statement for 10 iterations.

From the repository root, run either reproduction on its own:

```sh
source <(sed -n '10,28p' ../node-oracledb/config-iam.sh)
go test ./compat-tests -run '^TestManyQueryRow$' -v -timeout 5m
go test ./compat-tests -run '^TestPreparedStmt$' -v -timeout 5m
```

By default the tests load `../test-config.json` from this package directory and
use the `iam-tcps` profile. Set `GO_ORACLEDB_TEST_CONFIG` and
`GO_ORACLEDB_TEST_CONFIG_NAME` to select another JSON file or profile. For the
TCPS profile, load the environment assignment block from the local IAM config
as shown above. The test maps `NODE_ORACLEDB_WALLET_PASSWORD` to the Go driver's
expected variable without printing its value.

If a hard Go test timeout prevents deferred cleanup, remove the two test tables
with:

```sh
go test ./compat-tests -run '^TestCleanupArtifacts$' -v -timeout 1m
```
