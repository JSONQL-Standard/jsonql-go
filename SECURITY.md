# Security Policy

## Supported Versions

JSONQL follows semantic versioning. Security fixes are backported to the latest
minor release of each supported major version.

| Version | Supported |
| ------- | --------- |
| 1.1.x   | Yes       |
| 1.0.x   | Yes       |
| < 1.0   | No        |

## Reporting a Vulnerability

Do not open a public issue for security vulnerabilities.

Please report security issues privately through GitHub private vulnerability
reporting in the affected repository:

1. Go to the Security tab of the affected repository.
2. Click Report a vulnerability.
3. Include the affected repository, SDK, version, or commit SHA.
4. Include reproduction steps or a minimal proof of concept.
5. Include impact details and any suggested mitigation, if known.

If GitHub private reporting is unavailable, email security@jsonql.dev if it is
configured, or contact the maintainer at law108000@gmail.com with the subject
line `[SECURITY] JSONQL <repo>: <short summary>`.

## Response Targets

| Severity | Acknowledge | Initial assessment | Fix target |
| -------- | ----------- | ------------------ | ---------- |
| Critical | 24 hours    | 72 hours           | 7 days     |
| High     | 48 hours    | 1 week             | 30 days    |
| Medium   | 1 week      | 2 weeks            | 90 days    |
| Low      | 2 weeks     | 4 weeks            | Next minor |

We will coordinate disclosure and credit the reporter unless anonymity is
requested.

## Scope

In scope:

- Code in JSONQL-Standard repositories.
- The JSONQL specification and shared compliance tests.
- SDK parser, validator, transpiler, driver, hydrator, and adapter behavior.
- Build, release, and documentation tooling.

Out of scope:

- Vulnerabilities in third-party dependencies, unless the report concerns how
  JSONQL uses them.
- Issues only reproducible against unsupported versions.
- Theoretical attacks without a practical proof of concept or clear impact.

## Security Invariants

JSONQL guarantees the following at the specification and SDK level. A deviation
from these invariants is treated as a security issue:

1. Parameterized queries only; no string-concatenated SQL.
2. No code execution; no `$where`, `eval`, regex, or scripting operators.
3. No filesystem or network side effects from parsing or transpilation.
4. Invalid queries are rejected at boundaries rather than coerced.

Reports demonstrating violations of these invariants will be triaged as High or
Critical depending on exploitability and impact.

## Maintainer Handling

Security reports remain private until a fix or mitigation is available. When a
fix is released, maintainers may publish a GitHub security advisory, release
notes, and remediation guidance for affected versions.
