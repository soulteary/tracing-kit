# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Because Go encodes the major version in the import path, every major release
also changes the module path. The current one is
`github.com/soulteary/tracing-kit/v2`.

## [Unreleased]

## [2.0.0] — 2026-09-21

### Changed — BREAKING

- **Tracer initialisation moved to the `otlp` subpackage.** The root package no
  longer imports the OTLP exporter, and with it no longer imports the
  OpenTelemetry SDK, gRPC or protobuf. Measured against v1.5.1 for a program
  that imports only the root package — starting spans and propagating trace
  context, which is all a library does:

  | | v1.5.1 | v2.0.0 |
  |---|---:|---:|
  | binary | 18,095,419 B | 7,319,801 B (**−59.5%**) |
  | linked packages outside the standard library | 191 | 42 |
  | modules contributing to the build | 22 | 8 |
  | `// indirect` requirements in the consumer's `go.mod` | 21 | 7 |
  | modules in the consumer's `go.sum` | 28 | 13 |

  Fifteen modules leave that program's `go.sum` outright: the two otlptrace
  modules, and the thirteen they drag along — `cenkalti/backoff/v5`,
  `golang/protobuf`, `grpc-ecosystem/grpc-gateway/v2`, `otel/sdk/metric`,
  `proto/otlp`, `go.uber.org/goleak`, `golang.org/x/net`, `golang.org/x/text`,
  `gonum.org/v1/gonum`, both `genproto/googleapis` modules,
  `google.golang.org/grpc` and `google.golang.org/protobuf`.

  A program that *does* export over OTLP pays exactly what it paid before:
  21,482,897 → 21,485,381 bytes, 394 linked packages either way, the same 23
  modules and the same 57-line `go.sum`.

  | Removed from the root package | Replacement |
  |---|---|
  | `tracing.InitTracer` | `otlp.InitTracer` |
  | `tracing.InitTracerWithConfig` | `otlp.InitTracerWithConfig` |
  | `tracing.Config` | `otlp.Config` |
  | `tracing.Config.OTLPEndpoint` | `otlp.Config.Endpoint` |
  | `tracing.DefaultSampleRatio` | `otlp.DefaultSampleRatio` |

  Keeping these as deprecated shims was not an option: a shim has to import
  otlptracehttp, which relinks gRPC and protobuf and gives back the entire
  benefit.

  `OTLPEndpoint` became `Endpoint` because `otlp.Config.OTLPEndpoint` says it
  twice. That rename is the only field change; the rest of `Config` — including
  the `SampleRatio` zero-value rule and `SampleNone` — behaves exactly as it
  did.

- **The test helpers moved to the `tracingtest` subpackage.** `testhelper.go`
  was normal package code importing `go.opentelemetry.io/otel/sdk/trace/tracetest`,
  so every production binary depending on this library linked the in-memory
  test exporter.

  | Removed from the root package | Replacement |
  |---|---|
  | `tracing.SetupTestTracer` | `tracingtest.Setup` |
  | `tracing.TeardownTestTracer` | `tracingtest.Teardown` |
  | `tracing.ShutdownTracerProvider` | `tracingtest.Shutdown` |
  | `tracing.ForceFlushTracerProvider` | `tracingtest.ForceFlush` |
  | `tracing.TestingT` | `tracingtest.TestingT` |

  `tracingtest.Setup` now registers its own cleanup through `TestingT.Cleanup`,
  which the interface has always required and the old helper never called, so
  the `defer func() { Shutdown(tp); Teardown() }()` every test carried is no
  longer needed. Leaving it in is harmless: both are idempotent.

- **Subpackages, not separate modules.** Module graph pruning keeps a
  requirement that no imported package needs out of the consumer's `go.mod`
  entirely, which is what the table above measures. It is not free of all
  traces: this module's own `go.mod` still names otlptracehttp and the SDK, so
  their `/go.mod` checksums stay in a root-only consumer's `go.sum` and their
  minimum versions still take part in that consumer's version selection. What
  disappears is the code, the downloads and the `go.mod` requirements.

  **The module path is therefore now `github.com/soulteary/tracing-kit/v2`**,
  by the import compatibility rule. Every user must update the import path,
  including those who keep using the OTLP exporter and are otherwise
  unaffected.

### Added

- The `otlp` subpackage. Besides the two `InitTracer` forms it exports the
  pieces they are built from, so a provider that needs one of them done
  differently can be assembled without reimplementing the rest:
  `NewExporter`, `Config.Sampler` and `Config.Resource`.
- The `tracingtest` subpackage, and `tracingtest.ServiceName` naming the
  `"test-service"` the old helper installed anonymously.
- `tracing.Install`, `tracing.InstallDisabled` and `tracing.Uninstall` manage
  the process-wide provider, which is what the root package kept for itself
  before. `Install` takes `tracing.Provider` — `Tracer` plus `Shutdown` —
  rather than `*sdktrace.TracerProvider`, so a stdout exporter, a vendor SDK or
  a hand-assembled provider installs the same way the OTLP one does, and so
  that the root package needs no SDK to declare the parameter.

  Taking an interface reopens a hole the concrete type closed by construction,
  so `Install` closes it explicitly: a plain `p == nil` misses a typed nil,
  such as an unassigned `*sdktrace.TracerProvider` field, and taking a tracer
  from one panics. A nil provider is reported through OpenTelemetry's error
  handler and changes nothing — a tracing library that takes the process down
  is worse than one that records nothing, and a caller whose argument came out
  nil has said nothing about the provider already running. `Install` likewise
  does not compare two providers with `==` when deciding whether it is
  replacing itself: interfaces holding an uncomparable type panic on
  comparison.
- `tracing.SetDefaultPropagator` installs the W3C trace context and baggage
  propagator that `InjectTraceContext` and `ExtractTraceContext` need.
  `InitTracerWithConfig` calls it, as before, but it is now reachable on its
  own — a service that exports no spans still has to pass an incoming trace
  context to the next hop, and OpenTelemetry's default propagator carries
  nothing.
- A package doc in `doc.go` describing the layout and why it is split that way.
- Runnable examples (`Example`, `ExampleInstall`, `ExampleInstallDisabled`,
  `ExampleInjectTraceContext`, `ExampleRecordError`, `ExampleConfig_Sampler`,
  `ExampleInitTracerWithConfig_disabled`) that `go test` verifies, so they
  cannot drift from the API.
- `CHANGELOG.md`.
- `.github/workflows/release.yml`. Nine tags exist with nothing having checked
  any of them, and the mistake a `/vN` module makes is caught at tag time or
  not at all. It runs on a `v*` tag (and on demand): the module path must carry
  the tag's major version, with v0 and v1 taking no suffix, and both READMEs'
  `go get` line must name that same path. Then the CI gate against the tagged
  commit — gofmt, `go mod tidy` cleanliness, vet, golangci-lint, `go test
  -race` with coverage, and govulncheck. Verification only: it publishes
  nothing and takes no write permissions.
- `.github/dependabot.yml`. Weekly gomod and github-actions updates, minor and
  patch grouped into one PR, majors left separate. The release gate is an
  action too, so a silently stale action would be a stale release check.

### Changed

- `Uninstall` (which `tracingtest.Teardown` calls) points the global
  OpenTelemetry provider at a no-op provider. `TeardownTestTracer` set it to
  `nil`, which is not a provider: a later `otel.Tracer(...)` on that global
  panicked.
- A failed `InitTracerWithConfig` now leaves the running provider alone. It
  used to overwrite the recorded service name before the first thing that can
  fail, so a failed reconfiguration left the process reporting a name whose
  provider was never installed.
- Coverage is 100% of statements across all three packages, measured with
  `go test -race ./... -covermode=atomic`.

### Fixed

- Errors from shutting down the provider being replaced go to OpenTelemetry's
  error handler instead of `log.Printf`, so a program that configures a logger
  is not written to on stderr behind its back. This also drops `log` from the
  root package's imports.

### Notes on dependencies

Nothing to upgrade. Every direct requirement is already on the newest stable
OpenTelemetry release, v1.46.0; the only newer tag is v1.47.0-rc.1, which a
library should not pin its users to. The two indirect requirements with
newer versions — `google.golang.org/grpc` v1.83.2 → v1.84.0 and the two
`genproto/googleapis` modules — are reachable only through otlptracehttp, so
after this release they are not in a root-only consumer's graph at all. The
dependency work here is subtraction, not version bumps.

## [1.5.1] — 2026-09-21

### Changed

- The semantic conventions moved from `semconv/v1.21.0` to `semconv/v1.43.0`,
  the version bundled in the `go.opentelemetry.io/otel` v1.46.0 this module
  already required. `go.mod` and `go.sum` were untouched, and the exported
  resource is byte-for-byte what it was: `ServiceName` and `ServiceVersion`
  have the same signature in both versions and return the same keys.

## [1.5.0] — 2026-09-12

### Added

- `InitTracerWithConfig` and `Config`, making TLS, sampling and the export
  timeout configurable. They had been hard-coded — `otlptracehttp.WithInsecure()`
  and `AlwaysSample()` — with comments telling the reader to change them in
  production and no way to do so, so **every deployment exported traces in
  cleartext and recorded 100% of them**.

### Changed — BREAKING

- `SetResourceNewFunc`, `SetOtlptraceNewFunc` and `ResetHooks` left the public
  API. They lived in `test_helpers.go`, a normal source file importing
  `testing`, which linked the testing package into every production binary
  depending on this library, registered its `-test.*` flags into
  `flag.CommandLine`, and let anything in the process swap the trace exporter
  at run time.
- `SetupTestTracer` takes a `tracing.TestingT` (`Helper` + `Cleanup`) rather
  than `*testing.T`. `*testing.T` satisfies it, so existing calls compiled
  unchanged.

### Fixed

- `InitTracer` with no endpoint returns a provider that records nothing
  instead of `(nil, nil)`, which made the idiomatic `defer tp.Shutdown(ctx)`
  a nil dereference whenever tracing was switched off.
- The package-level tracer state is synchronised. `InitTracer` racing with
  `GetTracer` or `IsEnabled` was a data race, never caught because the tests
  run sequentially.
- Reconfiguring to an empty endpoint retires the provider being replaced
  instead of leaving it exporting to the old collector.
- `SampleNone` maps to `NeverSample` rather than a zero ratio, which
  `ParentBased` overrode for any request arriving already sampled.

[Unreleased]: https://github.com/soulteary/tracing-kit/compare/v2.0.0...HEAD
[2.0.0]: https://github.com/soulteary/tracing-kit/compare/v1.5.1...v2.0.0
[1.5.1]: https://github.com/soulteary/tracing-kit/compare/v1.5.0...v1.5.1
[1.5.0]: https://github.com/soulteary/tracing-kit/compare/v1.4.0...v1.5.0
