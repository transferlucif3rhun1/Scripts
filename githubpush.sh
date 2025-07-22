#!/bin/bash

set -euo pipefail

SCRIPT_NAME=$(basename "$0")

if [[ -L "$0" ]]; then
    if command -v readlink >/dev/null 2>&1; then
        SCRIPT_DIR=$(dirname "$(readlink -f "$0" 2>/dev/null || readlink "$0")")
    else
        SCRIPT_DIR=$(dirname "$0")
    fi
else
    SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
fi

LFS_CONFIGURED=false
RETRY_COUNT=0
MAX_RETRIES=3
MAIN_TOKEN_VALIDATED=false
BACKUP_TOKEN_VALIDATED=false

cleanup() {
    local exit_code=$?
    exit $exit_code
}

trap cleanup EXIT INT TERM

COMMIT_MESSAGE=""
VERBOSE=false
DRY_RUN=false

usage() {
    cat >&2 << EOF
Usage: $SCRIPT_NAME [OPTIONS]

Options:
    -m MESSAGE    Specify commit message
    -v            Enable verbose output
    -d            Dry run mode (no changes)
    -h            Show this help message

Examples:
    $SCRIPT_NAME -m "Update documentation"
    $SCRIPT_NAME -v -d
EOF
    exit "${1:-0}"
}

while getopts "m:vdh" opt; do
    case $opt in
        m) COMMIT_MESSAGE="$OPTARG";;
        v) VERBOSE=true;;
        d) DRY_RUN=true;;
        h) usage 0;;
        *) usage 1;;
    esac
done

shift $((OPTIND-1))

REPO_NAME=$(basename "$(pwd)")

if [[ ! "$REPO_NAME" =~ ^[a-zA-Z0-9][a-zA-Z0-9._-]*[a-zA-Z0-9]$|^[a-zA-Z0-9]$ ]]; then
    echo "Error: Invalid repository name: $REPO_NAME" >&2
    exit 1
fi

if [[ ${#REPO_NAME} -gt 100 ]]; then
    echo "Error: Repository name too long: $REPO_NAME" >&2
    exit 1
fi

MAIN_USER="${GITHUB_MAIN_USER:-lucif3rhun1}"
BACKUP_USER="${GITHUB_BACKUP_USER:-transferlucif3rhun1}"
MAIN_REMOTE="${GITHUB_MAIN_REMOTE:-origin}"
BACKUP_REMOTE="${GITHUB_BACKUP_REMOTE:-backup}"
MAIN_TOKEN=""
BACKUP_TOKEN=""
LARGE_REPO_THRESHOLD_GB=1
API_VERSION="2022-11-28"

if [[ -t 2 ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    PURPLE='\033[0;35m'
    NC='\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    PURPLE=''
    NC=''
fi

log_error() {
    echo -e "${RED}✗ $*${NC}" >&2
}

log_success() {
    echo -e "${GREEN}✓ $*${NC}" >&2
}

log_warning() {
    echo -e "${YELLOW}⚠ $*${NC}" >&2
}

log_info() {
    echo -e "${BLUE}ℹ $*${NC}" >&2
}

log_verbose() {
    if [[ "$VERBOSE" == true ]]; then
        echo -e "  $*" >&2
    fi
}

error_exit() {
    log_error "$1"
    exit "${2:-1}"
}

run_with_timeout() {
    local timeout_seconds=$1
    shift
    local cmd=("$@")
    
    if command -v timeout >/dev/null 2>&1; then
        timeout "$timeout_seconds" "${cmd[@]}"
    elif command -v gtimeout >/dev/null 2>&1; then
        gtimeout "$timeout_seconds" "${cmd[@]}"
    else
        "${cmd[@]}" &
        local pid=$!
        local count=0
        while kill -0 "$pid" 2>/dev/null && [[ $count -lt $timeout_seconds ]]; do
            sleep 1
            ((count++))
        done
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null
            wait "$pid" 2>/dev/null
            return 124
        fi
        wait "$pid"
    fi
}

validate_github_token() {
    local token=$1
    if [[ "$token" =~ ^ghp_[a-zA-Z0-9]{36}$ ]]; then
        return 0
    elif [[ "$token" =~ ^github_pat_[a-zA-Z0-9]{22}_[a-zA-Z0-9]{59}$ ]]; then
        return 0
    else
        return 1
    fi
}

extract_rate_limit_info() {
    local response_headers=$1
    local remaining=""
    local reset=""
    local retry_after=""
    
    remaining=$(echo "$response_headers" | grep -i "x-ratelimit-remaining:" | head -1 | cut -d: -f2 | tr -d ' \r\n' || echo "")
    reset=$(echo "$response_headers" | grep -i "x-ratelimit-reset:" | head -1 | cut -d: -f2 | tr -d ' \r\n' || echo "")
    retry_after=$(echo "$response_headers" | grep -i "retry-after:" | head -1 | cut -d: -f2 | tr -d ' \r\n' || echo "")
    
    echo "$remaining|$reset|$retry_after"
}

handle_rate_limit() {
    local rate_info=$1
    local remaining=$(echo "$rate_info" | cut -d'|' -f1)
    local reset=$(echo "$rate_info" | cut -d'|' -f2)
    local retry_after=$(echo "$rate_info" | cut -d'|' -f3)
    
    if [[ -n "$retry_after" && "$retry_after" -gt 0 ]]; then
        log_warning "Rate limited, waiting $retry_after seconds"
        if [[ "$DRY_RUN" == false ]]; then
            sleep "$retry_after"
        fi
        return 0
    fi
    
    if [[ -n "$remaining" && "$remaining" -le 10 ]]; then
        local wait_time=60
        if [[ -n "$reset" ]]; then
            local current_time=$(date +%s)
            wait_time=$((reset - current_time + 5))
            if [[ $wait_time -le 0 ]]; then
                wait_time=60
            fi
        fi
        log_warning "Rate limit low ($remaining remaining), waiting $wait_time seconds"
        if [[ "$DRY_RUN" == false ]]; then
            sleep "$wait_time"
        fi
    fi
}

make_github_request() {
    local method=$1
    local url=$2
    local token=$3
    local data=${4:-""}
    local max_attempts=3
    local attempt=1
    local wait_time=1
    
    while [[ $attempt -le $max_attempts ]]; do
        local response
        local http_code
        local rate_info
        local headers_file="/tmp/github_headers_$$"
        
        log_verbose "GitHub API Request (attempt $attempt/$max_attempts):"
        log_verbose "  Method: $method"
        log_verbose "  URL: $url"
        log_verbose "  Has data: $([[ -n "$data" ]] && echo 'yes' || echo 'no')"
        log_verbose "  Token: ${token:0:10}..."
        
        if [[ -n "$data" ]]; then
            log_verbose "  Request body: $data"
            if [[ "$VERBOSE" == true ]]; then
                local debug_output="/tmp/curl_debug_$"
                response=$(curl -v -s -w "\n%{http_code}" -X "$method" \
                    -H "Authorization: Bearer $token" \
                    -H "Accept: application/vnd.github+json" \
                    -H "Content-Type: application/json" \
                    -H "X-GitHub-Api-Version: $API_VERSION" \
                    -H "User-Agent: github-dual-sync/1.0" \
                    -D "$headers_file" \
                    -d "$data" \
                    "$url" 2>"$debug_output" || echo -e "\n000")
                if [[ -f "$debug_output" ]]; then
                    log_verbose "Curl debug output:"
                    while IFS= read -r line; do
                        log_verbose "  $line"
                    done < "$debug_output"
                    rm -f "$debug_output"
                fi
            else
                response=$(curl -s -w "\n%{http_code}" -X "$method" \
                    -H "Authorization: Bearer $token" \
                    -H "Accept: application/vnd.github+json" \
                    -H "Content-Type: application/json" \
                    -H "X-GitHub-Api-Version: $API_VERSION" \
                    -H "User-Agent: github-dual-sync/1.0" \
                    -D "$headers_file" \
                    -d "$data" \
                    "$url" 2>/dev/null || echo -e "\n000")
            fi
        else
            if [[ "$VERBOSE" == true ]]; then
                local debug_output="/tmp/curl_debug_$"
                response=$(curl -v -s -w "\n%{http_code}" -X "$method" \
                    -H "Authorization: Bearer $token" \
                    -H "Accept: application/vnd.github+json" \
                    -H "X-GitHub-Api-Version: $API_VERSION" \
                    -H "User-Agent: github-dual-sync/1.0" \
                    -D "$headers_file" \
                    "$url" 2>"$debug_output" || echo -e "\n000")
                if [[ -f "$debug_output" ]]; then
                    log_verbose "Curl debug output:"
                    while IFS= read -r line; do
                        log_verbose "  $line"
                    done < "$debug_output"
                    rm -f "$debug_output"
                fi
            else
                response=$(curl -s -w "\n%{http_code}" -X "$method" \
                    -H "Authorization: Bearer $token" \
                    -H "Accept: application/vnd.github+json" \
                    -H "X-GitHub-Api-Version: $API_VERSION" \
                    -H "User-Agent: github-dual-sync/1.0" \
                    -D "$headers_file" \
                    "$url" 2>/dev/null || echo -e "\n000")
            fi
        fi
        
        http_code="${response##*$'\n'}"
        response="${response%$'\n'*}"
        
        log_verbose "  HTTP Status: $http_code"
        
        if [[ -f "$headers_file" ]]; then
            rate_info=$(extract_rate_limit_info "$(cat "$headers_file")")
            rm -f "$headers_file"
        else
            rate_info="||"
        fi
        
        case "$http_code" in
            200|201|204)
                echo "$response"
                return 0
                ;;
            304)
                echo ""
                return 0
                ;;
            401)
                log_error "Authentication failed. Check token validity and scopes."
                log_verbose "Full response: $response"
                return 1
                ;;
            403)
                if echo "$response" | grep -q "rate limit\|API rate limit"; then
                    handle_rate_limit "$rate_info"
                    ((attempt++))
                    continue
                else
                    log_error "Permission denied. Check token scopes."
                    log_verbose "Full response: $response"
                    return 1
                fi
                ;;
            404)
                echo "NOT_FOUND"
                return 0
                ;;
            409)
                log_error "Conflict: Resource already exists or concurrent modification"
                log_verbose "Full response: $response"
                return 1
                ;;
            410)
                log_error "Resource no longer available"
                log_verbose "Full response: $response"
                return 1
                ;;
            422)
                local error_msg=$(echo "$response" | grep -o '"message":"[^"]*"' | cut -d'"' -f4 || echo "Validation failed")
                log_error "Validation failed: $error_msg"
                log_verbose "Full response: $response"
                return 1
                ;;
            429)
                handle_rate_limit "$rate_info"
                ((attempt++))
                continue
                ;;
            500|502|503|504)
                log_warning "Server error (HTTP $http_code), retrying in $wait_time seconds"
                log_verbose "Full response: $response"
                if [[ "$DRY_RUN" == false ]]; then
                    sleep "$wait_time"
                fi
                wait_time=$((wait_time * 2))
                ((attempt++))
                continue
                ;;
            000)
                log_warning "Network error, retrying in $wait_time seconds"
                log_verbose "Curl output: $response"
                if [[ "$DRY_RUN" == false ]]; then
                    sleep "$wait_time"
                fi
                wait_time=$((wait_time * 2))
                ((attempt++))
                continue
                ;;
            *)
                log_error "Unexpected HTTP code: $http_code"
                log_verbose "Full response: $response"
                return 1
                ;;
        esac
    done
    
    log_error "Request failed after $max_attempts attempts"
    return 1
}

check_and_install_dependencies() {
    local missing_deps=()
    local required_commands=("git" "curl")
    
    for cmd in "${required_commands[@]}"; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            missing_deps+=("$cmd")
        fi
    done
    
    if [[ ${#missing_deps[@]} -gt 0 ]]; then
        log_warning "Missing required dependencies: ${missing_deps[*]}"
        
        if [[ "$DRY_RUN" == false ]]; then
            echo "├── Installing missing dependencies..." >&2
            
            for dep in "${missing_deps[@]}"; do
                log_verbose "Installing $dep"
                
                if command -v brew >/dev/null 2>&1; then
                    brew install "$dep" >/dev/null 2>&1
                elif command -v apt-get >/dev/null 2>&1; then
                    sudo apt-get update >/dev/null 2>&1
                    sudo apt-get install -y "$dep" >/dev/null 2>&1
                elif command -v yum >/dev/null 2>&1; then
                    sudo yum install -y "$dep" >/dev/null 2>&1
                elif command -v pacman >/dev/null 2>&1; then
                    sudo pacman -S --noconfirm "$dep" >/dev/null 2>&1
                else
                    log_error "Cannot auto-install $dep. Please install manually."
                    return 1
                fi
                
                if command -v "$dep" >/dev/null 2>&1; then
                    log_verbose "$dep installed successfully"
                else
                    log_error "$dep installation failed"
                    return 1
                fi
            done
            
            log_success "All dependencies installed"
        else
            log_verbose "DRY RUN: Would install dependencies"
        fi
    fi
    
    return 0
}

check_repository_size() {
    local size_gb
    local size_bytes
    
    if command -v du >/dev/null 2>&1; then
        size_bytes=$(du -sb . 2>/dev/null | cut -f1 || echo "0")
        size_gb=$((size_bytes / 1024 / 1024 / 1024))
        
        if [[ $size_gb -gt $LARGE_REPO_THRESHOLD_GB ]]; then
            log_warning "Large repository detected: ${size_gb}GB"
            echo "    This may take longer to sync and could hit GitHub limits" >&2
            
            if [[ "$DRY_RUN" == false ]]; then
                echo -n "    Continue? (y/n): " >&2
                read -r confirm < /dev/tty
                if [[ ! "$confirm" =~ ^[Yy] ]]; then
                    exit 1
                fi
            fi
        fi
    fi
}

setup_git_lfs() {
    if ! command -v git-lfs >/dev/null 2>&1; then
        log_warning "Git LFS not installed. Installing automatically..."
        
        if [[ "$DRY_RUN" == false ]]; then
            if command -v brew >/dev/null 2>&1; then
                log_verbose "Installing Git LFS via Homebrew"
                brew install git-lfs >/dev/null 2>&1
            elif command -v apt-get >/dev/null 2>&1; then
                log_verbose "Installing Git LFS via apt"
                sudo apt-get update >/dev/null 2>&1
                sudo apt-get install -y git-lfs >/dev/null 2>&1
            elif command -v yum >/dev/null 2>&1; then
                log_verbose "Installing Git LFS via yum"
                sudo yum install -y git-lfs >/dev/null 2>&1
            elif command -v pacman >/dev/null 2>&1; then
                log_verbose "Installing Git LFS via pacman"
                sudo pacman -S --noconfirm git-lfs >/dev/null 2>&1
            else
                log_error "Cannot auto-install Git LFS. Please install manually:"
                echo "    macOS: brew install git-lfs" >&2
                echo "    Ubuntu/Debian: sudo apt install git-lfs" >&2
                echo "    RHEL/CentOS: sudo yum install git-lfs" >&2
                echo "    Arch: sudo pacman -S git-lfs" >&2
                return 1
            fi
            
            if command -v git-lfs >/dev/null 2>&1; then
                log_success "Git LFS installed successfully"
            else
                log_error "Git LFS installation failed"
                return 1
            fi
        else
            log_verbose "DRY RUN: Would install Git LFS"
        fi
    fi
    
    if [[ ! -f .gitattributes ]] || ! grep -q "filter=lfs" .gitattributes 2>/dev/null; then
        log_verbose "Initializing Git LFS"
        if [[ "$DRY_RUN" == false ]]; then
            git lfs install --local >/dev/null 2>&1
        fi
    fi
    
    return 0
}

detect_large_files() {
    local size_limit_mb=50
    local size_limit_bytes=$((size_limit_mb * 1024 * 1024))
    local large_files=()
    
    local files_to_check=()
    if [[ -d .git ]]; then
        while IFS= read -r -d '' file; do
            files_to_check+=("$file")
        done < <(git ls-files -z 2>/dev/null)
        
        while IFS= read -r -d '' file; do
            files_to_check+=("$file")
        done < <(git ls-files --others --exclude-standard -z 2>/dev/null)
    else
        while IFS= read -r -d '' file; do
            files_to_check+=("$file")
        done < <(find . -type f -print0 2>/dev/null)
    fi
    
    for file in "${files_to_check[@]}"; do
        if [[ -f "$file" ]] && [[ $(stat -f%z "$file" 2>/dev/null || stat -c%s "$file" 2>/dev/null || echo 0) -gt $size_limit_bytes ]]; then
            large_files+=("$file")
        fi
    done
    
    if [[ ${#large_files[@]} -gt 0 ]]; then
        echo "${large_files[@]}"
        return 0
    fi
    
    return 1
}

handle_large_files() {
    local large_files_detected=false
    
    if large_files=$(detect_large_files); then
        large_files_detected=true
        log_info "Large files detected (>50MB), adding to Git LFS:"
        echo "$large_files" | while read -r file; do
            local size_mb
            size_mb=$(( $(stat -f%z "$file" 2>/dev/null || stat -c%s "$file" 2>/dev/null || echo 0) / 1024 / 1024 ))
            echo "    - $file (${size_mb}MB)" >&2
        done
        
        if setup_git_lfs; then
            if [[ "$DRY_RUN" == false ]]; then
                echo "$large_files" | while read -r file; do
                    git lfs track "$file" >/dev/null 2>&1 || true
                done
                
                if [[ -f .gitattributes ]]; then
                    git add .gitattributes >/dev/null 2>&1 || true
                fi
                
                echo "$large_files" | while read -r file; do
                    git add "$file" >/dev/null 2>&1 || true
                done
                
                if git diff --cached --quiet 2>/dev/null; then
                    log_verbose "No LFS changes to commit"
                else
                    git commit -m "Configure Git LFS for large files" >/dev/null 2>&1 || true
                    log_verbose "Committed LFS configuration"
                fi
            fi
            
            log_success "Large files configured for Git LFS"
            LFS_CONFIGURED=true
        else
            log_error "Cannot configure Git LFS. Large files may cause push failures."
            return 1
        fi
    fi
    
    return 0
}

setup_git_credentials() {
    if [[ -z "$MAIN_TOKEN" || -z "$BACKUP_TOKEN" || "$DRY_RUN" == true ]]; then
        return 0
    fi
    
    local credentials_file="$HOME/.git-credentials"
    local main_entry="https://$MAIN_USER:$MAIN_TOKEN@github.com"
    local backup_entry="https://$BACKUP_USER:$BACKUP_TOKEN@github.com"
    
    log_verbose "Configuring Git credentials for HTTPS authentication"
    log_verbose "Main entry: https://$MAIN_USER:***@github.com"
    log_verbose "Backup entry: https://$BACKUP_USER:***@github.com"
    
    git config --global credential.helper store
    git config --global credential.https://github.com.username "$MAIN_USER"
    
    mkdir -p "$(dirname "$credentials_file")"
    touch "$credentials_file"
    chmod 600 "$credentials_file"
    
    if [[ -f "$credentials_file" ]]; then
        log_verbose "Existing credentials file content:"
        if [[ -r "$credentials_file" ]]; then
            while IFS= read -r line; do
                log_verbose "  $(echo "$line" | sed 's/:[^:]*@/:***@/')"
            done < "$credentials_file"
        fi
        
        grep -v "https://.*@github.com" "$credentials_file" > "${credentials_file}.tmp" 2>/dev/null || touch "${credentials_file}.tmp"
        mv "${credentials_file}.tmp" "$credentials_file"
    fi
    
    echo "$main_entry" >> "$credentials_file"
    echo "$backup_entry" >> "$credentials_file"
    
    log_verbose "Updated credentials file content:"
    while IFS= read -r line; do
        log_verbose "  $(echo "$line" | sed 's/:[^:]*@/:***@/')"
    done < "$credentials_file"
    
    git config --global --list | grep -i credential | while read -r line; do
        log_verbose "Git credential config: $line"
    done
    
    log_verbose "Git credentials configured for both accounts"
    log_success "HTTPS authentication configured"
}

validate_token_scopes() {
    local token=$1
    local expected_user=$2
    local is_main_user=$3
    
    if [[ "$is_main_user" == "true" && "$MAIN_TOKEN_VALIDATED" == true ]]; then
        return 0
    fi
    
    if [[ "$is_main_user" == "false" && "$BACKUP_TOKEN_VALIDATED" == true ]]; then
        return 0
    fi
    
    echo "└── Validating token scopes for $expected_user..." >&2
    
    if [[ "$DRY_RUN" == true ]]; then
        log_verbose "DRY RUN: Would validate token"
        return 0
    fi
    
    local response
    response=$(make_github_request "GET" "https://api.github.com/user" "$token")
    
    if [[ "$response" == "NOT_FOUND" ]] || [[ -z "$response" ]]; then
        log_error "Token validation failed for $expected_user"
        return 1
    fi
    
    if echo "$response" | grep -q '"login"'; then
        local actual_user
        actual_user=$(echo "$response" | grep -o '"login":"[^"]*"' | cut -d'"' -f4)
        log_verbose "Token validated for user: $actual_user"
        log_verbose "Expected user: $expected_user"
        
        if [[ "$is_main_user" == "true" ]]; then
            MAIN_USER="$actual_user"
            MAIN_TOKEN_VALIDATED=true
            log_verbose "Updated MAIN_USER to: $MAIN_USER"
        else
            BACKUP_USER="$actual_user"
            BACKUP_TOKEN_VALIDATED=true
            log_verbose "Updated BACKUP_USER to: $BACKUP_USER"
        fi
        
        return 0
    else
        log_error "Invalid response from GitHub API"
        return 1
    fi
}

create_github_repo() {
    local user=$1
    local token=""
    
    [[ "$user" == "$MAIN_USER" ]] && token="$MAIN_TOKEN" || token="$BACKUP_TOKEN"
    
    if [[ -z "$token" ]]; then
        echo "└── No token for $user" >&2
        echo "    Create manually: https://github.com/new" >&2
        if [[ "$DRY_RUN" == false ]]; then
            echo -n "    Press Enter after creating $REPO_NAME..." >&2
            read -r < /dev/tty
        fi
        return 0
    fi
    
    echo "└── Checking repository existence..." >&2
    
    if [[ "$DRY_RUN" == true ]]; then
        log_verbose "DRY RUN: Would check/create repository"
        return 0
    fi
    
    local response
    response=$(make_github_request "GET" "https://api.github.com/repos/$user/$REPO_NAME" "$token")
    
    if [[ "$response" != "NOT_FOUND" ]]; then
        echo "└── Repository exists" >&2
        return 0
    fi
    
    echo "└── Creating repository via API..." >&2
    log_verbose "Creating repository for user: $user"
    log_verbose "Repository name: $REPO_NAME"
    log_verbose "API URL: https://api.github.com/user/repos"
    
    local repo_data='{
        "name": "'"$REPO_NAME"'",
        "description": "Synchronized repository created by dual account script",
        "private": false,
        "auto_init": false,
        "has_issues": true,
        "has_projects": true,
        "has_wiki": true
    }'
    
    response=$(make_github_request "POST" "https://api.github.com/user/repos" "$token" "$repo_data")
    
    if [[ -n "$response" && "$response" != "NOT_FOUND" ]]; then
        echo "└── ✓ Repository created successfully" >&2
        return 0
    else
        echo "    Manual creation: https://github.com/new" >&2
        echo -n "    Press Enter after creating $REPO_NAME..." >&2
        read -r < /dev/tty
        return 1
    fi
}

get_github_tokens() {
    local token_file="$HOME/.github_dual_tokens"
    
    [[ -n "${GITHUB_MAIN_TOKEN:-}" ]] && MAIN_TOKEN="$GITHUB_MAIN_TOKEN"
    [[ -n "${GITHUB_BACKUP_TOKEN:-}" ]] && BACKUP_TOKEN="$GITHUB_BACKUP_TOKEN"
    
    if [[ -f "$token_file" ]]; then
        echo "└── Loading saved tokens..." >&2
        if [[ -r "$token_file" ]]; then
            while IFS='=' read -r key value; do
                [[ -z "$key" || "$key" =~ ^[[:space:]]*# ]] && continue
                
                value=$(echo "$value" | sed 's/^["'\''[:space:]]*//;s/["'\''[:space:]]*$//')
                
                case "$key" in
                    "MAIN_TOKEN") [[ -z "$MAIN_TOKEN" ]] && MAIN_TOKEN="$value";;
                    "BACKUP_TOKEN") [[ -z "$BACKUP_TOKEN" ]] && BACKUP_TOKEN="$value";;
                esac
            done < "$token_file"
        else
            log_warning "Token file exists but is not readable"
        fi
    fi
    
    if [[ -n "$MAIN_TOKEN" ]] && ! validate_github_token "$MAIN_TOKEN"; then
        log_error "Invalid main token format. Token must start with 'ghp_' or 'github_pat_'"
        MAIN_TOKEN=""
    fi

    if [[ -n "$BACKUP_TOKEN" ]] && ! validate_github_token "$BACKUP_TOKEN"; then
        log_error "Invalid backup token format. Token must start with 'ghp_' or 'github_pat_'"
        BACKUP_TOKEN=""
    fi

    if [[ -n "$MAIN_TOKEN" ]] && [[ "$DRY_RUN" == false ]]; then
        echo "└── Processing main account..." >&2
        if ! validate_token_scopes "$MAIN_TOKEN" "$MAIN_USER" "true"; then
            log_error "Main token failed API validation"
            MAIN_TOKEN=""
            [[ -f "$token_file" ]] && rm -f "$token_file"
        else
            log_verbose "Main account validated: $MAIN_USER"
        fi
    fi

    if [[ -n "$BACKUP_TOKEN" ]] && [[ "$DRY_RUN" == false ]]; then
        echo "└── Processing backup account..." >&2
        if ! validate_token_scopes "$BACKUP_TOKEN" "$BACKUP_USER" "false"; then
            log_error "Backup token failed API validation"
            BACKUP_TOKEN=""
            [[ -f "$token_file" ]] && rm -f "$token_file"
        else
            log_verbose "Backup account validated: $BACKUP_USER"
        fi
    fi

    if [[ -z "$MAIN_TOKEN" ]] && command -v gh >/dev/null 2>&1; then
        echo "└── Checking GitHub CLI..." >&2
        local gh_token
        gh_token=$(gh auth token 2>/dev/null || echo "")
        if [[ -n "$gh_token" ]] && validate_github_token "$gh_token"; then
            if [[ "$DRY_RUN" == false ]] && validate_token_scopes "$gh_token" "$MAIN_USER" "true"; then
                MAIN_TOKEN="$gh_token"
            elif [[ "$DRY_RUN" == true ]]; then
                MAIN_TOKEN="$gh_token"
            fi
        fi
    fi
    
    local need_save=false
    if [[ -z "$MAIN_TOKEN" ]]; then
        echo "└── GitHub Personal Access Token required" >&2
        echo "    1. Go to: https://github.com/settings/tokens/new" >&2
        echo "    2. Select scopes: ☑️ repo, ☑️ write:packages" >&2
        echo "    3. Generate token and copy it" >&2
        echo "    4. Token must start with 'ghp_' or 'github_pat_'" >&2
        
        if [[ "$DRY_RUN" == false ]]; then
            while true; do
                echo -n "Main token: " >&2
                IFS= read -rs MAIN_TOKEN < /dev/tty
                echo >&2
                
                if validate_github_token "$MAIN_TOKEN"; then
                    if validate_token_scopes "$MAIN_TOKEN" "main-user" "true"; then
                        break
                    else
                        log_error "Token validation failed, please try again"
                        MAIN_TOKEN=""
                    fi
                else
                    log_error "Invalid token format. Must be 'ghp_' or 'github_pat_' format."
                fi
            done
            need_save=true
        else
            log_verbose "DRY RUN: Would prompt for main token"
            MAIN_TOKEN="ghp_dry_run_token_placeholder_1234567890"
        fi
    fi
    
    if [[ -z "$BACKUP_TOKEN" ]]; then
        echo "    Same process for backup account" >&2
        
        if [[ "$DRY_RUN" == false ]]; then
            while true; do
                echo -n "Backup token: " >&2
                IFS= read -rs BACKUP_TOKEN < /dev/tty
                echo >&2
                
                if validate_github_token "$BACKUP_TOKEN"; then
                    if validate_token_scopes "$BACKUP_TOKEN" "backup-user" "false"; then
                        break
                    else
                        log_error "Token validation failed, please try again"
                        BACKUP_TOKEN=""
                    fi
                else
                    log_error "Invalid token format. Must be 'ghp_' or 'github_pat_' format."
                fi
            done
            need_save=true
        else
            log_verbose "DRY RUN: Would prompt for backup token"
            BACKUP_TOKEN="ghp_dry_run_token_placeholder_0987654321"
        fi
    fi
    
    [[ -z "$MAIN_TOKEN" ]] && error_exit "Main token cannot be empty"
    [[ -z "$BACKUP_TOKEN" ]] && error_exit "Backup token cannot be empty"
    
    log_verbose "Final user configuration:"
    log_verbose "  Main user: $MAIN_USER"
    log_verbose "  Backup user: $BACKUP_USER"
    log_verbose "  Repository: $REPO_NAME"
    
    if [[ "$need_save" == true && "$DRY_RUN" == false ]]; then
        mkdir -p "$(dirname "$token_file")"
        
        (umask 077; cat > "$token_file" << EOF
MAIN_TOKEN="$MAIN_TOKEN"
BACKUP_TOKEN="$BACKUP_TOKEN"
EOF
        )
        
        if [[ -f "$token_file" ]]; then
            chmod 600 "$token_file"
            echo "└── ✓ Tokens saved securely" >&2
        else
            log_warning "Failed to save tokens"
        fi
    else
        echo "└── ✓ Tokens loaded" >&2
    fi
}

setup_gitignore() {
    local gitignore_items=(
        "*.log"
        "*.tmp"
        "*.cache"
        "*.swp"
        "*.swo"
        "*~"
        ".DS_Store"
        ".DS_Store?"
        "._*"
        ".Spotlight-V100"
        ".Trashes"
        "ehthumbs.db"
        "Thumbs.db"
        "Desktop.ini"
        "node_modules/"
        "npm-debug.log*"
        "yarn-debug.log*"
        "yarn-error.log*"
        "package-lock.json"
        "yarn.lock"
        "__pycache__/"
        "*.pyc"
        "*.pyo"
        "*.pyd"
        ".Python"
        "env/"
        "venv/"
        ".env"
        ".env.local"
        ".env.development"
        ".env.test"
        ".env.production"
        "config.json"
        "config.local.json"
        "secrets.json"
        "credentials.json"
        ".vscode/"
        ".idea/"
        "*.sublime-project"
        "*.sublime-workspace"
        ".vs/"
        "dist/"
        "build/"
        "out/"
        "*.egg-info/"
        ".coverage"
        ".pytest_cache/"
        ".mypy_cache/"
        ".tox/"
        "target/"
        "*.class"
        "*.jar"
        "*.war"
        "*.ear"
        ".gradle/"
        "*.iml"
        "*.ipr"
        "*.iws"
        ".next/"
        ".nuxt/"
        ".output/"
        ".temp/"
        ".sass-cache/"
        ".parcel-cache/"
    )
    
    local missing_items=()
    local needs_update=false
    
    if [[ -f ".gitignore" ]]; then
        for item in "${gitignore_items[@]}"; do
            if ! grep -Fxq "$item" .gitignore 2>/dev/null; then
                missing_items+=("$item")
                needs_update=true
            fi
        done
        
        if [[ "$needs_update" == true && "$DRY_RUN" == false ]]; then
            {
                echo ""
                echo "# Auto-added by sync script"
                printf '%s\n' "${missing_items[@]}"
            } >> .gitignore
            log_verbose "Added ${#missing_items[@]} items to .gitignore"
        fi
    else
        if [[ "$DRY_RUN" == false ]]; then
            {
                echo "# Generated by GitHub Dual Account Sync script"
                echo "# $(date)"
                echo ""
                printf '%s\n' "${gitignore_items[@]}"
            } > .gitignore
        fi
        needs_update=true
        log_verbose "Created new .gitignore with ${#gitignore_items[@]} items"
    fi
    
    [[ "$needs_update" == true ]]
}

setup_remotes() {
    local main_url="https://github.com/$MAIN_USER/$REPO_NAME.git"
    local backup_url="https://github.com/$BACKUP_USER/$REPO_NAME.git"
    
    log_verbose "Setting up remotes with URLs:"
    log_verbose "  Main user: $MAIN_USER"
    log_verbose "  Backup user: $BACKUP_USER"
    log_verbose "  Repository: $REPO_NAME"
    log_verbose "  Main URL: $main_url"
    log_verbose "  Backup URL: $backup_url"
    
    set_remote() {
        local remote_name=$1
        local remote_url=$2
        local current_url=""
        
        remote_url="${remote_url%/}"
        
        if git remote get-url "$remote_name" >/dev/null 2>&1; then
            current_url=$(git remote get-url "$remote_name" 2>/dev/null || echo "")
            current_url="${current_url%/}"
            log_verbose "Current $remote_name URL: $current_url"
            
            if [[ "$current_url" != "$remote_url" ]]; then
                log_verbose "Updating remote $remote_name from $current_url to $remote_url"
                
                if [[ "$DRY_RUN" == false ]]; then
                    git remote set-url "$remote_name" "$remote_url"
                fi
            else
                log_verbose "Remote $remote_name already configured correctly"
            fi
        else
            log_verbose "Adding remote $remote_name"
            if [[ "$DRY_RUN" == false ]]; then
                git remote add "$remote_name" "$remote_url"
            fi
        fi
        
        if [[ "$DRY_RUN" == false ]]; then
            local final_url
            final_url=$(git remote get-url "$remote_name" 2>/dev/null || echo "")
            log_verbose "Final $remote_name URL: $final_url"
            
            if [[ -z "$final_url" ]]; then
                log_error "Failed to configure remote $remote_name"
                return 1
            fi
        fi
    }
    
    if ! set_remote "$MAIN_REMOTE" "$main_url"; then
        return 1
    fi
    
    if ! set_remote "$BACKUP_REMOTE" "$backup_url"; then
        return 1
    fi
    
    if [[ "$DRY_RUN" == false ]]; then
        git config push.default current
        git config credential.helper store
    fi
    
    log_verbose "Remotes configured:"
    log_verbose "  $MAIN_REMOTE -> $main_url"
    log_verbose "  $BACKUP_REMOTE -> $backup_url"
    
    return 0
}

sync_submodules() {
    if [[ -f .gitmodules ]]; then
        log_info "Submodules detected, synchronizing..."
        
        if [[ "$DRY_RUN" == false ]]; then
            git submodule sync --recursive
            git submodule update --init --recursive
            
            git submodule foreach --recursive '
                git fetch --all
                git checkout $(git rev-parse HEAD)
            '
        else
            log_verbose "DRY RUN: Would sync submodules"
        fi
        
        log_success "Submodules synchronized"
    fi
}

handle_push_conflict() {
    local remote=$1
    local user=$2
    
    if [[ "$DRY_RUN" == true ]]; then
        log_verbose "DRY RUN: Would handle push conflict"
        return 0
    fi
    
    echo "├── Repository diverged. Options:" >&2
    echo "    1) Pull and merge (recommended)" >&2
    echo "    2) Force push (overwrite remote)" >&2
    echo "    3) Skip this repository" >&2
    echo "    4) Exit script" >&2
    
    while true; do
        echo -n "├── Choice [1-4]: " >&2
        read -r choice < /dev/tty
        
        case $choice in
            1)
                echo "├── Attempting merge..." >&2
                
                if ! git fetch "$remote" 2>&1; then
                    log_error "Failed to fetch from $remote"
                    return 1
                fi
                
                if ! git rev-parse --verify "$remote/main" >/dev/null 2>&1; then
                    log_error "Remote branch $remote/main not found"
                    return 1
                fi
                
                local merge_output
                merge_output=$(git merge "$remote/main" --no-edit 2>&1)
                local merge_status=$?
                
                if [[ $merge_status -eq 0 ]]; then
                    echo "├── Merge successful, pushing..." >&2
                    if git push "$remote" main 2>&1; then
                        echo "├── ✓ Push successful after merge" >&2
                        return 0
                    else
                        log_error "Push failed after merge"
                        return 1
                    fi
                else
                    if echo "$merge_output" | grep -q "CONFLICT"; then
                        echo "├── Merge conflicts detected" >&2
                        echo "    Files with conflicts:" >&2
                        git diff --name-only --diff-filter=U | sed 's/^/      - /' >&2
                        echo "" >&2
                        echo "    To resolve:" >&2
                        echo "      1. Edit conflicted files" >&2
                        echo "      2. git add <resolved-files>" >&2
                        echo "      3. git commit" >&2
                        echo "      4. git push $remote main" >&2
                        
                        git merge --abort 2>/dev/null || true
                        return 1
                    else
                        log_error "Merge failed: $merge_output"
                        git merge --abort 2>/dev/null || true
                        return 1
                    fi
                fi
                ;;
                
            2)
                echo "├── Force pushing..." >&2
                log_warning "This will overwrite the remote repository!"
                
                if git push --force "$remote" main 2>&1; then
                    echo "├── ⚠️  Force push successful" >&2
                    return 0
                else
                    log_error "Force push failed"
                    return 1
                fi
                ;;
                
            3)
                echo "├── ⏭️  Skipping $user" >&2
                return 1
                ;;
                
            4)
                echo "├── 🛑 Exiting..." >&2
                exit 1
                ;;
                
            *)
                echo "    Invalid choice. Please enter 1-4." >&2
                ;;
        esac
    done
}

handle_large_file_push_error() {
    local remote=$1
    local user=$2
    local error_output=$3
    
    log_error "Large file detected during push"
    echo "$error_output" | grep -E "(file|exceeds)" | sed 's/^/    /' >&2
    
    if [[ "$LFS_CONFIGURED" == true ]]; then
        log_info "Git LFS was configured, but some files may not be tracked"
        echo "├── Attempting to migrate existing large files to LFS..." >&2
        
        if [[ "$DRY_RUN" == false ]]; then
            local large_files
            if large_files=$(detect_large_files); then
                echo "$large_files" | while read -r file; do
                    git lfs migrate import --include="$file" --everything 2>/dev/null || true
                done
                
                echo "├── Retrying push..." >&2
                if git push "$remote" main 2>&1; then
                    echo "├── ✓ Push successful after LFS migration" >&2
                    return 0
                fi
            fi
        fi
    fi
    
    if [[ "$DRY_RUN" == true ]]; then
        log_verbose "DRY RUN: Would handle large file error"
        return 0
    fi
    
    echo "├── Options:" >&2
    echo "    1) Configure Git LFS and retry" >&2
    echo "    2) Remove large files and retry" >&2
    echo "    3) Skip this repository" >&2
    echo -n "├── Choice [1-3]: " >&2
    read -r choice < /dev/tty
    
    case $choice in
        1)
            if setup_git_lfs && handle_large_files; then
                echo "├── Git LFS configured, retrying push..." >&2
                if git push "$remote" main 2>&1; then
                    echo "├── ✓ Push successful with LFS" >&2
                    return 0
                else
                    log_error "Push still failed after LFS setup"
                    return 1
                fi
            else
                log_error "Failed to configure Git LFS"
                return 1
            fi
            ;;
        2)
            echo "├── Use 'git rm --cached <large-file>' to remove" >&2
            echo "├── Then run this script again" >&2
            return 1
            ;;
        3)
            echo "├── ⏭️  Skipping due to large files" >&2
            return 1
            ;;
        *)
            echo "├── Invalid choice, skipping" >&2
            return 1
            ;;
    esac
}

push_to_repository() {
    local remote=$1
    local user=$2
    
    echo "├── Pushing to $user..." >&2
    
    if ! git remote get-url "$remote" >/dev/null 2>&1; then
        log_error "Remote '$remote' not configured"
        return 1
    fi
    
    if ! git rev-parse HEAD >/dev/null 2>&1; then
        log_error "No commits to push"
        return 1
    fi
    
    local remote_url
    remote_url=$(git remote get-url "$remote" 2>/dev/null || echo "")
    remote_url="${remote_url%/}"
    log_verbose "Remote URL for $remote: $remote_url"
    
    if [[ "$LFS_CONFIGURED" == true ]]; then
        log_verbose "LFS is configured, setting up authentication"
        local token=""
        [[ "$user" == "$MAIN_USER" ]] && token="$MAIN_TOKEN" || token="$BACKUP_TOKEN"
        
        if [[ -n "$token" ]]; then
            log_verbose "Configuring LFS authentication for $user"
            git config --local lfs.url "https://github.com/$user/$REPO_NAME.git/info/lfs"
            git config --local lfs.batch true
            git config --local lfs.transfer.maxretries 3
        fi
    fi
    
    if [[ "$DRY_RUN" == true ]]; then
        log_verbose "DRY RUN: Would push to $user"
        echo "├── ✓ Push would succeed (dry run)" >&2
        return 0
    fi
    
    log_verbose "Testing repository access..."
    local repo_accessible=true
    if ! git ls-remote "$remote" >/dev/null 2>&1; then
        repo_accessible=false
    fi
    
    if [[ "$repo_accessible" == false ]]; then
        log_verbose "Repository access test failed, attempting to create repository"
        if create_github_repo "$user"; then
            log_verbose "Repository created successfully"
        else
            log_error "Failed to create repository for $user"
            return 1
        fi
    fi
    
    local push_output
    local push_status
    local temp_log="/tmp/git_push_log_$$"
    
    if [[ "$LFS_CONFIGURED" == true ]]; then
        log_verbose "Pushing LFS files first"
        if ! run_with_timeout 300 git lfs push "$remote" main 2>&1; then
            log_warning "LFS push failed or timed out, continuing with regular push"
        fi
    fi
    
    log_verbose "Executing: git push $remote main"
    push_output=$(run_with_timeout 300 git push "$remote" main 2>&1 | tee "$temp_log")
    push_status=$?
    
    if [[ $push_status -eq 124 ]]; then
        log_error "Push operation timed out"
        rm -f "$temp_log"
        return 1
    fi
    
    log_verbose "Git push output (status: $push_status):"
    if [[ -f "$temp_log" ]]; then
        while IFS= read -r line; do
            log_verbose "  $line"
        done < "$temp_log"
        rm -f "$temp_log"
    fi
    
    if [[ $push_status -eq 0 ]]; then
        if echo "$push_output" | grep -q "Everything up-to-date"; then
            echo "├── ✓ Already up-to-date" >&2
        else
            echo "├── ✓ Push successful" >&2
        fi
        return 0
    else
        if echo "$push_output" | grep -qE "(rejected.*fetch first|non-fast-forward|Updates were rejected)"; then
            handle_push_conflict "$remote" "$user"
            return $?
        elif echo "$push_output" | grep -qE "(Repository not found|does not exist|404)"; then
            echo "├── Repository not found, creating..." >&2
            if create_github_repo "$user"; then
                sleep 5
                log_verbose "Retrying push after repository creation"
                if run_with_timeout 300 git push -u "$remote" main 2>&1; then
                    echo "├── ✓ Push successful after creation" >&2
                    return 0
                else
                    log_error "Push failed after creation"
                    return 1
                fi
            else
                return 1
            fi
        elif echo "$push_output" | grep -qE "(Authentication failed|authentication required|403|401|Bad credentials)"; then
            log_error "Authentication failed for $user"
            echo "    Check token validity and scopes" >&2
            return 1
        elif echo "$push_output" | grep -qE "file.*larger|exceeds.*file size limit"; then
            handle_large_file_push_error "$remote" "$user" "$push_output"
            return $?
        elif echo "$push_output" | grep -qE "(rate limit|API rate limit)"; then
            log_warning "Rate limit exceeded, waiting..."
            sleep 60
            echo "├── Retrying push..." >&2
            push_to_repository "$remote" "$user"
            return $?
        elif echo "$push_output" | grep -qE "(network|timeout|connection|Could not resolve)"; then
            log_warning "Network error, retrying..."
            sleep 5
            echo "├── Retrying push..." >&2
            push_to_repository "$remote" "$user"
            return $?
        else
            log_error "Push failed with unknown error"
            echo "$push_output" | sed 's/^/    /' >&2
            echo "├── Options:" >&2
            echo "    1) Retry" >&2
            echo "    2) Skip this repository" >&2
            echo -n "├── Choice [1-2]: " >&2
            read -r choice < /dev/tty
            case $choice in
                1)
                    push_to_repository "$remote" "$user"
                    return $?
                    ;;
                2)
                    echo "├── ⏭️  Skipping $user" >&2
                    return 1
                    ;;
                *)
                    echo "├── Invalid choice, skipping" >&2
                    return 1
                    ;;
            esac
        fi
    fi
}

push_to_github() {
    echo "Synchronizing repositories sequentially..." >&2
    
    local main_success=false
    local backup_success=false
    
    echo "├── Phase 1: Processing main account ($MAIN_USER)..." >&2
    log_verbose "Main account details:"
    log_verbose "  User: $MAIN_USER"
    log_verbose "  Remote: $MAIN_REMOTE"
    log_verbose "  Token validated: $MAIN_TOKEN_VALIDATED"
    
    if push_to_repository "$MAIN_REMOTE" "$MAIN_USER"; then
        main_success=true
        echo "├── ✓ Main account synchronized successfully" >&2
    else
        echo "├── ✗ Main account synchronization failed" >&2
    fi
    
    echo "├── Phase 2: Processing backup account ($BACKUP_USER)..." >&2
    log_verbose "Backup account details:"
    log_verbose "  User: $BACKUP_USER"
    log_verbose "  Remote: $BACKUP_REMOTE"
    log_verbose "  Token validated: $BACKUP_TOKEN_VALIDATED"
    
    if push_to_repository "$BACKUP_REMOTE" "$BACKUP_USER"; then
        backup_success=true
        echo "├── ✓ Backup account synchronized successfully" >&2
    else
        echo "├── ✗ Backup account synchronization failed" >&2
    fi
    
    echo "└── Summary:" >&2
    if [[ "$main_success" == true ]] && [[ "$backup_success" == true ]]; then
        echo "    🎉 Both repositories synchronized successfully" >&2
        return 0
    elif [[ "$main_success" == true ]] || [[ "$backup_success" == true ]]; then
        echo "    ⚠️  Partial synchronization completed" >&2
        [[ "$main_success" == true ]] && echo "    ✓ Main repository updated" >&2
        [[ "$backup_success" == true ]] && echo "    ✓ Backup repository updated" >&2
        [[ "$main_success" == false ]] && echo "    ✗ Main repository failed" >&2
        [[ "$backup_success" == false ]] && echo "    ✗ Backup repository failed" >&2
        return 1
    else
        echo "    ✗ Synchronization failed for both repositories" >&2
        return 1
    fi
}

main() {
    if [[ ! -d "." ]] || [[ ! -w "." ]]; then
        error_exit "Current directory is not accessible"
    fi
    
    echo "├── Checking and installing dependencies..." >&2
    check_and_install_dependencies
    
    local git_version
    git_version=$(git --version | grep -oE '[0-9]+\.[0-9]+' | head -1)
    local major_version=${git_version%%.*}
    if [[ $major_version -lt 2 ]]; then
        log_warning "Git version $git_version is old. Some features may not work properly."
        log_warning "Consider upgrading to Git 2.0 or newer."
    fi
    
    if [[ "$DRY_RUN" == false ]]; then
        git config --global init.defaultBranch main 2>/dev/null || true
        git config --global advice.defaultBranchName false 2>/dev/null || true
        git config --global advice.detachedHead false 2>/dev/null || true
        git config --global advice.addIgnoredFile false 2>/dev/null || true
        
        if ! git config --global user.name >/dev/null 2>&1; then
            git config --global user.name "GitHub Sync Script"
        fi
        if ! git config --global user.email >/dev/null 2>&1; then
            git config --global user.email "sync@github.local"
        fi
    fi
    
    if [[ "$DRY_RUN" == true ]]; then
        echo "🔍 DRY RUN MODE - No changes will be made" >&2
    fi
    
    echo "🚀 GitHub Dual Account Sync: $REPO_NAME" >&2
    
    check_repository_size
    
    echo "├── Retrieving GitHub tokens..." >&2
    get_github_tokens
    
    echo "├── Configuring HTTPS authentication..." >&2
    setup_git_credentials
    
    if [[ -d ".git" ]]; then
        echo "├── Repository found, checking branch..." >&2
        
        local current_branch
        current_branch=$(git branch --show-current 2>/dev/null || git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "")
        
        if [[ -z "$current_branch" ]]; then
            log_warning "Repository is in detached HEAD state"
            echo "├── Creating main branch..." >&2
            if [[ "$DRY_RUN" == false ]]; then
                git checkout -b main
            fi
        elif [[ "$current_branch" != "main" ]]; then
            echo "├── Current branch: $current_branch" >&2
            echo "├── Switching to main branch..." >&2
            
            if [[ "$DRY_RUN" == false ]]; then
                if git show-ref --verify --quiet refs/heads/main 2>/dev/null; then
                    git checkout main
                else
                    git checkout -b main
                fi
            fi
        fi
        
        echo "├── Checking for large files..." >&2
        handle_large_files
        
        echo "├── Synchronizing submodules..." >&2
        sync_submodules
        
        echo "├── Configuring remotes..." >&2
        setup_remotes
        
        echo "├── Updating .gitignore..." >&2
        local gitignore_updated=false
        setup_gitignore && gitignore_updated=true
        
        if [[ "$DRY_RUN" == false ]] && [[ -n "$(git status --porcelain 2>/dev/null)" || -n "$(git submodule status | grep '^+\|^-')" ]]; then
            echo "├── Committing changes..." >&2
            
            git add -A >/dev/null 2>&1
            git submodule foreach --recursive 'git add -A' >/dev/null 2>&1 || true
            
            local commit_msg
            if [[ -n "$COMMIT_MESSAGE" ]]; then
                commit_msg="$COMMIT_MESSAGE"
            elif [[ "$gitignore_updated" == true ]]; then
                commit_msg="Update .gitignore and files $(date '+%Y-%m-%d %H:%M:%S')"
            else
                commit_msg="Update $(date '+%Y-%m-%d %H:%M:%S')"
            fi
            
            if git commit -m "$commit_msg" 2>/dev/null; then
                echo "├── ✓ Changes committed" >&2
            else
                log_warning "Nothing to commit (working tree clean)"
            fi
        else
            echo "├── No changes to commit" >&2
        fi
        
        push_to_github
    else
        echo "├── Initializing repository..." >&2
        
        if [[ "$DRY_RUN" == false ]]; then
            git init --initial-branch=main --quiet
        fi
        
        echo "├── Checking for large files..." >&2
        handle_large_files
        
        echo "├── Creating initial content..." >&2
        
        if [[ $(find . -maxdepth 1 -type f ! -name ".*" | wc -l) -eq 0 ]]; then
            if [[ "$DRY_RUN" == false ]]; then
                cat > README.md << EOF
# $REPO_NAME

This repository is synchronized to multiple GitHub accounts.

## Accounts
- Main: https://github.com/$MAIN_USER/$REPO_NAME
- Backup: https://github.com/$BACKUP_USER/$REPO_NAME

Created on $(date)
EOF
            fi
        fi
        
        echo "├── Setting up .gitignore..." >&2
        setup_gitignore
        
        echo "├── Initial commit..." >&2
        if [[ "$DRY_RUN" == false ]]; then
            git add -A
            
            local init_msg
            if [[ -n "$COMMIT_MESSAGE" ]]; then
                init_msg="$COMMIT_MESSAGE"
            else
                init_msg="Initial commit"
            fi
            
            if git commit -m "$init_msg" 2>/dev/null; then
                echo "├── ✓ Initial commit created" >&2
            else
                error_exit "Failed to create initial commit"
            fi
        fi
        
        echo "├── Configuring remotes..." >&2
        setup_remotes
        
        push_to_github
    fi
    
    echo "" >&2
    echo "📊 Repository Links:" >&2
    echo "├── Main: https://github.com/$MAIN_USER/$REPO_NAME" >&2
    echo "└── Backup: https://github.com/$BACKUP_USER/$REPO_NAME" >&2
    
    return 0
}

if ! main "$@"; then
    exit 1
fi