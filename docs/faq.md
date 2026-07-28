---
layout: default
title: FAQ
nav_order: 9
---

# FAQ

## Why "datum"?

Datum is the singular form of "data" - fitting for a tool that manages individual data sources.

## How is this different from downloading files manually?

Datum provides:
- Automated verification
- Cryptographic fingerprints
- Policy-based handling of changes
- Single configuration file for all data sources
- Reproducibility for your entire data pipeline

## Can I use this in CI/CD?

Yes! Use `datum check` in your CI pipeline to verify that external data hasn't changed
unexpectedly.

## What happens if my policy is "fail" and data changes?

The `check` command will exit with code 1, and you'll see which datasets have changed. You can
then:
1. Investigate why the data changed
2. Run `datum fetch <dataset-id>` to update specific datasets
3. Commit the updated lockfile

## How do I version control the lockfile?

Yes, commit both `.data.yaml` and `.data.lock.yaml` to version control. This ensures:
- Team members have the same data versions
- Historical record of when data changed
- Reproducible builds

---

Didn't find your answer? [Open an issue](https://github.com/jprybylski/datum/issues) on GitHub.
