# FreeDB Release Process

## Version Alignment Rule

**The git tag, binary version, and migration version must always match.**

When you tag `v1.1`, the binary reports `freedb v1.1`, and the latest migration is `v1.1`. This keeps things simple for users — one version number everywhere.

## How to Ship a Release

### 1. Bundle migration changes into one script

All schema changes, script updates, and platform modifications for a release go into a single migration file:

```
tui/internal/upgrade/migrations/v1.1.sh
```

If you need multiple steps, put them all in that one file. Do not create intermediate migration versions (no `v1.1.1.sh`).

### 2. Register the migration

Add the new version to the migrations list in `tui/internal/upgrade/upgrade.go`:

```go
var migrations = []Migration{
    // ...existing...
    {Version: "v1.1", Script: "v1.1.sh"},
}
```

### 3. Update the version history

Add an entry to the version table in `README.md`:

```markdown
| v1.1 | Per-database backups, restore command, architecture detection |
```

### 4. Merge to main

Get the PR reviewed and merged.

### 5. Tag the release

```bash
git checkout main && git pull
git tag -a v1.1 -m "v1.1: Short description of changes"
git push origin v1.1
```

This triggers the GitHub Release workflow which builds binaries for linux/amd64, linux/arm64, and darwin/arm64.

### 6. Verify the release

- Check GitHub Releases for the binaries and checksums
- Download a binary and verify: `./freedb --version` shows `v1.1`
- Run `sudo freedb upgrade --dry-run` on a test instance — should show `v1.1` as pending

## During Development

Between releases, it's fine to have unreleased migration scripts on feature branches. But before tagging:

- Squash any intermediate migrations into a single file matching the release version
- Ensure the migration is idempotent (safe to re-run)
- Test the upgrade path from the previous release

## CI Enforcement

The CI workflow validates that the latest migration version matches the git tag on release builds. If you push a tag `v1.1` but the latest migration is `v1.0`, the build will fail.

## What Not to Do

- Don't create migration files without a corresponding release plan
- Don't tag a release without a migration (even if it's empty — create a no-op script)
- Don't auto-tag based on migration changes
- Don't reuse or amend existing migration scripts after they've been released
