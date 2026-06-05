# 🚀 GitHub Actions & QA Setup - Getting Started Guide

## What Was Added

### 📋 New Files Created

```
✅ .github/workflows/ci.yml      - GitHub Actions CI/CD pipeline
✅ .golangci.yml                  - Linter configuration
✅ .air.toml                      - Hot-reload configuration
✅ scripts/setup.sh               - Developer setup script
✅ CONTRIBUTING.md                - Contribution guidelines
✅ QUICKSTART.md                  - Daily command reference
✅ QA_SETUP.md                    - Complete setup documentation
```

### 📝 Files Modified

```
✅ Makefile                       - Enhanced with QA targets
✅ README.md                      - Added dev & QA sections
✅ .gitignore                     - Added development artifacts
```

## 🎯 Quick Start (3 Steps)

### Step 1: Initial Setup (One Time)
```bash
cd cargo.mleczki.pl
bash scripts/setup.sh
```

This will:
- ✓ Check Go and SQLite installations
- ✓ Download dependencies
- ✓ Suggest optional tools (air, golangci-lint, gosec)

### Step 2: Start Development
```bash
make dev
```

App runs with hot-reload on `http://localhost:8080`

### Step 3: Before Committing
```bash
make qa
```

This runs:
- ✓ Code formatting checks
- ✓ Go vet analysis
- ✓ Unit tests
- ✓ Linting

## 📚 Documentation Navigation

| Document | Purpose | Audience |
|----------|---------|----------|
| **[QUICKSTART.md](QUICKSTART.md)** | Daily command reference | All developers |
| **[CONTRIBUTING.md](CONTRIBUTING.md)** | Full contribution guide | New contributors |
| **[QA_SETUP.md](QA_SETUP.md)** | Detailed setup info | DevOps/Leads |
| **[README.md](README.md)** | Project overview | Everyone |

## 🛠️ Daily Workflow

```bash
# Morning: Start development with hot reload
make dev

# During work: Changes auto-reload, tests on save

# Before commit: Run QA checks
make qa

# Commit with conventional format
git commit -m "feat: add new feature"

# Push to GitHub
git push origin feature/branch

# CI/CD Pipeline Runs Automatically:
# ✓ Tests pass
# ✓ Code is formatted
# ✓ Lint passes
# ✓ Security checks pass
# ✓ Build succeeds
```

## 🔧 Make Commands Reference

### Regular Dev
- `make help` - Show all commands
- `make dev` - Run with hot reload
- `make run` - Run without hot reload
- `make build` - Build binary

### Testing & QA
- `make test` - Run all tests
- `make test-coverage` - Tests + HTML coverage
- `make qa` - All QA checks

### Code Quality
- `make fmt` - Auto-format code
- `make fmt-check` - Check formatting
- `make vet` - Go vet analysis
- `make lint` - Run golangci-lint
- `make security` - Run gosec

### Database & Cleanup
- `make clean` - Clean artifacts and databases

### Docker
- `make docker-build` - Build Docker image
- `make docker-run` - Run container

## 🔍 GitHub Actions CI/CD

Automatically runs on:
- Every push to `main` or `develop`
- Every pull request to `main` or `develop`

### What It Checks

```
┌─────────────────────────────────────────┐
│        GitHub Actions Pipeline           │
├─────────────────────────────────────────┤
│                                         │
│  ✓ Tests          (with race detection) │
│  ✓ Lint           (golangci-lint)       │
│  ✓ Format Check   (gofmt)               │
│  ✓ Security       (gosec)               │
│                                         │
│  ↓ (if all pass)                        │
│                                         │
│  ✓ Build          (compile binary)      │
│  ✓ Docker         (build image)         │
│                                         │
└─────────────────────────────────────────┘
```

### View Results on GitHub

1. Go to your repository
2. Click **Actions** tab
3. Select workflow run
4. Click specific job to see details

## 📦 Optional Tools

These tools are optional but recommended:

### Air (Hot Reload)
```bash
go install github.com/cosmtrek/air@latest
# Then: make dev
```

### golangci-lint (Linting)
```bash
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
# Then: make lint
```

### gosec (Security)
```bash
go install github.com/securego/gosec/v2/cmd/gosec@latest
# Then: make security
```

### act (Test CI Locally)
```bash
brew install act           # macOS
# Then: act -j test       # Run tests job locally
```

## ❓ Common Questions

### Q: My hot reload isn't working?
```bash
rm -rf tmp/
make dev
```

### Q: How do I run just one test?
```bash
go test -v -run TestName ./internal/package
```

### Q: Can I test the CI pipeline locally?
```bash
# Install act: brew install act
act -j test              # Test job only
act                      # All jobs
```

### Q: How do I add a new Go module?
```bash
go get github.com/username/module@v1.0.0
go mod tidy
```

### Q: Where's my test coverage?
```bash
make test-coverage
# Opens coverage.html in browser
```

## 🚀 First Commit Checklist

Before your first PR:

- [ ] Run `bash scripts/setup.sh`
- [ ] Test with `make dev`
- [ ] Read `QUICKSTART.md`
- [ ] Study code structure
- [ ] Make your changes
- [ ] Run `make qa` - everything passes?
- [ ] Commit with `git commit -m "feat: ..."`
- [ ] Push to GitHub
- [ ] Create Pull Request
- [ ] CI/CD pipeline confirms everything works

## 📞 Need Help?

1. **For daily tasks** → See `QUICKSTART.md`
2. **For setting up** → Run `bash scripts/setup.sh`
3. **For contributing** → See `CONTRIBUTING.md`
4. **For CI/CD details** → See `README.md` → GitHub Actions section
5. **For complete setup info** → See `QA_SETUP.md`

## ✨ You're All Set!

```bash
# Ready to code?
bash scripts/setup.sh
make dev
```

Happy coding! 🎉

---

**Last Updated:** June 5, 2026  
**For more details:** See other documentation files in the repository

