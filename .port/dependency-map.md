# Dependency Map

The following table maps the external Node.js dependencies identified in `.port/manifest.json` to their idiomatic Go standard library or third-party equivalents.

| Source Dependency | Go Module | Import Path | Classification | Notes |
|---|---|---|---|---|
| `argv-tools` | Standard Library | `flag` | `stdlib` | Go's standard library `flag` package handles command line arguments. |
| `array-back` | N/A (Language Feature) | N/A | `stdlib` | Go supports slices natively; wrapping a single element or managing empty slices is done via native slice literals or logic. |
| `axios` | Standard Library | `net/http` | `stdlib` | Standard `net/http` package provides an idiomatic and high-performance HTTP client. |
| `cookie-parser` | Standard Library | `net/http` | `stdlib` | Request cookies are parsed natively using `(*http.Request).Cookies()` and `(*http.Request).Cookie(name)`. |
| `core-util-is` | N/A (Language Feature) | N/A | `stdlib` | Handled natively by Go's strong type system, type assertions, and type switches (or `reflect` package if dynamic). |
| `debug` | Standard Library | `log/slog` | `stdlib` | Go's structured logging package `log/slog` (or standard `log`) provides conditional/environmental logging. |
| `express` | Standard Library | `net/http` | `stdlib` | Standard library `net/http` (specifically `http.ServeMux`) provides routing. Go 1.22+ supports HTTP method matching and path parameters natively. |
| `express-rate-limit` | `golang.org/x/time` | `golang.org/x/time/rate` | `direct` | Official sub-repository rate limiter package. |
| `express-session` | `github.com/alexedwards/scs/v2` | `github.com/alexedwards/scs/v2` | `direct` | Modern, actively maintained session manager for Go web servers. |
| `find-replace` | N/A (Language Feature) | N/A | `stdlib` | Handled using standard slice operations and loops. |
| `graceful-fs` | Standard Library | `os`, `io` | `stdlib` | Standard filesystem calls (`os`, `io`) are robust. System-wide limits (EMFILE) can be managed using custom channel-based concurrency limiters. |
| `htmlparser2` | `golang.org/x/net` | `golang.org/x/net/html` | `direct` | Official HTML parser and tokenizer. |
| `inherits` | N/A (Language Feature) | N/A | `stdlib` | Handled natively by struct embedding, composition, and interfaces. |
| `isarray` | N/A (Language Feature) | N/A | `stdlib` | Handled natively using Go slice and array type checking. |
| `jsonwebtoken` | `github.com/golang-jwt/jwt/v5` | `github.com/golang-jwt/jwt/v5` | `direct` | Standard JWT library, already in `go.mod`. |
| `lru-cache` | `github.com/hashicorp/golang-lru/v2` | `github.com/hashicorp/golang-lru/v2` | `direct` | Generic thread-safe LRU cache package from HashiCorp. |
| `mime-types` | Standard Library | `mime` | `stdlib` | Standard `mime` package handles MIME type lookup by extension and vice-versa. |
| `module` | N/A (Language Feature) | N/A | `stdlib` | Represented natively by Go's package and module import system. |
| `ms` | Standard Library | `time` | `stdlib` | Go's standard `time.ParseDuration` parses strings like "300ms", "1.5h". Complex strings like "2 days" must be handled with simple custom helpers. |
| `node-unrar-js` | `github.com/nwaples/rardecode/v2` | `github.com/nwaples/rardecode/v2` | `direct` | Pure Go RAR archive decompression/decoding library. |
| `nodemailer` | Standard Library | `net/smtp` | `stdlib` | Go standard library `net/smtp` sends mail. For more complex use cases (e.g. attachments, HTML bodies), a direct equivalent like `github.com/wneessen/go-mail` is recommended. |
| `openid-client` | `github.com/coreos/go-oidc/v3` | `github.com/coreos/go-oidc/v3` | `direct` | Modern OpenID Connect client implementation in Go. |
| `p-throttle` | `golang.org/x/time` | `golang.org/x/time/rate` | `stdlib` | Rate limiting or concurrency throttling is handled natively using channels/goroutines or `golang.org/x/time/rate`. |
| `passport` | Standard Library / `github.com/markbates/goth` | `net/http` / `github.com/markbates/goth` | `partial` | **Blocker**: Go does not have a single monolithic authentication framework. Standard practice is to write custom HTTP middleware injecting user session into request context, or use `goth` for third-party authentication. |
| `passport-jwt` | `github.com/golang-jwt/jwt/v5` | `github.com/golang-jwt/jwt/v5` | `direct` | Handled by custom middleware validating JWTs using `github.com/golang-jwt/jwt/v5`. |
| `passport-strategy` | Standard Library | `net/http` | `stdlib` | Handled by standard `http.Handler` / middleware interfaces. |
| `perf_hooks` | Standard Library | `time`, `runtime/pprof` | `stdlib` | Handled using standard `time` measurement and profiling packages. |
| `process-nextick-args`| N/A (Language Feature) | N/A | `stdlib` | Handled natively using the `go` keyword for scheduling goroutines. |
| `ripstat` | Standard Library | `os` | `stdlib` | Go's standard `os.Stat` is natively compiled and highly performant. |
| `safe-buffer` | Standard Library | `bytes`, `strings` | `stdlib` | Standard library slice and string handling is memory-safe. |
| `semver` | `github.com/Masterminds/semver/v3` | `github.com/Masterminds/semver/v3` | `direct` | Industry standard Go package for semantic version parsing and comparison. |
| `sequelize` | Standard Library | `database/sql` | `stdlib` | Standard `database/sql` is used instead of a heavy ORM (already implemented in Go codebase). SQLite driver is `modernc.org/sqlite`. |
| `socket.io` | `github.com/zishang520/socket.io/v2` | `github.com/zishang520/socket.io/v2` | `direct` | Socket.io server implementation in Go, already in `go.mod`. |
| `sqlite3` | `modernc.org/sqlite` | `modernc.org/sqlite` | `direct` | CGO-free SQLite driver, already in `go.mod`. |
| `ssrf-req-filter` | `github.com/doyensec/safeurl` | `github.com/doyensec/safeurl` | `direct` | SSRF and DNS-rebinding protection wrapper for `net/http` client. |
| `string_decoder` | Standard Library | `unicode/utf8` | `stdlib` | Standard `unicode/utf8` package handles multi-byte UTF-8 character decoding. |
| `ts-node` | N/A (Language Feature) | N/A | `stdlib` | N/A. Go compiles directly to native binaries. |
| `typical` | N/A (Language Feature) | N/A | `stdlib` | N/A. Go has static compile-time typing. |
| `umzug` | Standard Library / `github.com/golang-migrate/migrate/v4` | `database/sql` / `github.com/golang-migrate/migrate/v4` | `partial` | **Blocker**: Go does not have a single standard library migration tool matching `umzug`. Custom migrations can be written using SQL script executions in `database/sql`, or using a third-party module like `golang-migrate`. |
| `uuid` | `github.com/google/uuid` | `github.com/google/uuid` | `direct` | UUID generation package, already in `go.mod`. |
| `worker_threads` | N/A (Language Feature) | N/A | `stdlib` | Handled natively by goroutines and channels. |
| `xml2js` | Standard Library | `encoding/xml` | `stdlib` | Standard `encoding/xml` handles XML serialization and deserialization. |
