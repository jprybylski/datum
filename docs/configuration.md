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

`data-schema.json` is the authored configuration contract. `go generate .` embeds that exact file
in the binary, and tests fail if the generated copy is stale or if YAML fields in Datum's decoder
and the schema drift apart.

## Environment Variables and Secrets

Any YAML string value can reference an environment variable with `${NAME}`. Datum substitutes
the value after parsing the YAML but before validating or using the configuration, so even a
secret containing characters meaningful to YAML remains a single value:

```yaml
datasets:
  - id: private_api
    desc: Data from an authenticated API
    source:
      type: http
      url: "https://api.example.com/export?token=${API_TOKEN}"
    target: "${DATA_DIR}/private.csv"
```

```bash
export API_TOKEN='replace-with-a-secret-from-your-secret-manager'
export DATA_DIR=data
datum fetch private_api
```

- Variable names must match `[A-Za-z_][A-Za-z0-9_]*`.
- An unset variable is a configuration error. A variable explicitly set to an empty value is
  allowed.
- Only the braced `${NAME}` form is expanded. Plain `$NAME` is left untouched, which lets command
  sources continue to use normal shell variables from their inherited environment.
- Write `$${NAME}` when the resulting configuration value must contain the literal `${NAME}`.
- Substitution applies to values, never YAML mapping keys. Values are not written to the lockfile,
  but a substituted secret may still be exposed by a downstream URL, command, or error message;
  prefer purpose-built authentication environment variables (such as the Git handler's
  `GIT_TOKEN`) when one is available.

Quote a value when its placeholder replaces the whole value and the final type must be a string.
Some editors may report a schema warning when a placeholder hides a required literal prefix, such
as `https://`; Datum validates the expanded value at runtime.

## Configuration File Structure

```yaml
version: 1                    # Config format version

defaults:
  policy: fail                # Default policy for all datasets
  algo: sha256                # Hashing algorithm (currently only sha256)
  ignore: false               # Ignore targets in a detected Git/SVN working copy

datasets:
  - id: unique_identifier     # Unique ID for this dataset
    desc: Human-readable description
    source:                   # Where to get the data (single source)
      type: http              # Handler type (http, file, git, command)
      url: https://...        # Handler-specific fields
    target: path/to/local/file.csv  # Where to save locally
    policy: update            # Override default policy (optional)
    ignore: true              # Override defaults.ignore (optional)
```

## Version-Control Ignore Rules

Set `defaults.ignore: true` to ignore fetched targets globally, then use a dataset-level
`ignore: false` when a particular target should remain visible to version control. The inverse
also works: keep the default false and opt individual datasets in.

Datum only manages ignore rules when it is run from inside a known working copy. In a Git
repository it maintains a clearly marked block in the repository-root `.gitignore`; in an SVN
working copy it maintains `svn:ignore` properties while tracking which entries belong to Datum.
Running outside Git or SVN is a no-op even when `ignore` is true.

Datum never untracks files. In a detected working copy, an ignored target must be untracked and
inside that working copy. SVN additionally requires the target's parent directory to already be
versioned so the ignore property can be applied precisely. These checks happen before a fetch.

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

Next: [Commands]({{ '/commands.html' | relative_url }}) for the full command reference and flags,
or [Handlers]({{ '/handlers.html' | relative_url }}) for what each `source.type` supports.
