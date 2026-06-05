# Development Quick Reference

## Daily Tasks

### Start Development
```bash
# With hot reload (recommended)
make dev

# Without hot reload
make run

# View at http://localhost:8080
```

### Before Committing
```bash
# Run all QA checks
make qa

# Or fix formatting and run tests
make fmt
make test
```

### After Making Changes
```bash
# Test your specific package
go test -v ./internal/your-package

# Check specific test
go test -v -run TestName ./internal/package
```

## Common Commands

| Task | Command |
|------|---------|
| **Development** | |
| Run with hot reload | `make dev` |
| Run normally | `make run` |
| Build binary | `make build` |
| **Testing** | |
| Run all tests | `make test` |
| Tests with coverage | `make test-coverage` |
| Specific test | `go test -v -run TestName ./path` |
| **Code Quality** | |
| Format code | `make fmt` |
| Check formatting | `make fmt-check` |
| Lint code | `make lint` |
| Security check | `make security` |
| Go vet analysis | `make vet` |
| **All checks** | `make qa` |
| **Database** | |
| Clean databases | `make clean` |
| Query events | `sqlite3 db/event_store.db` |
| Query read models | `sqlite3 db/read_models.db` |
| **Docker** | |
| Build image | `make docker-build` |
| Run container | `make docker-run` |
| **Help** | |
| Show all targets | `make help` |

## Debugging

### Enable verbose output
```go
log.Printf("Debug: %+v", variable)
```

### Inspect SQLite databases
```bash
# Event store
sqlite3 db/event_store.db
> .tables
> SELECT COUNT(*) FROM events;
> SELECT * FROM events ORDER BY id DESC LIMIT 5;

# Read models
sqlite3 db/read_models.db
> .tables
> SELECT * FROM orders LIMIT 5;
```

### Clear data and restart
```bash
make clean
make run
```

## File Structure Quick Reference

```
cmd/server/
  └── main.go              ← Entry point
  └── server.go            ← HTTP handlers

internal/domain/
  ├── order.go             ← Order aggregate
  ├── user.go              ← User aggregate
  ├── transfer.go          ← Transfer aggregate
  └── product.go           ← Product model

internal/eventstore/
  ├── store.go             ← Interface
  ├── sqlite_store.go      ← SQLite implementation
  └── event.go             ← Event model

internal/projections/
  ├── projector.go         ← Event processor
  ├── read_models.go       ← Read model database
  └── projector_test.go    ← Tests

internal/products/
  ├── parser.go            ← Markdown parser
  └── parser_test.go       ← Tests

web/
  ├── templates/           ← HTML templates
  └── static/             ← CSS, JS files

data/products/
  ├── cargo.md             ← Product definitions
  └── ...
```

## Common Issues & Solutions

### Hot reload not working
```bash
rm -rf tmp/
make dev
```

### Tests timeout
```bash
go test -v -timeout 30s ./...
```

### Port 8080 already in use
```bash
# Kill process on port 8080
lsof -ti:8080 | xargs kill -9

# Or use different port
PORT=8081 go run ./cmd/server
```

### SQLite database locked
```bash
# Close all sqlite connections and try again
make clean
make run
```

### Import cycle errors
- Avoid circular imports between packages
- Move shared types to `domain/` package
- Use interfaces for decoupling

## Git Workflow

```bash
# Create feature branch
git checkout -b feature/my-feature

# Make changes and test
make dev
make qa

# Commit with conventional format
git commit -m "feat: add new feature"

# Push to origin
git push origin feature/my-feature

# Create pull request on GitHub
# CI/CD will verify:
#   ✓ Tests pass
#   ✓ Code is formatted
#   ✓ Linting passes
#   ✓ Security checks pass
#   ✓ Build succeeds
```

## Performance Tips

### Local development
- Use `make dev` for hot reload (faster development cycle)
- Keep database files in `db/` (they're git-ignored)
- Tests run with `-race` flag to catch race conditions

### Before deployment
- Run `make qa` to ensure all checks pass
- Review test coverage: `make test-coverage`
- Check for security issues: `make security`
- Verify build: `make build`

## Resources

- 📖 [CONTRIBUTING.md](./CONTRIBUTING.md) - Full contribution guide
- 📖 [README.md](./README.md) - Project overview
- 🔧 [Makefile](./Makefile) - Build automation
- ⚙️ [.golangci.yml](./.golangci.yml) - Linter configuration
- ⚙️ [.github/workflows/ci.yml](./.github/workflows/ci.yml) - CI/CD configuration

---

**Happy coding!** 🚀

For more detailed information, see [CONTRIBUTING.md](./CONTRIBUTING.md)

