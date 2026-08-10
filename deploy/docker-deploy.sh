#!/bin/bash
# =============================================================================
# Sub2API Docker Deployment Preparation Script
# =============================================================================
# This script prepares deployment files for Sub2API:
#   - Downloads docker-compose.local.yml and .env.example
#   - Generates secure secrets (JWT_SECRET, TOTP_ENCRYPTION_KEY, POSTGRES_PASSWORD)
#   - Creates necessary data directories
#
# After running this script, you can start services with:
#   docker compose up -d
# =============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Private deployment source. UPDATE_GITHUB_TOKEN only needs read-only Contents
# access. UPDATE_GITHUB_REF can be pinned to a reviewed private release tag.
UPDATE_REPOSITORY="${UPDATE_REPOSITORY:-hxly520/sub2api}"
UPDATE_GITHUB_REF="${UPDATE_GITHUB_REF:-main}"
GITHUB_API_ROOT="https://api.github.com"
GITHUB_API_VERSION="2022-11-28"

# Print colored message
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Generate random secret
generate_secret() {
    openssl rand -hex 32
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

validate_source() {
    if [[ ! "$UPDATE_REPOSITORY" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
        print_error "Invalid UPDATE_REPOSITORY: $UPDATE_REPOSITORY"
        return 2
    fi
    if [[ ! "$UPDATE_GITHUB_REF" =~ ^[A-Za-z0-9._/-]+$ ]] || [[ "$UPDATE_GITHUB_REF" == *".."* ]]; then
        print_error "Invalid UPDATE_GITHUB_REF: $UPDATE_GITHUB_REF"
        return 2
    fi
    if [ -n "${UPDATE_GITHUB_TOKEN:-}" ] && [[ ! "$UPDATE_GITHUB_TOKEN" =~ ^[A-Za-z0-9_.-]+$ ]]; then
        print_error "UPDATE_GITHUB_TOKEN contains unsupported characters"
        return 2
    fi
}

download_repository_file() {
    local repository_path="$1"
    local output="$2"
    local url="${GITHUB_API_ROOT}/repos/${UPDATE_REPOSITORY}/contents/${repository_path}?ref=${UPDATE_GITHUB_REF}"

    validate_source || return $?
    {
        printf 'header = "Accept: application/vnd.github.raw+json"\n'
        printf 'header = "X-GitHub-Api-Version: %s"\n' "$GITHUB_API_VERSION"
        printf 'header = "User-Agent: sub2api-private-deployer"\n'
        if [ -n "${UPDATE_GITHUB_TOKEN:-}" ]; then
            printf 'header = "Authorization: Bearer %s"\n' "$UPDATE_GITHUB_TOKEN"
        fi
    } | env UPDATE_GITHUB_TOKEN='' GITHUB_TOKEN='' GH_TOKEN='' curl \
        -q --globoff --fail --silent --show-error \
        --proto '=https' --connect-timeout 10 --max-time 60 \
        --config - --output "$output" --url "$url"
}

# Main installation function
main() {
    local download_temp
    echo ""
    echo "=========================================="
    echo "  Sub2API Deployment Preparation"
    echo "=========================================="
    echo ""

    # Check if openssl is available
    if ! command_exists openssl; then
        print_error "openssl is not installed. Please install openssl first."
        exit 1
    fi
    if ! command_exists curl; then
        print_error "curl is not installed. Please install curl first."
        exit 1
    fi
    validate_source

    # Check if deployment already exists
    if [ -f "docker-compose.yml" ] || [ -f ".env" ] || [ -f ".env.example" ]; then
        print_warning "Deployment files already exist in current directory."
        read -p "Overwrite existing files? (y/N): " -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            print_info "Cancelled."
            exit 0
        fi
    fi

    download_temp=$(mktemp -d "./.sub2api-deploy.XXXXXX")
    trap 'rm -rf -- "$download_temp"' EXIT

    # Download docker-compose.local.yml and save as docker-compose.yml
    print_info "Downloading docker-compose.yml..."
    if ! download_repository_file "deploy/docker-compose.local.yml" "$download_temp/docker-compose.yml"; then
        print_error "Failed to download docker-compose.local.yml. Check UPDATE_REPOSITORY, UPDATE_GITHUB_REF, and UPDATE_GITHUB_TOKEN."
        exit 1
    fi

    # Download .env.example
    print_info "Downloading .env.example..."
    if ! download_repository_file "deploy/.env.example" "$download_temp/.env.example"; then
        print_error "Failed to download .env.example. Check private repository access."
        exit 1
    fi
    if ! grep -Eq '^services:' "$download_temp/docker-compose.yml" || ! grep -Eq '^SUB2API_IMAGE=' "$download_temp/.env.example"; then
        print_error "Downloaded deployment files failed content validation."
        exit 1
    fi
    mv -f "$download_temp/docker-compose.yml" "docker-compose.yml"
    mv -f "$download_temp/.env.example" ".env.example"
    rmdir "$download_temp"
    trap - EXIT
    print_success "Downloaded docker-compose.yml"
    print_success "Downloaded .env.example"

    # Generate .env file with auto-generated secrets
    print_info "Generating secure secrets..."
    echo ""

    # Generate secrets
    JWT_SECRET=$(generate_secret)
    TOTP_ENCRYPTION_KEY=$(generate_secret)
    POSTGRES_PASSWORD=$(generate_secret)

    # Create .env from .env.example
    cp .env.example .env

    # Update .env with generated secrets (cross-platform compatible)
    if sed --version >/dev/null 2>&1; then
        # GNU sed (Linux)
        sed -i "s/^JWT_SECRET=.*/JWT_SECRET=${JWT_SECRET}/" .env
        sed -i "s/^TOTP_ENCRYPTION_KEY=.*/TOTP_ENCRYPTION_KEY=${TOTP_ENCRYPTION_KEY}/" .env
        sed -i "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=${POSTGRES_PASSWORD}/" .env
        sed -i "s|^UPDATE_REPOSITORY=.*|UPDATE_REPOSITORY=${UPDATE_REPOSITORY}|" .env
        if [ -n "${UPDATE_GITHUB_TOKEN:-}" ]; then
            sed -i "s/^UPDATE_GITHUB_TOKEN=.*/UPDATE_GITHUB_TOKEN=${UPDATE_GITHUB_TOKEN}/" .env
        fi
    else
        # BSD sed (macOS)
        sed -i '' "s/^JWT_SECRET=.*/JWT_SECRET=${JWT_SECRET}/" .env
        sed -i '' "s/^TOTP_ENCRYPTION_KEY=.*/TOTP_ENCRYPTION_KEY=${TOTP_ENCRYPTION_KEY}/" .env
        sed -i '' "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=${POSTGRES_PASSWORD}/" .env
        sed -i '' "s|^UPDATE_REPOSITORY=.*|UPDATE_REPOSITORY=${UPDATE_REPOSITORY}|" .env
        if [ -n "${UPDATE_GITHUB_TOKEN:-}" ]; then
            sed -i '' "s/^UPDATE_GITHUB_TOKEN=.*/UPDATE_GITHUB_TOKEN=${UPDATE_GITHUB_TOKEN}/" .env
        fi
    fi

    # Create data directories
    print_info "Creating data directories..."
    mkdir -p data postgres_data redis_data
    print_success "Created data directories"

    # Set secure permissions for .env file (readable/writable only by owner)
    chmod 600 .env
    echo ""

    # Display completion message
    echo "=========================================="
    echo "  Preparation Complete!"
    echo "=========================================="
    echo ""
    echo "Generated secure credentials:"
    echo "  POSTGRES_PASSWORD:     ${POSTGRES_PASSWORD}"
    echo "  JWT_SECRET:            ${JWT_SECRET}"
    echo "  TOTP_ENCRYPTION_KEY:   ${TOTP_ENCRYPTION_KEY}"
    echo ""
    print_warning "These credentials have been saved to .env file."
    print_warning "Please keep them secure and do not share publicly!"
    echo ""
    echo "Directory structure:"
    echo "  docker-compose.yml        - Docker Compose configuration"
    echo "  .env                      - Environment variables (generated secrets)"
    echo "  .env.example              - Example template (for reference)"
    echo "  data/                     - Application data (will be created on first run)"
    echo "  postgres_data/            - PostgreSQL data"
    echo "  redis_data/               - Redis data"
    echo ""
    echo "Next steps:"
    echo "  1. (Optional) Edit .env to customize configuration"
    echo "  2. Start services:"
    echo "     docker compose up -d"
    echo ""
    echo "  3. View logs:"
    echo "     docker compose logs -f sub2api"
    echo ""
    echo "  4. Access Web UI:"
    echo "     http://localhost:8080"
    echo ""
    print_info "If admin password is not set in .env, it will be auto-generated."
    print_info "Check logs for the generated admin password on first startup."
    echo ""
}

# Run main function
main "$@"
