# Contributing to Datum

Thank you for your interest in contributing to Datum! This document provides guidelines and instructions for contributing to the project.

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR-USERNAME/datum.git
   cd datum
   ```
3. **Set up your development environment**:
   ```bash
   go mod download
   go build ./cmd/datum
   ```

## Development Workflow

### Before You Start

- Check existing issues and pull requests to avoid duplicate work
- For major changes, open an issue first to discuss your approach
- Keep changes focused - one feature or fix per pull request

### Making Changes

1. **Create a new branch** for your work:
   ```bash
   git checkout -b feature/your-feature-name
   # or
   git checkout -b fix/issue-description
   ```

2. **Make your changes** following the code style guidelines below

3. **Write or update tests** to cover your changes:
   ```bash
   go test ./...
   ```

4. **Run code quality checks**:
   ```bash
   go fmt ./...
   go vet ./...
   golangci-lint run --timeout=5m
   ```

5. **Build and test** your changes:
   ```bash
   # Build without git support
   go build ./cmd/datum

   # Build with git support
   go build -tags git ./cmd/datum

   # Run the examples
   ./datum --config examples/basic/.data.yaml fetch
   ./datum --config examples/basic/.data.yaml check
   ```

6. **Commit your changes** with clear, descriptive commit messages:
   ```bash
   git add .
   git commit -m "Add feature: brief description"
   ```

7. **Push to your fork**:
   ```bash
   git push origin feature/your-feature-name
   ```

8. **Open a pull request** on GitHub

## Code Style Guidelines

### General Principles

- Follow standard Go conventions and idioms
- Write clear, self-documenting code with helpful comments
- Keep functions focused and reasonably sized
- Handle errors explicitly - don't ignore them

### Go-Specific Guidelines

- Run `go fmt` on all code before committing
- Follow the [Effective Go](https://go.dev/doc/effective_go) guidelines
- Use meaningful variable and function names
- Add package-level comments for all packages
- Document exported functions, types, and constants

### Code Organization

- Keep related functionality together in the same package
- Use the existing package structure:
  - `internal/core/` - Core business logic
  - `internal/handlers/` - Data source handlers
  - `internal/registry/` - Handler registry
  - `internal/runtime/` - Platform-specific code
- Follow the plugin pattern for new handlers (see "Adding New Handlers" below)

### Error Handling

- Use error wrapping with `fmt.Errorf("context: %w", err)` to preserve error chains
- Provide meaningful context in error messages
- Check errors immediately after operations that can fail

### Testing

- Write table-driven tests using subtests
- Test both success and error cases
- Use descriptive test names that explain what is being tested
- Mock external dependencies when appropriate
- Aim for good test coverage, especially for core logic

## Adding New Handlers

To add a new data source handler:

1. **Create a new package** in `internal/handlers/yourhandler/`

2. **Implement the `Fetcher` interface**:
   ```go
   type Fetcher interface {
       Check(ctx context.Context, src Source) (CheckResult, error)
       Fetch(ctx context.Context, src Source, dest string) (FetchResult, error)
   }
   ```

3. **Register your handler** in an `init()` function:
   ```go
   func init() {
       registry.Register("yourscheme", &YourHandler{})
   }
   ```

4. **Write comprehensive tests** in `yourhandler_test.go`

5. **Update documentation**:
   - Add usage examples to README.md
   - Create an example configuration in `examples/`
   - Update CLAUDE.md if there are architectural notes

6. **Consider build tags** if your handler has optional dependencies (like the git handler)

## Testing Guidelines

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with race detector and coverage
go test -v -race -coverprofile=coverage.txt ./...

# Run specific package tests
go test ./internal/core
go test ./internal/handlers/http
```

### Writing Tests

- Use table-driven tests for testing multiple scenarios
- Use `t.Run()` for subtests to organize test cases
- Clean up resources (files, directories) in tests
- Use `t.TempDir()` for temporary test directories
- Test edge cases and error conditions

Example:
```go
func TestHandler(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"valid input", "test", "result", false},
        {"invalid input", "", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Handler(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("Handler() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("Handler() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Build Tags

If your contribution requires optional dependencies:

1. Use build tags to make the feature optional
2. Add the tag at the top of the file:
   ```go
   //go:build yourtag
   ```
3. Update build scripts and documentation
4. Ensure CI tests build both with and without the tag

## Continuous Integration

All pull requests must pass CI checks:

- **Tests**: Run on Ubuntu, macOS, and Windows with Go 1.23 and stable
- **Build**: Verify compilation with and without build tags
- **Lint**: Pass golangci-lint checks (v2.6.0)
- **Examples**: All examples must work correctly

You can run the same checks locally before pushing.

## Pull Request Process

1. **Ensure all tests pass** and code is formatted
2. **Update documentation** if you've added or changed functionality
3. **Add examples** if you've added a new handler or major feature
4. **Write a clear PR description** explaining:
   - What problem does this solve?
   - What changes were made?
   - How was it tested?
5. **Link related issues** using "Fixes #123" or "Closes #123"
6. **Respond to review feedback** promptly
7. **Keep your branch up to date** with main if needed

## Commit Message Guidelines

Write clear, concise commit messages:

```
Add HTTP retry logic with exponential backoff

- Implement retry mechanism for transient HTTP errors
- Add configurable max retries and backoff settings
- Update tests to cover retry scenarios
```

- Use the imperative mood ("Add feature" not "Added feature")
- Keep the first line under 72 characters
- Add detailed explanation in the body if needed
- Reference issues and PRs where relevant

## Code Review

All contributions go through code review. Reviewers will check:

- Code quality and style
- Test coverage
- Documentation completeness
- Performance considerations
- Security implications
- Compatibility with existing features

Be open to feedback and willing to iterate on your changes.

## Reporting Issues

When reporting bugs or requesting features:

1. **Search existing issues** first to avoid duplicates
2. **Use issue templates** if available
3. **Provide details**:
   - Go version (`go version`)
   - OS and version
   - Steps to reproduce
   - Expected vs actual behavior
   - Relevant logs or error messages
4. **Include configuration files** if applicable (sanitize sensitive data)

## Questions or Need Help?

- Open a GitHub issue with the "question" label
- Check existing documentation in README.md and CLAUDE.md
- Review examples in the `examples/` directory

## License

By contributing to Datum, you agree that your contributions will be licensed under the MIT License.

## Recognition

Contributors will be recognized in the project. Thank you for helping make Datum better!
