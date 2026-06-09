# Architectural & Dependency Decisions

## 1. Authentication Framework
* **Decision:** Replace Node.js `passport`, `passport-jwt`, and `passport-strategy` with standard idiomatic Go `net/http` middleware (`AuthMiddleware`) using `github.com/golang-jwt/jwt/v5` for validation.
* **Rationale:** Go does not have a monolithic authentication framework equivalent to Passport. Standard `http.Handler` chaining is highly performant, type-safe, and idiomatic for Go web applications.
* **Interface Impact:** Authenticators and authorization guards will be written as HTTP middleware functions that inject `*UserSession` into the request context (`context.Context`), rather than passing strategies/callbacks to a Passport instance.

## 2. Database Layer
* **Decision:** Map Node.js `sequelize` ORM to standard library `database/sql` combined with the CGO-free `modernc.org/sqlite` driver.
* **Rationale:** The Go port has been built using raw SQLite queries directly inside controllers/handlers, avoiding heavy ORM dependencies. This ensures high-performance, predictable SQLite queries and avoids the overhead of mapping Go structs through complex ORM layers.
* **Interface Impact:** All database querying interfaces must accept `*sql.DB` or `context.Context` and execute explicit SQL queries rather than using active-record pattern methods.

## 3. Session Management
* **Decision:** Map `express-session` to `github.com/alexedwards/scs/v2`.
* **Rationale:** `scs` is the community standard for modern Go session management, offering top performance, type-safe context propagation, and modular database session storage.
* **Interface Impact:** Session access will be routed through `scs.SessionManager` middleware, using context keys to read/write session data.

## 4. SSRF Prevention
* **Decision:** Map `ssrf-req-filter` to `github.com/doyensec/safeurl` to protect outgoing metadata/provider requests.
* **Rationale:** `safeurl` functions as a direct wrapper/replacement for `net/http.Client` to protect connections from SSRF and DNS rebinding attacks at the socket dialing layer.
* **Interface Impact:** All external HTTP clients (e.g. for Audibletags or other metadata search providers) must be instantiated through `safeurl.Client()` or use safe transport configurations.

## 5. Database Migrations
* **Decision:** Map `umzug` to custom SQL script runner or `github.com/golang-migrate/migrate/v4`.
* **Rationale:** Since the Go port doesn't have a direct equivalent of Sequelize + Umzug, migrations must be executed either via standard Go sql script executors at startup or using a dedicated migrator module.
* **Interface Impact:** Migrations interface will run on startup using defined `.sql` schema migration files, rather than javascript modules.
