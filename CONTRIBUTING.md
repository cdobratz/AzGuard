# Contributing to azguard

Thank you for your interest in contributing to azguard.

## Code of Conduct

Please be respectful and professional in all interactions. We follow the [Contributor Covenant](https://www.contributor-covenant.org/).

## How to Contribute

### Reporting Bugs

1. Check existing issues to avoid duplicates
2. Use the bug report template
3. Include:
   - Clear description
   - Steps to reproduce
   - Expected vs actual behavior
   - Environment details

### Suggesting Features

1. Check existing issues/discussions
2. Describe the use case
3. Explain how it should work
4. Consider backwards compatibility

### Pull Requests

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes
4. Add tests if applicable
5. Ensure code passes `go fmt` and `go vet`
6. Commit with clear messages
7. Push to your fork
8. Submit a pull request

## Development Setup

### Prerequisites

- Go 1.21+
- Git

### Setup

```bash
git clone https://github.com/cdobratz/AzGuard.git
cd AzGuard
go mod download
go build -o azguard ./cmd/azguard
go test ./...
```

### Project Structure

```
cmd/azguard/     CLI main package
internal/        config, cost, cloud clients, findings
configs/         free-tier limit files
```

### Coding Standards

- Use `go fmt` before committing
- Run `go vet` to catch issues
- Add comments for exported functions
- Keep functions focused and small
- Write tests for new features

### Commit Messages

Use clear, descriptive commit messages:

```
feat: add cost trend analysis command
fix: resolve Azure auth token refresh issue
docs: update README with new commands
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation
- `refactor`: Code refactoring
- `test`: Adding tests
- `chore`: Maintenance

## Getting Help

- Open an issue for bugs/features
- Start a discussion for questions
- Join community discussions

## Recognition

Contributors will be acknowledged in the README and release notes.
