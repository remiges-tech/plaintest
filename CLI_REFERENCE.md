# PlainTest CLI Reference

PlainTest wraps Newman. Adds setup-test separation, CSV row selection, and script extraction.

## Prerequisites

Newman and reporter required:

```bash
npm install -g newman newman-reporter-htmlextra
```

## Installation

Install with Go:

```bash
go install github.com/remiges-tech/plaintest/cmd/plaintest@latest
```

Or build from source:

```bash
go build -o plaintest ./cmd/plaintest
```

## Commands

### version

Shows version.

```bash
plaintest version
```

### init

Creates project structure.

```bash
plaintest init
```

Creates:
- collections/raw/ - Postman exports
- collections/build/ - Generated collections
- scripts/ - JavaScript files
- data/ - CSV files
- environments/ - Environment configs
- reports/ - Test results

Includes DummyJSON templates.

### list

Shows project resources.

```bash
plaintest list collections   # Show collections
plaintest list data          # Show CSV files
plaintest list environments  # Show environment files
plaintest list scripts       # Show script directories
```

### scripts pull

Extracts scripts to JavaScript files.

```bash
plaintest scripts pull my-api
```

Creates files in scripts/my-api/.
Always overwrites existing files.

### scripts push

Updates collection with edited scripts.

```bash
plaintest scripts push my-api
```

Scripts become source of truth after extraction.

### run

Executes tests with Newman.

Basic:

```bash
plaintest run smoke
plaintest run user_tests -d data.csv
```

Setup-test separation:

```bash
plaintest run --setup auth --test user_tests -d data.csv
```

Link specification:

```bash
plaintest run --setup "auth.Login" --test "users.Create,Update"
```

## Flags

### PlainTest Flags

**-r, --rows** - Select CSV rows

```bash
-r 3        # Row 3 only
-r 2-5      # Rows 2 through 5
-r 1,3,5    # Specific rows
```

**--setup** - Runs once

```bash
--setup auth
--setup "auth.Login"
```

**--test** - Iterates with CSV

```bash
--test user_tests
--test "users.Create User"
```

**--reports** - Generate timestamped reports

Creates HTML and JSON in reports/.

**--debug** - Show Newman command

Prints exact command before running.

### Newman Flags

Pass through to Newman:

**-e, --environment** - Environment file

```bash
-e production                                    # By name
-e environments/production.postman_environment.json  # By path
```

**-d, --iteration-data** - CSV file

```bash
-d users                  # By name
-d data/users.csv        # By path
```

**--verbose** - Show request/response
**--timeout** - Request timeout (ms)
**--bail** - Stop on first failure
**--reporters** - Output formats

See `newman run --help` for all flags.

## Auto-Discovery

PlainTest finds resources automatically.

Collections in:
- collections/build/*.postman_collection.json
- collections/*.postman_collection.json

Environments in:
- environments/*.postman_environment.json

Data in:
- data/*.csv

Reference by name:

```bash
plaintest run auth user_tests -e production -d users
```

Single environment auto-detected if only one exists.

## Environment Chaining

Collections share environment variables.

First collection sets token:

```javascript
pm.environment.set("auth_token", pm.response.json().token);
```

Next collection uses it:

```javascript
pm.request.headers.add({
    key: "Authorization",
    value: "Bearer {{auth_token}}"
});
```

## CSV Format

Three column types:

**META** (test_*)
- test_id
- test_name

**INPUT** (input_*)
- input_email
- input_age

**EXPECTED** (expected_*)
- expected_status
- expected_message

Example:

```csv
test_id,input_email,expected_status
valid,user@test.com,200
invalid,bad-email,400
```

## Link Specification

Run parts of collections.

Syntax:

```bash
collection                    # Entire collection
"collection.Request Name"     # One request
"collection.Folder Name"      # One folder
"collection.Item1,Item2"      # Multiple items
```

Examples:

```bash
plaintest run auth                           # Run auth collection
plaintest run "users.Create User"           # Run one request
plaintest run "users.Registration Folder"   # Run one folder
plaintest run "users.Create,Update,Delete"  # Run specific requests
```

Use quotes for names with spaces.

## Setup-Test Execution

Setup runs once. Tests iterate with CSV.

**Setup phase** - Authentication, database prep

```bash
plaintest run --setup auth --test users -d data.csv
```

Auth runs once. Users runs for each CSV row.

**Multiple links**

```bash
plaintest run --setup db_init --setup auth --test users --test orders
```

Links run in order. Environment flows between them.

**Mixed execution**

```bash
plaintest run smoke --setup auth --test users -d data.csv
```

Order: smoke → auth → users (with CSV iteration).

## Reports

**--reports** flag creates:
- collection_YYYYMMDDTHHMMSS.json - Machine readable
- collection_YYYYMMDDTHHMMSS.html - Human readable

Query JSON:

```bash
jq '.run.executions[] | select(.response.code >= 400)' reports/*.json
```

**Newman reporters**

```bash
plaintest run users --reporters cli,json --reporter-json-export results.json
plaintest run users --reporters htmlextra --reporter-htmlextra-export report.html
```

## Examples

**Smoke test**

```bash
plaintest run smoke --timeout 5000
```

**Development**

```bash
plaintest run --setup auth --test users -d data.csv -r 3 --verbose
```

**CI/CD**

```bash
plaintest run --test smoke --test regression --bail --reporters cli,json
```

**Debug specific row**

```bash
plaintest run --test users -d data.csv -r 47 --verbose
```

**Multiple environments**

```bash
plaintest run smoke -e staging
plaintest run smoke -e production
```

**Complex workflow**

```bash
plaintest run --setup db_reset --setup auth --test "users.Registration" --test "users.Profile Updates" -d user_scenarios.csv -r 10-20 --verbose
```

## Troubleshooting

**Newman not found**

Install: `npm install -g newman`

**Collection not found**

Check collections/ directory.
Run `plaintest list collections`.

**CSV rows not working**

Need -d flag with CSV file.
Rows start at 1 (header not counted).

**Single environment not auto-detected**

Only works with exactly one file in environments/.

**Setup-test not separating**

Check --setup and --test flags.
Setup runs once regardless of CSV.
Test iterates with CSV data.

**Link specification not working**

Use quotes for names with spaces.
Check exact folder/request names in Postman.

**Environment variables not flowing**

Collections run in sequence.
Variables set in first collection available in second.
Use `pm.environment.set()` and `{{variable}}` syntax.

## Working with Reports

**View failed requests**

```bash
jq '.run.executions[] | select(.response.code >= 400) | {request: .request.name, status: .response.code}' reports/*.json
```

**Extract response body**

```bash
jq -r '.run.executions[0].response.stream.data | implode' reports/*.json
```

**Count total requests**

```bash
jq '.run.executions | length' reports/*.json
```

**HTML reports**

Open in browser for formatted view with request/response details.

## Script Management

**Pull scripts from collection**

```bash
plaintest scripts pull users
```

Creates:
- scripts/users/_collection__prerequest.js
- scripts/users/_collection__test.js
- scripts/users/create-user__test.js
- scripts/users/get-user__prerequest.js

**Edit in IDE**

```bash
code scripts/users/
```

Edit with syntax highlighting, linting, debugging.

**Push changes back**

```bash
plaintest scripts push users
```

Updates collections/users.postman_collection.json with script changes.

**Workflow**

1. Export collection from Postman
2. Pull scripts once: `plaintest scripts pull collection`
3. Edit JavaScript files in your IDE
4. Push changes: `plaintest scripts push collection`
5. Run updated tests: `plaintest run collection`

Scripts become source of truth after extraction. Don't edit in Postman after pulling.

## Workflow Examples

**API Health Monitoring**

```bash
plaintest run smoke --timeout 5000
```

**Development Testing**

```bash
plaintest run --setup auth --test api_tests -d dev_data.csv --verbose
```

**CI/CD Pipeline**

```bash
plaintest run --test smoke --test api_tests --bail --timeout 30000 --reporters cli,json
```

**Manual Testing**

```bash
plaintest run api_tests -d manual_test.csv -r 5-10 --verbose
```
