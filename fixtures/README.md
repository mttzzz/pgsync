# Fixtures

Generated SQL fixtures are intentionally not committed. Create deterministic fixtures with:

- `make fixture-tiny`
- `make fixture-medium`
- `make fixture-large`

Each `.sql.gz` file has a JSON sidecar with expected table, row, and sequence metadata.

`download-dvdrental.sh` is the hook for the public small fixture. It leaves downloaded SQL ignored by git.
