# Driver Connection Lifecycle Under Load — P1

Owner: @yashgarg. Updated October 29, 2026.

## Purpose and current status

The network workload repeatedly constructs a connector, establishes a physical TCPS connection, authenticates, initializes the session, executes a validated query, and closes the connection. The purpose is to investigate accumulation of network/TLS objects, heap memory, goroutines, sockets, and Oracle sessions during sustained connection churn.

The lifecycle implementation uses one connection operation, a small worker runner, raw runtime samples, and ordinary checked file writes/closure. All lifecycle helpers, fakes, and database-free unit tests are consolidated in oracle/OracleDriver_connection_lifecycle_stress_test.go; the separate OracleDriver_direct_connection_test.go file has been removed after relocating its required symbols. This reduces navigation and makes ownership explicit; it does not by itself reduce algorithmic complexity. There are no stage callbacks, injectable session initializers, artifact-state framework, metadata subprocesses, or allocation-delta calculations. External collection remains separate.

The previous implementation completed a 16-worker Windows TCPS soak on October 8, 2026: 456,386 measured cycles over 24 hours, zero failures, all workers finished, matching client connection/query/closure counts, and finalized evidence. Preserve run cnx-2cb8a8b6ebe43d2d and its original schema unchanged. This is not a live result for the simplified implementation and does not independently prove absence of leaks. The simplified implementation requires its own baseline, smoke runs, and 24-hour soak.

The existing pooled resource-retention workload represents normal pool usage and remains a separate run. Direct Connector.Connect deterministically bypasses database/sql pooling. Reusing a connector also bypasses pooling; creating a connector every cycle extends coverage to connector construction and object release.

## One stress operation, with existing functional prerequisites

Each iteration performs:

1. Call a connector factory exactly once.
2. Call its Connector.Connect exactly once.
3. Verify SELECT SYS_CONTEXT('USERENV', 'NETWORK_PROTOCOL') FROM DUAL returns one tcps value and close its rows.
4. Set CLIENT_IDENTIFIER to the run ID and MODULE to go-connection-lifecycle-stress.
5. Execute SELECT 1 FROM DUAL on the same driver connection, consume exactly one value of 1, and close its rows.
6. Close the driver connection, release local references, and repeat.

The helper owns connection and row cleanup, including after initialization/query failures. Primary and cleanup errors are joined. Per-cycle defers are confined to the helper and finish before another iteration starts. Connector has no standard Close method; connector objects become eligible for GC when references are released.

Session initialization occurs after the raw Connect succeeds, so a later protocol/tagging failure does not hide an opened connection from the returned result. Each protocol query's rows are independently closed. The row_closes workload counter describes the representative SELECT's rows. Unexpected panics use Go's normal stack reporting, not panic-to-error wrappers.

Use the existing TestDriver_SimpleConnection and TestDriver_Functional_SelectDual tests as functional prerequisites with the selected TCPS profile. They validate basic connection/query behavior through database/sql; they do not execute the exact direct-Connector cycle, protocol-verification query, or session tagging. The lifecycle smoke test covers that combined stress operation. The additional TestDriver_DirectConnectionLifecycle functional test and its registration have been removed; the direct-cycle helper and database-free stress tests remain.

TestDriver_TCPS_Pipeline_SelectDual provides existing TCPS functional coverage when its dedicated pipeline_test_tcps configuration, wallet, and certificate DN are available. It does not use an arbitrary selected profile, so a skipped pipeline test is not TCPS evidence. BenchmarkSimpleOpen demonstrates direct non-pooled opening but is a benchmark, not a functional prerequisite or sustained soak test. The stress entry point continues to reject TCP.

## Environment prerequisites

- A valid TCPS listener: host, actual TLS port, service, and valid credentials.
- A configured wallet/trust store and expected certificate name or DN.
- Explicit ssl_server_dn_match=on; weak DN matching disabled.
- A dedicated workload account with SELECT access and permission to identify its own sessions through DBMS_SESSION/DBMS_APPLICATION_INFO.
- A separate monitoring account with DBA-approved access to the required GV$ views in the workload PDB.
- A stable network/VPN route throughout the normal workload.
- Go version compatible with go.mod; Python 3 and ps/lsof for the external macOS collector, Python 3 and /proc for Linux, and PowerShell/Get-NetTCPConnection on Windows.

Use the disabled, credential-free JSON template in Appendix A below to create a private test-config-tcps.json. Replace every placeholder, confirm the actual listener port (2484 is only an example), then set enabled=true. The private filename is ignored by Git. Do not commit credentials or wallets.

The driver verifies trust and certificate matching during connection establishment; server-reported NETWORK_PROTOCOL is checked on every successfully connected cycle. This test does not expose TLS cipher/version information through a new production API.

## Configuration and profiles

All settings remain under stress.connection_lifecycle in the selected JSON entry. Existing fixed-count profiles remain valid.

| Field | Meaning | Default/rule |
| --- | --- | --- |
| workers | Concurrent workers | Required positive integer |
| cycles_per_worker | Fixed cycles per worker | Positive; mutually exclusive with duration |
| duration | Time admitting new cycles | Positive Go duration; mutually exclusive with cycles_per_worker |
| timeout | Overall safety deadline, including warm-up and observation | Required positive duration |
| operation_timeout | Context deadline covering one cycle | 2m |
| sample_interval | Natural runtime sampling interval | 1m |
| output_directory | Evidence root | stress-results |
| diagnostic_profiles | Heap endpoints and failure goroutine dump | true when omitted; explicit false supported |

Lifecycle resolves output_directory to an absolute path on a local configuration copy. The selected JSON entry and resource-retention settings are not modified. A relative directory is relative to the test process's working directory, not necessarily the invoking terminal's directory. With ordinary go test ./oracle execution, the default is normally <checkout>/oracle/stress-results. Use the absolute samples path printed at startup and its parent directory for external collectors; do not assume <checkout>/stress-results. An explicitly absolute output_directory remains absolute.

Duration-mode timeout must exceed duration + operation_timeout + 15m. This headroom covers warm-up, final drain, profile writes and external observation. Go's test timeout must exceed the JSON timeout; the stress entry point checks this before running.

The agreed progression is:

| Profile | Workers | Measured execution | JSON timeout | Go timeout |
| --- | ---: | --- | --- | --- |
| Initial smoke | 1 | 16 cycles/worker | 10m | 15m |
| Concurrent smoke | 16 | 16 cycles/worker | 15m | 20m |
| First soak | 16 | 24h | 26h | 27h |
| Extended soak | 16 | 48h | 50h | 51h |

Example 24-hour profile:

```json
"connection_lifecycle": {
  "workers": 16,
  "duration": "24h",
  "timeout": "26h",
  "operation_timeout": "2m",
  "sample_interval": "1m",
  "output_directory": "stress-results",
  "diagnostic_profiles": true
}
```

For 48 hours use duration=48h and timeout=50h. Remove cycles_per_worker when switching to duration mode. Increasing only Go's -timeout does not extend the workload.

## Execution

Run from the repository root. Record the driver commit, local source changes, database version, client machine/Go version, and network/VPN conditions in the execution notes before starting. The workload no longer launches platform-specific subprocesses to discover these details. Confirm that the route remains stable throughout the run.

First run the existing connection and SELECT functional tests against the populated TCPS profile:

```bash
go test -count=1 -timeout=3m -v ./oracle \
  -run '^TestDriver_(SimpleConnection|Functional_SelectDual)$' \
  -args \
  -driver.config.filename="$PWD/test-config-tcps.json" \
  -driver.config.name=phoenix_tcps
```

With the initial smoke JSON profile, run:

```bash
go test -count=1 -timeout=15m -v ./oracle \
  -run '^TestCategoryExecutor$/^TestDriver_ConnectionLifecycleStress$' \
  -args \
  -test.category=stress \
  -driver.config.filename="$PWD/test-config-tcps.json" \
  -driver.config.name=phoenix_tcps
```

For the 24-hour JSON profile use the same command with -timeout=27h; for 48 hours use -timeout=51h. The test remains manually selected and exclusive; it has no Go-discoverable stress function and is not added to the weekly workflow. A missing lifecycle profile is an explicit skip.

On Windows PowerShell, quote native arguments using an array (including dotted flags). Set the wallet-password environment variable in the same session first, and use a Windows wallet path in the private JSON:

```powershell
$configPath = (Resolve-Path .\test-config-tcps.json).Path
$soakArgs = @(
    'test'
    '-count=1'
    '-timeout=27h'
    '-v'
    './oracle'
    '-run=^TestCategoryExecutor$/^TestDriver_ConnectionLifecycleStress$'
    '-args'
    '-test.category=stress'
    "-driver.config.filename=$configPath"
    '-driver.config.name=phoenix_tcps'
)
& go @soakArgs
if ($LASTEXITCODE -ne 0) { throw 'Lifecycle soak failed' }
```

For the existing functional prerequisites use the quoted argument '-run=^TestDriver_(SimpleConnection|Functional_SelectDual)$' and -timeout=3m without the category argument. For a smoke run keep the stress selector, use -timeout=15m or 20m, and select the corresponding fixed-count JSON profile. Replace the example config filename/name with the selected private profile.

For investigations, preserve a compiled test binary using go test -c -o <chosen-binary-path> ./oracle and execute that exact binary. A directly invoked binary takes -test.v, -test.timeout and -test.run flags rather than go test's -v/-timeout/-run, and has no -args separator. A later go test invocation may build a different executable; do not treat a separately compiled binary as proof of which executable ran.

Record the binary's hash and build information before execution. On macOS/Linux use shasum -a 256 <binary> and go version -m <binary>. On Windows use Get-FileHash -Algorithm SHA256 -LiteralPath <binary> and go version -m <binary>. Keep build output, the build-time source revision, and the local source patch alongside the executable. A revision observed in the checkout later is checkout metadata, not necessarily executable provenance.

## Simplified implementation

The existing connection-lifecycle stress-test file contains the manual entry point, direct-cycle helper, worker runner, runtime output functions, and focused database-free tests in marked sections. No symbols or harness coverage are discarded by consolidation. Basic functional coverage is reused from the existing suite; focused harness tests do not add functional-product workloads. No new framework or repository files are introduced. Configuration is validated/normalized once at the entry point. One overall safety context bounds warm-up, work, drain, and observation; per-cycle contexts bound operations. The sampler is stopped and joined before endpoint writes. Rows and connections use separate checked defers confined to one cycle.

Shared retention helpers and fake-driver contracts are unchanged. The independent pool-based retention test retains its own configuration and output semantics. Run both database-free suites under the race detector, configuration tests, package compilation, and Windows cross-compilation after lifecycle edits. Run the existing functional prerequisites with a TCPS profile, followed by lifecycle smoke profiles, before another 24-hour soak.

## Admission, failure, and timing

Workers synchronize their first attempt once, then run independently. The duration clock starts when their barrier is released. At duration expiry, workers stop starting cycles and let active cycles finish. Warm-up and observation are excluded from active workload time.

One warm-up cycle per worker establishes initial driver/TLS state. Its counters are stored separately from measured counters. A warm-up failure stops execution.

Unexpected errors stop admission and cancel peers. Each worker stores at most one final cycle error in its own slot; collection is bounded by worker count, not operation count. All workers are joined before the slots are read or final checkpoints begin. The primary cancellation cause is reported first, then independent failures in worker-index order. Cancellation-only followers and already-contained causes are not repeated. Joined cleanup failures remain visible, including Close errors wrapping cancellation. Unknown mixed wrapper errors are kept intact rather than dropping independent details or error types. Errors retain worker/cycle/stage context and errors.Is/errors.As access. Failed-cycle counters include unsuccessful cancelled followers even when their redundant diagnostics are suppressed.

Context deadlines are cooperative: if a driver call or Close ignores cancellation, Go's overall timeout provides the final bound and goroutine stack dump.

The final result records workload_elapsed_ns for worker execution through joining, including drain time. For a successful duration run, active_ns is the configured admission duration and drain_ns is the excess worker elapsed time. For a successful fixed-count run, active_ns equals worker elapsed time and drain_ns is zero. For failed/not-started workloads, active_ns and drain_ns are null: early failure and cancellation cleanup do not support that precise split. Terminal output labels them unavailable.

observation_ns records actual time spent in the drained observation wait; observation_completed is true only when its requested timer completes. A skipped observation has zero duration and false completion. The result record's elapsed_ns is time from run initialization to record writing, before that record and file Sync/Close complete. Terminal wall time is captured after file cleanup and failure diagnostics. No throughput or latency pass/fail threshold is set. Network/Oracle failures fail this normal run and must be investigated using the recorded environment.

## Measurements and artifacts

Each unique cnx- run directory contains samples.jsonl, endpoint heap profiles when enabled, and a failure-goroutines.txt dump on an unexpected failure. Earlier evidence is never overwritten. OS and Oracle collectors produce separate evidence files.

Lifecycle counters are: cycles_started, connection_attempts, successful_connections, query_completions, row_closes, connection_closes, completed_cycles, failed_cycles, and workers_finished. Connector construction, protocol verification, and session tagging remain mandatory but no longer have redundant counters.

cycles_started increments before calling the helper. Other per-cycle totals are merged once that helper and its cleanup return, including on error. Thus periodic stage totals describe returned cycles, not every resource currently in flight; up to workers cycles can be in progress. After workers finish, every admitted normal cycle must complete its mandatory operation and cleanup, and attempts/connections/queries/row closes/connection closes must equal completed_cycles. All workers must finish and failed_cycles must be zero. Warm-up totals are separate.

Those equalities apply to successful runs. On failure, stage counts can differ legitimately: for example, a non-nil partial connection returned together with a Connect error is closed, but is not counted as a successful connection. Protocol verification and session-tagging failures retain the successful connection/close counts while preventing the representative query from running.

The runtime stream records HeapAlloc, HeapObjects, HeapInuse, HeapSys, HeapReleased, StackInuse, TotalAlloc, Mallocs, Frees, NumGC, PauseTotalNs, GCCPUFraction, and goroutines. It retains only current counters and streams samples directly to disk; allocation/GC deltas are derived afterward if needed. No per-cycle terminal logging is performed. Common sample fields remain phase, run_id, pid, timestamp_utc, elapsed_ns, counts, memory, and goroutines.

Checkpoint sequence:

1. Warm-up finishes and all its resources close.
2. baseline_natural, then baseline_post_gc and baseline-heap.pprof.
3. Continuous natural samples every configured interval.
4. All measured workers finish and the sampler is stopped and joined.
5. final_natural, then final_post_gc and final-heap.pprof.
6. observation_start; keep the test process visible for two minutes without new connections.
7. settled_post_gc and settled-heap.pprof.
8. Write one flat workload result with counts, separate warmup_counts, workload_elapsed_ns, active_ns/drain_ns (or null), observation_ns/observation_completed, result-write elapsed_ns, and a redacted error. Synchronize and close samples.jsonl, checking both errors, before returning to the terminal summary and Go PASS/FAIL. On an unexpected error, join workers and attempt endpoint diagnostics without the normal observation delay.

Only the drained endpoints force GC. Periodic workload samples observe natural behavior. The workload does not change GC percentage, memory limits, or memory profiling rate; record custom settings in the execution notes. Profile writing and reporting create their own temporary allocations, which must be considered when reviewing endpoints.

The configuration record contains run ID, PID, UTC start time, Go version, OS, architecture, selected profile name, and resolved workload settings. executable_path comes from os.Executable (or is unavailable). Its build object is explicitly sourced from runtime/debug.ReadBuildInfo and contains only allowlisted package/module identification and embedded vcs.revision, vcs.time, and vcs.modified settings. Missing revision/time are unavailable and unknown modified state is null. Module version (devel) is not a revision. A test binary may have no VCS stamp; the current checkout is never used as a fallback. Even a stamped modified=true build does not identify the local patch; preserve that patch separately.

The record does not serialize database credentials, the full database configuration, or unrestricted build flags. No Git/OS metadata subprocess is launched. Record environment, database version, external monitoring availability, and stronger binary/source attribution in the execution notes. Credentials, DSNs, and wallet locations are redacted from saved and terminal errors.

Inspect retained allocations with a matching binary, for example:

```bash
go tool pprof -base <absolute-run-directory>/baseline-heap.pprof <test-binary> <absolute-run-directory>/settled-heap.pprof
```

## External collectors

Start Oracle monitoring before workload admission so the dedicated account/service has a baseline. Start OS sampling as soon as the test prints its run ID and PID. Use the test executable PID, not the parent go/terminal PID. Collect through the drained observation period.

### Linux and macOS

```bash
python3 '/absolute/local/collect-posix-resources.py' \
  --pid <test-process-pid> \
  --run-id <cnx-run-id> \
  --output-file <absolute-run-directory>/os.jsonl \
  --interval-seconds 60
```

Save Appendix B outside the checkout before invoking that local file. The manual collector records RSS, numeric process file descriptors, process-owned sockets, TCP states, timestamps and process identity. Linux filters namespace TCP entries by owned socket inode. macOS uses ps/lsof and ignores cwd/text inventory entries as descriptors. Unavailable values are null with explicit status; they are not zero. The collector refuses PID reuse and never overwrites an output file.

### Windows

```powershell
& 'C:\StressMonitoring\collect-windows-resources.ps1' -TargetProcessId <test-process-pid> -RunId <cnx-run-id> -OutputFile "<absolute-run-directory>/os.csv" -IntervalSeconds 60
```

Save the PowerShell snippet from Appendix B of resource-retention-memory-leak-stress-test.md outside the checkout before invoking it. The manual Windows collector records working set, private memory, process handles and process-attributed TCP states. Handle count is not a POSIX descriptor count. OS-level TIME_WAIT entries can lose PID/inode ownership; process-owned counts cannot inventory all historical sockets.

### Oracle

Execute the self-contained read-only SQL in Appendix C from a separate monitoring account in the workload PDB. Set these variables first:

```sql
DEFINE lifecycle_run_id = 'cnx-id-printed-by-test'
DEFINE lifecycle_username = 'DEDICATED_WORKLOAD_USER'
DEFINE lifecycle_service = 'ACTUAL_SERVICE_NAME'
-- Then execute Appendix C below in this same session.
```

Schedule/export these read-only queries every 60 seconds through the DBA's approved tool. Preserve an account/service baseline before the run and explicit post-run zero rows. The appendix includes run-scoped RAC/PDB session/cursor/PGA inventories and adds account/service counts and untagged/other-session attribution; no external SQL include is required. This catches sessions opened before run identification succeeds.

A dedicated account is required for account/service attribution; sharing it with other applications makes leak sign-off incomplete. Monitoring failures are not healthy zeros. Do not count the monitoring connection as part of the workload.

## Acceptance and next execution

The Go workload passes only when all admitted cycles succeed, every worker finishes, resource counters agree, and runtime evidence is written. Query, initialization, closure, panic, timeout and evidence-write errors fail the run.

The final JSON record reports workload_status as not_started (for example, warm-up failed), complete, or failed. It describes measured cycles, not successful observation, reporting, file finalization, or a leak verdict. A completed workload followed by an observation, profile, write, Sync, or Close failure remains workload_status=complete but the Go test fails. Later observation errors are stage-labelled; reporting errors remain in the returned diagnostic. There are no artifact_status or run_status state machines.

The authoritative overall outcome is Go PASS/FAIL and the executable exit code, emitted after file cleanup. A nonzero exit code always means the run failed regardless of workload_status. The last JSON record cannot certify its own subsequent write/Sync/Close success, and can itself be missing after a write failure. Preserve terminal output and exit status alongside the artifacts. Missing terminal/exit evidence is incomplete evidence, not a pass.

Leak assessment remains a separate manual review of runtime, heap profiles, external OS/socket evidence, and scoped Oracle sessions. Keep an assessment.md beside the evidence:

| Assessment | Meaning |
| --- | --- |
| Incomplete evidence | Required external data, scope, continuity or attribution is missing |
| Investigation required | Unexplained retained heap/object, goroutine, socket or session accumulation |
| No accumulation observed | Reviewed evidence remains bounded and resources settle after work |

TotalAlloc/NumGC naturally increase; HeapSys can remain high; TIME_WAIT and server monitoring delays need interpretation. One larger final heap value does not identify a driver leak. Use reproducible observations and heap/goroutine profiles to investigate ownership before assigning a defect.

Verification order: database-free helper/config/runner/recorder checks and targeted race checks, existing connection/SELECT functional tests with the selected TCPS profile, single-worker lifecycle smoke, 16-worker smoke with collectors, 16-worker 24-hour soak, evidence review, then 16-worker 48-hour soak. Keep the normal pool workload and fault-injection/recovery coverage as separate invocations.

Live execution prerequisites still needed: actual TCPS listener/trust settings, isolated workload account, monitoring permissions and a stable client route. The disabled template is a starting point, not a tested connection configuration.

## Shared dependency ownership

Lifecycle owns its direct-cycle helper, worker runner, measurements, unit tests and this document. Retention owns its pooled workload implementation. The following current dependencies remain shared contracts, despite retention-oriented names:

- validateRetentionTLS: TCPS required, explicit certificate-name matching, weak matching disabled.
- newRetentionRunID: unique run-ID generation; lifecycle retains its distinct cnx- prefix.
- writeRetentionProfile and redactRetentionText: profile format/no-overwrite/error handling and error-secret redaction.
- retentionFakeConnector, its connection/rows/driver fakes and hooks: shared database-free verification infrastructure.
- Manual Oracle monitoring query contract: run-scoped RAC/PDB resource inventories and explicit zero session counts; the SQL is preserved in the documentation rather than an included repository file.
- Test configuration clone/merge and category registration: preserve both independently selected workloads.

Lifecycle-local path and output simplification do not change these contracts, retention configuration, checkpoint policy or output semantics. Coordinate narrowly scoped changes to shared functions; do not revert combined files or replace lifecycle initialization with a retention-specific connector wrapper.


## Manual external setup — 2026-10-06

The repository scripts directory has been removed without changing lifecycle Go
code, counters, configuration or runtime measurements. Execute the procedures
below manually using approved local/DBA tooling. Keep private scripts outside the
checkout. External collection is not automatic; unavailable collectors must be
reported as missing evidence. Removed originals and the POSIX collector unit tests
were backed up outside the repository before removal.

### Appendix A — Private TCPS configuration template

```json
[
  {
    "config_name": "phoenix_tcps",
    "enabled": false,
    "database_version": "0.0.0",
    "driver": { "name": "oracledb" },
    "database": {
      "host": "REPLACE_WITH_TCPS_HOST",
      "port": 2484,
      "serviceName": "REPLACE_WITH_SERVICE_NAME",
      "protocol": "tcps"
    },
    "credentials": {
      "username": "REPLACE_WITH_DEDICATED_WORKLOAD_USER",
      "password": "REPLACE_LOCALLY",
      "logonMode": ""
    },
    "security": {
      "wallet_location": "REPLACE_WITH_LOCAL_WALLET_PATH",
      "ssl_server_dn_match": "on",
      "ssl_server_cert_dn": "REPLACE_WITH_EXPECTED_SERVER_CERTIFICATE_DN",
      "ssl_allow_weak_dn_match": "off"
    },
    "stress": {
      "connection_lifecycle": {
        "workers": 1,
        "cycles_per_worker": 16,
        "timeout": "10m",
        "operation_timeout": "2m",
        "sample_interval": "1m",
        "output_directory": "stress-results",
        "diagnostic_profiles": true
      }
    }
  }
]
```

### Appendix B — Manual Linux/macOS collection

Save this snippet outside the repository, then use the invocation above with the
actual executable PID and an absolute artifact path. Keep Python 3 and platform
utilities available. The collector stops when the process exits or changes
identity. Tests for this standalone collector are no longer part of the repository;
validate it in the execution environment before relying on its evidence.

```python
#!/usr/bin/env python3
# Copyright (c) 2026 Oracle and/or its affiliates.
# The Universal Permissive License (UPL), Version 1.0
"""External Linux/macOS collector. Run against the test binary's PID.

Only current process-owned sockets are attributed to the PID. Kernel TIME_WAIT
entries can outlive their process/inode; these counts are not a host inventory.
Unavailable measurements are null with a status, never a fabricated zero.
"""

import argparse
import datetime
import json
import os
import pathlib
import re
import subprocess
import sys
import time


def utc_now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def command(*args):
    result = subprocess.run(args, capture_output=True, text=True, timeout=10,
                            env=dict(os.environ, LC_ALL="C", TZ="UTC"))
    if result.returncode:
        raise RuntimeError("collector command failed: " + args[0])
    return result.stdout.strip()


def linux_identity(pid):
    # comm can contain spaces and parentheses. Fields after its closing ')' start
    # at state (field 3), so starttime (field 22) is element 19.
    stat = pathlib.Path("/proc", str(pid), "stat").read_text()
    return stat.rsplit(")", 1)[1].split()[19]


def mac_identity(pid):
    value = command("ps", "-p", str(pid), "-o", "lstart=")
    if not value:
        raise ProcessLookupError(pid)
    return value


def parse_linux_tcp(text, owned_inodes):
    counts = {"tcp_established": 0, "tcp_close_wait": 0,
              "tcp_time_wait": 0, "tcp_other": 0}
    states = {"01": "tcp_established", "08": "tcp_close_wait", "06": "tcp_time_wait"}
    for line in text.splitlines()[1:]:
        fields = line.split()
        if len(fields) > 9 and fields[9] in owned_inodes:
            counts[states.get(fields[3], "tcp_other")] += 1
    return counts


def parse_lsof(text):
    descriptors, sockets = set(), set()
    counts = {"tcp_established": 0, "tcp_close_wait": 0,
              "tcp_time_wait": 0, "tcp_other": 0}
    states = {"ESTABLISHED": "tcp_established", "CLOSE_WAIT": "tcp_close_wait",
              "TIME_WAIT": "tcp_time_wait"}
    fd, kind, state = None, None, None

    def finish():
        if fd is not None:
            descriptors.add(fd)
            if kind in ("IPv4", "IPv6", "unix"):
                sockets.add(fd)
            # UDP/unix sockets have no TCP state and are not TCP counts.
            if kind in ("IPv4", "IPv6") and state is not None:
                counts[states.get(state, "tcp_other")] += 1

    for line in text.splitlines():
        if line.startswith("f"):
            finish()
            fd = line[1:] if line[1:].isdigit() else None
            kind, state = None, None
        elif line.startswith("t"):
            kind = line[1:]
        elif line.startswith("TST="):
            state = line[4:]
    finish()
    return dict(counts, open_file_descriptors=len(descriptors), open_sockets=len(sockets))


def unavailable(sample, name, error):
    sample[name + "_status"] = "unavailable"
    sample.setdefault("errors", []).append(type(error).__name__ + ": " + name)


def linux_sample(pid, identity):
    root = pathlib.Path("/proc", str(pid))
    boot = next(line.split()[1] for line in pathlib.Path("/proc/stat").read_text().splitlines()
                if line.startswith("btime "))
    start = float(boot) + int(identity) / os.sysconf("SC_CLK_TCK")
    sample = {"process_start_utc": datetime.datetime.fromtimestamp(start, datetime.timezone.utc).isoformat(),
              "rss_bytes": None, "open_file_descriptors": None, "open_sockets": None,
              "tcp_established": None, "tcp_close_wait": None, "tcp_time_wait": None, "tcp_other": None}
    try:
        sample["rss_bytes"] = int((root / "statm").read_text().split()[1]) * os.sysconf("SC_PAGE_SIZE")
        sample["rss_status"] = "available"
    except (OSError, ValueError) as error:
        unavailable(sample, "rss", error)
    try:
        entries = list((root / "fd").iterdir())
        owned_inodes = set()
        for entry in entries:
            try:
                match = re.fullmatch(r"socket:\[(\d+)\]", os.readlink(entry))
                if match:
                    owned_inodes.add(match.group(1))
            except FileNotFoundError:
                pass  # Connection closure raced with this read.
        sample.update(open_file_descriptors=len(entries), open_sockets=len(owned_inodes),
                      fd_status="available", socket_status="available")
        totals = {key: 0 for key in ("tcp_established", "tcp_close_wait", "tcp_time_wait", "tcp_other")}
        for table in ("tcp", "tcp6"):
            values = parse_linux_tcp((root / "net" / table).read_text(), owned_inodes)
            for key in totals:
                totals[key] += values[key]
        sample.update(totals, tcp_status="available")
    except (OSError, ValueError) as error:
        unavailable(sample, "tcp", error)
        if sample["open_sockets"] is None:
            unavailable(sample, "socket", error)
            unavailable(sample, "fd", error)
    return sample


def mac_sample(pid, identity):
    start = datetime.datetime.strptime(identity, "%a %b %d %H:%M:%S %Y").replace(tzinfo=datetime.timezone.utc)
    sample = {"process_start_utc": start.isoformat(), "process_start_precision": "1s",
              "rss_bytes": None, "open_file_descriptors": None, "open_sockets": None,
              "tcp_established": None, "tcp_close_wait": None, "tcp_time_wait": None, "tcp_other": None}
    try:
        sample["rss_bytes"] = int(command("ps", "-p", str(pid), "-o", "rss=")) * 1024
        sample["rss_status"] = "available"
    except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
        unavailable(sample, "rss", error)
    try:
        values = parse_lsof(command("lsof", "-a", "-p", str(pid), "-nP", "-FftT"))
        sample.update(values, fd_status="available", socket_status="available", tcp_status="available")
    except (OSError, RuntimeError, subprocess.TimeoutExpired) as error:
        for name in ("fd", "socket", "tcp"):
            unavailable(sample, name, error)
    return sample


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pid", type=int, required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output-file", type=pathlib.Path, required=True)
    parser.add_argument("--interval-seconds", type=float, default=60)
    parser.add_argument("--samples", type=int, default=0, help="0 collects until the process exits")
    args = parser.parse_args()
    if args.pid <= 0 or not 0 < args.interval_seconds <= 86400 or args.samples < 0:
        parser.error("invalid PID, interval, or sample count")
    if sys.platform == "linux":
        identity_fn, sample_fn = linux_identity, linux_sample
    elif sys.platform == "darwin":
        identity_fn, sample_fn = mac_identity, mac_sample
    else:
        parser.error("use the Windows PowerShell collector on Windows")
    expected = identity_fn(args.pid)
    args.output_file.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    fd = os.open(args.output_file, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as output:
        index = 0
        while True:
            try:
                if identity_fn(args.pid) != expected:
                    raise RuntimeError("PID reused; refusing to sample another process")
                sample = sample_fn(args.pid, expected)
                if identity_fn(args.pid) != expected:
                    raise RuntimeError("PID reused during collection")
            except (ProcessLookupError, FileNotFoundError):
                break
            except RuntimeError:
                # ps returns nonzero after process exit. A live process with a
                # different identity must still fail instead of silently stopping.
                try:
                    os.kill(args.pid, 0)
                except ProcessLookupError:
                    break
                raise
            sample.update(run_id=args.run_id, pid=args.pid, timestamp_utc=utc_now(),
                          socket_scope="process_owned; excludes detached TIME_WAIT")
            output.write(json.dumps(sample) + "\n")
            output.flush()
            index += 1
            if args.samples and index >= args.samples:
                break
            time.sleep(args.interval_seconds)


if __name__ == "__main__":
    main()
```

### Appendix C — Manual lifecycle Oracle collection

Set lifecycle_run_id, lifecycle_username and lifecycle_service as shown above.
Execute this complete block in the same SQL*Plus/SQLcl session; it has no external
include. Schedule/export through the approved DBA tool every 60 seconds, including
before admission and through the final observation period. The monitoring account
must be distinct from the dedicated workload account.

```sql
-- Copyright (c) 2026 Oracle and/or its affiliates.
-- The Universal Permissive License (UPL), Version 1.0
-- Run through a separate DBA-approved monitoring account in the workload PDB.
-- Define lifecycle_run_id, lifecycle_username and lifecycle_service first.
-- Schedule this read-only script every 60s, before workload admission and during
-- the final observation period. Save timestamps and explicit zero rows.
-- The username/service scope requires a dedicated workload account: it catches
-- sessions leaked before client_identifier is set. Monitor with another user.
WHENEVER SQLERROR EXIT FAILURE
WHENEVER OSERROR EXIT FAILURE
SET VERIFY OFF
SET LINESIZE 240
SET PAGESIZE 50000

-- Reuse the run-scoped RAC/PDB session, cursor and PGA inventories.
DEFINE retention_run_id = '&&lifecycle_run_id'
-- Optional read-only DBA collector. Run in the workload PDB through a separate
-- account/connection with DBA-approved grants on the required GV$ views.
-- SQL*Plus/SQLcl: DEFINE retention_run_id = 'ret-<id-from-samples.jsonl>'
-- Sample every 60s using the DBA's approved scheduling/export tool. Include
-- samples before/after pool shutdown. Monitor connection is not tagged.
-- GV$ keys include INST_ID and CON_ID; a RAC service can span instances.
-- No workload grants, DDL, session killing, or setup commands are performed here.
SET LINESIZE 240
SET PAGESIZE 50000

SELECT SYS_EXTRACT_UTC(SYSTIMESTAMP) timestamp_utc,
       s.client_identifier run_id, s.inst_id, s.con_id, s.sid, s.serial#,
       s.username, s.service_name, s.status, s.server,
       MAX(CASE WHEN n.name='opened cursors current' THEN st.value END) opened_cursors_current,
       MAX(CASE WHEN n.name='session pga memory' THEN st.value END) session_pga_bytes,
       MAX(CASE WHEN n.name='session pga memory max' THEN st.value END) session_pga_max_bytes
FROM gv$session s
LEFT JOIN gv$sesstat st ON st.inst_id=s.inst_id AND st.sid=s.sid AND st.con_id=s.con_id
LEFT JOIN gv$statname n ON n.inst_id=st.inst_id AND n.statistic#=st.statistic# AND n.con_id IN (0,st.con_id)
WHERE s.client_identifier='&&retention_run_id'
GROUP BY s.client_identifier,s.inst_id,s.con_id,s.sid,s.serial#,
         s.username,s.service_name,s.status,s.server;

-- Includes cached cursors: inventory is NOT itself an active-leak count.
SELECT SYS_EXTRACT_UTC(SYSTIMESTAMP) timestamp_utc,
       s.client_identifier run_id,s.inst_id,s.con_id,s.sid,s.serial#,
       c.cursor_type, COUNT(*) cursor_entries
FROM gv$session s JOIN gv$open_cursor c
  ON c.inst_id=s.inst_id AND c.sid=s.sid AND c.con_id=s.con_id
WHERE s.client_identifier='&&retention_run_id'
GROUP BY s.client_identifier,s.inst_id,s.con_id,s.sid,s.serial#,c.cursor_type;

-- Emit an explicit zero after sessions vanish; a missing detail row alone is
-- not evidence that collection is healthy or sessions have disappeared.
SELECT SYS_EXTRACT_UTC(SYSTIMESTAMP) timestamp_utc,
       '&&retention_run_id' run_id,COUNT(*) workload_sessions
FROM gv$session WHERE client_identifier='&&retention_run_id';

-- Do not attribute an entire GV$PROCESS PGA to one shared-server session.
-- Session PGA counters above remain supporting server-side observations.

SELECT SYS_EXTRACT_UTC(SYSTIMESTAMP) timestamp_utc,
       '&&lifecycle_run_id' run_id,
       UPPER('&&lifecycle_username') workload_username,
       '&&lifecycle_service' workload_service,
       COUNT(*) account_service_sessions,
       COALESCE(SUM(CASE WHEN client_identifier='&&lifecycle_run_id' THEN 1 ELSE 0 END),0) tagged_sessions,
       COALESCE(SUM(CASE WHEN client_identifier IS NULL OR client_identifier<>'&&lifecycle_run_id' THEN 1 ELSE 0 END),0) untagged_or_other_sessions
FROM gv$session
WHERE username=UPPER('&&lifecycle_username')
  AND service_name='&&lifecycle_service';

-- Attribution evidence for account/service sessions that have not been tagged.
SELECT SYS_EXTRACT_UTC(SYSTIMESTAMP) timestamp_utc,
       '&&lifecycle_run_id' run_id,inst_id,con_id,sid,serial#,
       client_identifier,module,machine,program,status
FROM gv$session
WHERE username=UPPER('&&lifecycle_username')
  AND service_name='&&lifecycle_service'
  AND (client_identifier IS NULL OR client_identifier<>'&&lifecycle_run_id');
```
