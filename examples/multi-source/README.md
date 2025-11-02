# Multi-Source Example

This example demonstrates datum's multi-source functionality, which allows you to specify multiple data sources with automatic fallback support.

## Overview

When you specify multiple sources for a dataset, datum will try them in order:
1. Attempts to fetch/verify from the first source
2. If the first source fails, automatically falls back to the second source
3. Continues through all sources until one succeeds
4. Applies the configured policy only after all sources have been tried

This is useful for:
- **High availability**: Maintain access to data even when primary sources are down
- **Geographic redundancy**: Try different mirrors or regional endpoints
- **Development workflows**: Fall back to local cached copies when offline
- **Graceful degradation**: Use alternative tools or methods as fallbacks

## Configuration

The key difference from single-source configurations is using `sources:` (plural) instead of `source:`:

```yaml
datasets:
  - id: my_data
    sources:                    # Note: "sources" (plural)
      - type: http              # Primary source
        url: https://primary.example.com/data.csv
      - type: http              # Backup source
        url: https://backup.example.com/data.csv
    target: data/my_data.csv
```

## Examples in this Directory

### 1. HTTP with Backup Mirror
```yaml
sources:
  - type: http
    url: https://www.cdc.gov/growthcharts/data/zscore/wtage.csv
  - type: http
    url: https://example.com/backup/wtage.csv
```

If the primary CDC server is unavailable, datum automatically tries the backup URL.

### 2. Remote with Local Fallback
```yaml
sources:
  - type: http
    url: https://config.example.com/app-config.json
  - type: file
    path: ./backups/app-config.json
```

Useful for development: tries to fetch fresh config from server, but falls back to local cache if offline.

### 3. Multiple Mirror Servers
```yaml
sources:
  - type: http
    url: https://mirror1.example.com/dataset.csv
  - type: http
    url: https://mirror2.example.com/dataset.csv
  - type: http
    url: https://mirror3.example.com/dataset.csv
```

Provides maximum reliability by trying multiple independent mirror servers.

### 4. Command Tool Fallback
```yaml
sources:
  - type: command
    fetch_cmd: "curl -sL https://example.com/data.json -o {{dest}}"
  - type: command
    fetch_cmd: "wget -q https://example.com/data.json -O {{dest}}"
```

Tries curl first, automatically falls back to wget if curl is not available.

## Usage

### Initial Fetch
```bash
# Fetch all datasets (tries sources in order)
datum fetch

# Fetch specific dataset
datum fetch cdc_wtage_with_backup
```

### Verification
```bash
# Check all datasets
datum check
```

### What You'll See

When a source fails and fallback occurs, you'll see output like:
```
[WARN] my_data: source 1/2: fingerprint: connection timeout (trying next source)
[FETCH] my_data
```

When all sources fail:
```
[ERR ] my_data: all 2 sources failed, last error: connection refused
```

When the first source succeeds immediately:
```
[FETCH] my_data
[OK  ] my_data: up-to-date
```

## Policy Behavior with Multi-Source

All three policies (`fail`, `update`, `log`) work the same with multi-source:
- **Policies are applied AFTER all sources have been tried**
- If any source succeeds, the policy is evaluated against that source's fingerprint
- If all sources fail, the operation fails regardless of policy

### Example: `fail` policy
- Tries all sources in order
- If any succeeds, compares fingerprint with lockfile
- Fails only if fingerprint changed OR all sources failed

### Example: `update` policy
- Tries all sources in order
- If any succeeds, fetches data and updates lockfile
- Records error only if all sources failed

### Example: `log` policy
- Tries all sources in order
- If any succeeds, logs status but doesn't fail or update
- Logs error only if all sources failed

## Best Practices

1. **Order matters**: List sources from most reliable to least reliable
2. **Use compatible types**: All sources should provide the same data format
3. **Consider fingerprinting**: Different source types use different fingerprinting methods
4. **Test your fallbacks**: Ensure backup sources actually work
5. **Monitor warnings**: Watch for fallback messages indicating primary source issues

## Backward Compatibility

Single-source configurations continue to work:
```yaml
datasets:
  - id: my_data
    source:                     # Note: "source" (singular) - still supported
      type: http
      url: https://example.com/data.csv
    target: data/my_data.csv
```

## Error Handling

- **Configuration Error**: Specifying both `source:` and `sources:` is not allowed
- **Configuration Error**: Omitting both `source:` and `sources:` is not allowed
- **Runtime**: If all sources fail, datum records the error in the lockfile with timestamps

## Related Examples

- [Basic Example](../basic/) - Simple single-source HTTP usage
- [Multi-Policy Example](../multi-policy/) - Different policies for different datasets
- [Command Example](../command-system/) - Using command-line tools as sources
