---
layout: default
title: Configuration
nav_order: 3
---

# Configuration

Datum uses two files:

1. **`.data.yaml`** - Your configuration (version controlled)
2. **`.data.lock.yaml`** - Generated lockfile with fingerprints (version controlled)

## IDE Support with JSON Schema

Datum provides a JSON Schema file ([`data-schema.json`](https://github.com/jprybylski/datum/blob/main/data-schema.json))
for IDE autocomplete, validation, and documentation.

**VS Code:**

Add this to the top of your `.data.yaml` file:
```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/jprybylski/datum/main/data-schema.json
```

Or configure it globally in VS Code settings:
```json
{
  "yaml.schemas": {
    "https://raw.githubusercontent.com/jprybylski/datum/main/data-schema.json": ".data.yaml"
  }
}
```

**JetBrains IDEs (IntelliJ, PyCharm, etc.):**

1. Go to Settings → Languages & Frameworks → Schemas and DTDs → JSON Schema Mappings
2. Add a new mapping:
   - Name: "Datum Configuration"
   - Schema URL: `https://raw.githubusercontent.com/jprybylski/datum/main/data-schema.json`
   - File path pattern: `.data.yaml`

**Local Schema:**

For offline use, reference the schema file locally:
```yaml
# yaml-language-server: $schema=./data-schema.json
```

The schema provides:
- Autocomplete for all fields and values
- Validation of data types and required fields
- Documentation on hover
- Handler-specific field validation based on `type`

## Configuration File Structure

```yaml
version: 1                    # Config format version

defaults:
  policy: fail                # Default policy for all datasets
  algo: sha256                # Hashing algorithm (currently only sha256)

datasets:
  - id: unique_identifier     # Unique ID for this dataset
    desc: Human-readable description
    source:                   # Where to get the data (single source)
      type: http              # Handler type (http, file, git, command)
      url: https://...        # Handler-specific fields
    target: path/to/local/file.csv  # Where to save locally
    policy: update            # Override default policy (optional)
```

## Multi-Source Configuration

Datum supports specifying multiple sources with automatic fallback. If the first source fails,
datum will try subsequent sources in order:

```yaml
datasets:
  - id: my_data
    desc: Data with fallback sources
    sources:                  # Note: "sources" (plural) instead of "source"
      - type: http            # Primary source
        url: https://primary.example.com/data.csv
      - type: http            # Backup source (used if primary fails)
        url: https://backup.example.com/data.csv
      - type: file            # Local fallback
        path: ./cache/data.csv
    target: data/my_data.csv
```

**Key points:**
- Use either `source:` (single) or `sources:` (multiple), but not both
- Sources are tried in the order they are listed
- The final policy judgment is applied after all sources have been attempted
- Useful for high availability, geographic redundancy, and offline development

See the [multi-source example](https://github.com/jprybylski/datum/tree/main/examples/multi-source)
for more details.

## Policy Options

- **`fail`**: Verification fails if the remote data has changed (strict mode)
- **`update`**: Automatically fetch and update if the remote data has changed
- **`log`**: Log changes but don't fail or update (monitoring mode)

Next: [Commands]({% link commands.md %}) for `check`/`fetch` details and flags, or
[Handlers]({% link handlers.md %}) for what each `source.type` supports.
