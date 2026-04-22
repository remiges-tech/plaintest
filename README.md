# PlainTest

PlainTest separates setup from tests.

Setup runs once to prepare the environment. Tests iterate over CSV data to validate behavior. This prevents authentication collections from running multiple times and makes test execution predictable.

## Two Key Points

1. Use `--setup` for authentication and preparation
2. Use `--test` for actual validation that iterates with data

## Example

```bash
# Auth runs once, API tests run for each CSV row
./plaintest run --setup get_auth --test api_tests -d users.csv
```

This command:
- Runs `get_auth` collection once to authenticate
- Runs `api_tests` collection for each row in `users.csv`
- Shares authentication token between phases automatically

## How It Works

PlainTest wraps Newman and adds CSV iteration control.

Newman runs Postman collections. PlainTest decides which collections iterate with CSV data and which run once.

## Install

### Option 1: install with Go

```bash
go install github.com/remiges-tech/plaintest/cmd/plaintest@latest
```

### Option 2: build from source

```bash
go build -o plaintest ./cmd/plaintest
```

### Prerequisite

PlainTest also needs Newman:

```bash
npm install -g newman newman-reporter-htmlextra
```

## Use

```bash
# Create project
./plaintest init

# Basic separation
./plaintest run --setup get_auth --test api_tests

# With CSV data
./plaintest run --setup get_auth --test api_tests -d users.csv

# Select specific rows
./plaintest run --test api_tests -d users.csv -r 1-3

# Use Newman flags
./plaintest run --test smoke --verbose --bail
```

## Documentation

### What do you want to do?

**Start testing quickly**
[QUICK_START.md](QUICK_START.md) has step-by-step instructions for beginners.

**Understand the design**
[SETUP_TEST_DESIGN.md](SETUP_TEST_DESIGN.md) explains why setup and test phases are separate.

**Learn the architecture**
[ARCHITECTURE.md](ARCHITECTURE.md) shows how PlainTest proxies Newman.

### Documents by Purpose

| Document | Answers |
|----------|---------|
| [QUICK_START.md](QUICK_START.md) | How do I run my first tests? Step-by-step guide. |
| [CLI_REFERENCE.md](CLI_REFERENCE.md) | How do I run tests? What commands exist? How do I edit Postman scripts? |
| [SETUP_TEST_DESIGN.md](SETUP_TEST_DESIGN.md) | Why separate setup from tests? |
| [ARCHITECTURE.md](ARCHITECTURE.md) | How does PlainTest work internally? |

### For Different Users

**Testers**: Start with [QUICK_START.md](QUICK_START.md). Read CLI_REFERENCE for all commands.

**Developers**: Read ARCHITECTURE for internals. Check CLI_REFERENCE for commands.

**Architects**: Read SETUP_TEST_DESIGN for design rationale. Review ARCHITECTURE for implementation.

**Command reference**: Run `./plaintest --help` for command options.

## CSV Data Format

PlainTest uses three-column CSV structure:

- **META columns** (`test_*`): Test identification and metadata
- **INPUT columns** (`input_*`): Request parameters and data
- **EXPECTED columns** (`expected_*`): Expected response values

## Test Types

- **Smoke Tests**: Maximum 5 tests, 30-second timeout
- **Full Tests**: Complete test suite with CSV data iteration
- **Setup-Test Flow**: Setup runs once, test iterates with CSV data

## Architecture

```
cmd/plaintest/          # CLI entry point
internal/
├── core/              # Version and utilities
├── newman/            # Newman service wrapper
├── csv/               # CSV processing
├── scriptsync/        # Script sync utilities
└── templates/         # Project templates
```

## Development

```bash
# Run tests
go test ./...

# Build binary
go build -o plaintest ./cmd/plaintest
```

## Version

Current version: 0.0.1-dev
