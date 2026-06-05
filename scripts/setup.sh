#!/bin/bash

# Setup script for cargo.mleczki.pl development environment
# Usage: bash scripts/setup.sh

set -e

echo "🚀 Setting up cargo.mleczki.pl development environment..."

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Check Go installation
echo -e "${YELLOW}Checking Go installation...${NC}"
if ! command -v go &> /dev/null; then
    echo -e "${RED}Go is not installed. Please install Go 1.26.4 or later${NC}"
    echo "Visit: https://golang.org/dl/"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}')
echo -e "${GREEN}✓ Go installed: $GO_VERSION${NC}"

# Check SQLite
echo -e "${YELLOW}Checking SQLite installation...${NC}"
if ! command -v sqlite3 &> /dev/null; then
    echo -e "${RED}SQLite 3 is not installed${NC}"
    if [[ "$OSTYPE" == "darwin"* ]]; then
        echo "Install with: brew install sqlite"
    elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
        echo "Install with: sudo apt-get install sqlite3"
    fi
    exit 1
fi

SQLITE_VERSION=$(sqlite3 --version | awk '{print $1}')
echo -e "${GREEN}✓ SQLite installed: $SQLITE_VERSION${NC}"

# Download Go dependencies
echo -e "${YELLOW}Downloading Go dependencies...${NC}"
go mod download
echo -e "${GREEN}✓ Dependencies downloaded${NC}"

# Check for optional tools
echo ""
echo -e "${YELLOW}Checking optional development tools...${NC}"

# Check air (hot reload)
if command -v air &> /dev/null; then
    echo -e "${GREEN}✓ air installed (hot reload)${NC}"
else
    echo -e "${YELLOW}✗ air not installed${NC}"
    echo "  Install with: go install github.com/cosmtrek/air@latest"
    echo "  Then run: make dev"
fi

# Check golangci-lint
if command -v golangci-lint &> /dev/null; then
    echo -e "${GREEN}✓ golangci-lint installed (linting)${NC}"
else
    echo -e "${YELLOW}✗ golangci-lint not installed${NC}"
    echo "  Install with: curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b \$(go env GOPATH)/bin"
    echo "  Then run: make lint"
fi

# Check gosec
if command -v gosec &> /dev/null; then
    echo -e "${GREEN}✓ gosec installed (security checks)${NC}"
else
    echo -e "${YELLOW}✗ gosec not installed${NC}"
    echo "  Install with: go install github.com/securego/gosec/v2/cmd/gosec@latest"
    echo "  Then run: make security"
fi

# Create database directory if it doesn't exist
echo -e "${YELLOW}Setting up database directory...${NC}"
mkdir -p db
echo -e "${GREEN}✓ Database directory ready${NC}"

# Run initial tests
echo ""
echo -e "${YELLOW}Running initial tests...${NC}"
if go test -v ./... -timeout 5s 2>/dev/null; then
    echo -e "${GREEN}✓ All tests passed${NC}"
else
    echo -e "${YELLOW}⚠ Some tests failed or timed out${NC}"
fi

echo ""
echo -e "${GREEN}✅ Setup complete!${NC}"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo "1. Start development server with hot reload:"
echo "   make dev"
echo ""
echo "2. Or run without hot reload:"
echo "   make run"
echo ""
echo "3. Run QA checks:"
echo "   make qa"
echo ""
echo "4. View available commands:"
echo "   make help"
echo ""
echo "📖 For more information, see CONTRIBUTING.md"

