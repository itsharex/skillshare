# CLI E2E Runbook: Upgrade Auto-Sudo Detection

Verifies that `ss upgrade` detects non-writable binary locations and attempts `sudo` re-execution (Issue #105).

## Scope

- `needsSudo` correctly identifies writable vs non-writable directories
- `reexecWithSudo` invokes sudo with correct arguments
- `upgrade --skill --dry-run` still works (no regression)
- Unit tests pass inside container

## Environment

Run inside devcontainer. Requires the Linux binary to be built.

## Step 1: Run unit tests for needsSudo and reexecWithSudo

```bash
cd /workspace && go test ./cmd/skillshare/ -run "TestNeedsSudo|TestReexecWithSudo" -v -count=1
```

Expected:
- exit_code: 0
- PASS: TestNeedsSudo_WritableDir
- regex: (PASS|SKIP): TestNeedsSudo_NonWritableDir
- PASS: TestNeedsSudo_NonExistentDir
- PASS: TestReexecWithSudo_NoSudoInPath
- PASS: TestReexecWithSudo_ExecArgs

## Step 2: Upgrade skill dry-run (no regression)

```bash
ss upgrade --skill --dry-run --force
```

Expected:
- exit_code: 0
- regex: (Would download|Would re-download|Already up to date)

## Step 3: Create non-root user and verify needsSudo triggers

```bash
id testrunner 2>/dev/null || useradd -m -s /bin/bash testrunner
mkdir -p /opt/restricted-bin
cp /workspace/bin/skillshare /opt/restricted-bin/skillshare
chmod 755 /opt/restricted-bin/skillshare
chown root:root /opt/restricted-bin
chmod 755 /opt/restricted-bin
# Verify non-root cannot write to /opt/restricted-bin
su -s /bin/bash testrunner -c 'touch /opt/restricted-bin/.write-test 2>&1 || echo "write_denied=yes"'
```

Expected:
- exit_code: 0
- write_denied=yes

## Step 4: Non-root upgrade triggers sudo path

```bash
su -s /bin/bash testrunner -c '
  export HOME=/tmp/testrunner-home
  mkdir -p $HOME/.config $HOME/.local/share $HOME/.local/state $HOME/.cache
  /opt/restricted-bin/skillshare upgrade --cli --force 2>&1 || true
'
```

Expected:
- exit_code: 0
- Need elevated permissions

## Step 5: Writable location does NOT trigger sudo

```bash
mkdir -p /tmp/writable-bin
cp /workspace/bin/skillshare /tmp/writable-bin/skillshare
chmod 777 /tmp/writable-bin
su -s /bin/bash testrunner -c '
  export HOME=/tmp/testrunner-home2
  mkdir -p $HOME/.config $HOME/.local/share $HOME/.local/state $HOME/.cache
  /tmp/writable-bin/skillshare upgrade --cli --dry-run --force 2>&1
'
```

Expected:
- exit_code: 0
- Not Need elevated permissions

## Step 6: Cleanup

```bash
rm -rf /opt/restricted-bin /tmp/writable-bin /tmp/testrunner-home /tmp/testrunner-home2
userdel -r testrunner 2>/dev/null || true
```

Expected:
- exit_code: 0

## Pass/Fail Criteria

Pass when all are true:

- Unit tests for `needsSudo` and `reexecWithSudo` pass
- `upgrade --skill --dry-run` works without regression
- Non-root user sees "Need elevated permissions" for restricted binary location
- Non-root user does NOT see sudo message for writable binary location
