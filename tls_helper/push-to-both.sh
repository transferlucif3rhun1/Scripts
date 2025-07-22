#!/bin/bash

set -e

REPO_NAME=$(basename "$(pwd)")
MAIN_USER="lucif3rhun1"
BACKUP_USER="transferlucif3rhun1"
MAIN_REMOTE="origin"
BACKUP_REMOTE="backup"
MAIN_TOKEN=""
BACKUP_TOKEN=""

cleanup_ssh_setup() {
    rm -f ~/.ssh/id_rsa_main ~/.ssh/id_rsa_main.pub ~/.ssh/config.bak 2>/dev/null || true
    rm -f ~/.ssh/id_rsa_backup ~/.ssh/id_rsa_backup.pub 2>/dev/null || true
    
    if [ -f ~/.ssh/config ]; then
        sed -i.bak '/Host github-main/,/^$/d; /Host github-backup/,/^$/d' ~/.ssh/config 2>/dev/null || true
        rm -f ~/.ssh/config.bak 2>/dev/null || true
    fi
    
    ssh-add -d ~/.ssh/id_rsa_main ~/.ssh/id_rsa_backup 2>/dev/null || true
}

generate_ssh_keys() {
    mkdir -p ~/.ssh && chmod 700 ~/.ssh
    
    ssh-keygen -t rsa -b 4096 -f ~/.ssh/id_rsa_main -N "" -C "$MAIN_USER@github.com" -q
    ssh-keygen -t rsa -b 4096 -f ~/.ssh/id_rsa_backup -N "" -C "$BACKUP_USER@github.com" -q
    
    chmod 600 ~/.ssh/id_rsa_main ~/.ssh/id_rsa_backup
    chmod 644 ~/.ssh/id_rsa_main.pub ~/.ssh/id_rsa_backup.pub
}

setup_ssh_config() {
    mkdir -p ~/.ssh && chmod 700 ~/.ssh
    
    if [ -f ~/.ssh/config ] && grep -q "Host github-main" ~/.ssh/config && grep -q "Host github-backup" ~/.ssh/config; then
        return 0
    fi
    
    [ -f ~/.ssh/config ] && sed -i.bak '/Host github-main/,/^$/d; /Host github-backup/,/^$/d' ~/.ssh/config 2>/dev/null || true
    rm -f ~/.ssh/config.bak 2>/dev/null || true
    
    cat >> ~/.ssh/config << 'EOF'

Host github-main
    HostName github.com
    User git
    IdentityFile ~/.ssh/id_rsa_main
    IdentitiesOnly yes
    AddKeysToAgent yes

Host github-backup
    HostName github.com
    User git
    IdentityFile ~/.ssh/id_rsa_backup
    IdentitiesOnly yes
    AddKeysToAgent yes

EOF
    
    chmod 600 ~/.ssh/config
    
    if [ ! -f ~/.ssh/known_hosts ] || ! grep -q "github.com" ~/.ssh/known_hosts; then
        ssh-keyscan github.com >> ~/.ssh/known_hosts 2>/dev/null
        chmod 644 ~/.ssh/known_hosts
    fi
}

setup_ssh_agent() {
    if [ -z "$SSH_AUTH_SOCK" ] || ! ssh-add -l >/dev/null 2>&1; then
        eval "$(ssh-agent -s)" >/dev/null 2>&1
    fi
    ssh-add ~/.ssh/id_rsa_main ~/.ssh/id_rsa_backup 2>/dev/null || true
}

test_ssh_keys() {
    setup_ssh_agent
    
    local main_result=$(ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 -T git@github-main 2>&1 || true)
    local backup_result=$(ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 -T git@github-backup 2>&1 || true)
    
    echo "$main_result" | grep -q "successfully authenticated" && echo "$backup_result" | grep -q "successfully authenticated"
}

display_keys_and_get_confirmation() {
    echo "Add SSH keys to GitHub accounts:"
    echo "├── Main ($MAIN_USER): https://github.com/settings/ssh/new"
    cat ~/.ssh/id_rsa_main.pub
    echo "├── Backup ($BACKUP_USER): https://github.com/settings/ssh/new"
    cat ~/.ssh/id_rsa_backup.pub
    
    while true; do
        read -p "└── Keys added? (y/n): " confirm
        case $confirm in
            [Yy]*) break;;
            [Nn]*) exit 1;;
            *) echo "    Answer y or n";;
        esac
    done
}

ensure_ssh_setup() {
    if [ -f ~/.ssh/id_rsa_main ] && [ -f ~/.ssh/id_rsa_backup ]; then
        echo "└── Testing SSH authentication..."
        setup_ssh_config
        setup_ssh_agent
        if test_ssh_keys; then
            echo "└── ✓ SSH authentication verified"
            return 0
        fi
        echo "└── SSH authentication failed, regenerating..."
    else
        echo "└── Generating SSH keys..."
    fi
    
    cleanup_ssh_setup
    generate_ssh_keys
    setup_ssh_config
    setup_ssh_agent
    display_keys_and_get_confirmation
    if test_ssh_keys; then
        echo "└── ✓ SSH setup completed"
    else
        echo "└── ✗ SSH setup failed"
        exit 1
    fi
}

get_github_tokens() {
    local token_file="$HOME/.github_dual_tokens"
    
    [ -n "$GITHUB_MAIN_TOKEN" ] && MAIN_TOKEN="$GITHUB_MAIN_TOKEN"
    [ -n "$GITHUB_BACKUP_TOKEN" ] && BACKUP_TOKEN="$GITHUB_BACKUP_TOKEN"
    
    if [ -f "$token_file" ]; then
        echo "└── Loading saved tokens..."
        while IFS='=' read -r key value; do
            case "$key" in
                "MAIN_TOKEN") [ -z "$MAIN_TOKEN" ] && MAIN_TOKEN=$(echo "$value" | tr -d '"');;
                "BACKUP_TOKEN") [ -z "$BACKUP_TOKEN" ] && BACKUP_TOKEN=$(echo "$value" | tr -d '"');;
            esac
        done < "$token_file"
    fi
    
    if [ -z "$MAIN_TOKEN" ] && command -v gh >/dev/null 2>&1; then
        echo "└── Checking GitHub CLI..."
        local gh_token=$(gh auth token 2>/dev/null || echo "")
        [ -n "$gh_token" ] && MAIN_TOKEN="$gh_token"
    fi
    
    local need_save=false
    
    if [ -z "$MAIN_TOKEN" ]; then
        echo "└── GitHub token required: https://github.com/settings/tokens (scope: repo)"
        echo "Main token ($MAIN_USER):"
        read -s MAIN_TOKEN
        echo
        need_save=true
    fi
    
    if [ -z "$BACKUP_TOKEN" ]; then
        echo "Backup token ($BACKUP_USER):"
        read -s BACKUP_TOKEN
        echo
        need_save=true
    fi
    
    if [ "$need_save" = true ] && [ -n "$MAIN_TOKEN" ] && [ -n "$BACKUP_TOKEN" ]; then
        cat > "$token_file" << EOF
MAIN_TOKEN="$MAIN_TOKEN"
BACKUP_TOKEN="$BACKUP_TOKEN"
EOF
        chmod 600 "$token_file"
        echo "└── ✓ Tokens saved securely"
    else
        echo "└── ✓ Tokens loaded"
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
    
    if [ -f ".gitignore" ]; then
        for item in "${gitignore_items[@]}"; do
            if ! grep -Fxq "$item" .gitignore; then
                missing_items+=("$item")
                needs_update=true
            fi
        done
        
        if [ "$needs_update" = true ]; then
            printf '\n%s\n' "${missing_items[@]}" >> .gitignore
        fi
    else
        printf '%s\n' "${gitignore_items[@]}" > .gitignore
        needs_update=true
    fi
    
    [ "$needs_update" = true ] && return 0 || return 1
}

create_github_repo() {
    local user=$1
    local token=""
    
    [ "$user" = "$MAIN_USER" ] && token="$MAIN_TOKEN" || token="$BACKUP_TOKEN"
    
    if [ -z "$token" ]; then
        echo "└── No token for $user"
        echo "    Create manually: https://github.com/new"
        read -p "    Press Enter after creating $REPO_NAME..."
        return 0
    fi
    
    echo "└── Checking repository existence..."
    local repo_check=$(curl -s -H "Authorization: token $token" "https://api.github.com/repos/$user/$REPO_NAME" 2>/dev/null || echo "")
    if echo "$repo_check" | grep -q '"name"'; then
        echo "└── Repository exists"
        return 0
    fi
    
    echo "└── Creating repository via API..."
    local response=$(curl -s -X POST -H "Authorization: token $token" -H "Accept: application/vnd.github.v3+json" \
        "https://api.github.com/user/repos" -d "{\"name\":\"$REPO_NAME\",\"private\":false,\"auto_init\":false}" 2>/dev/null)
    
    if echo "$response" | grep -q '"clone_url"\|"already exists"'; then
        echo "└── ✓ Repository created"
        return 0
    else
        echo "└── ✗ API creation failed"
        echo "    Create manually: https://github.com/new"
        read -p "    Press Enter after creating..."
        return 1
    fi
}

setup_remotes() {
    local main_url="git@github-main:$MAIN_USER/$REPO_NAME.git"
    local backup_url="git@github-backup:$BACKUP_USER/$REPO_NAME.git"
    
    git remote get-url $MAIN_REMOTE >/dev/null 2>&1 && git remote set-url $MAIN_REMOTE $main_url || git remote add $MAIN_REMOTE $main_url
    git remote get-url $BACKUP_REMOTE >/dev/null 2>&1 && git remote set-url $BACKUP_REMOTE $backup_url || git remote add $BACKUP_REMOTE $backup_url
}

handle_push_conflict() {
    local remote=$1
    local user=$2
    
    echo "├── Repository diverged. Options:"
    echo "    1) Pull and merge (recommended)"
    echo "    2) Force push (overwrite remote)"
    echo "    3) Skip this repository"
    echo "    4) Exit script"
    
    while true; do
        read -p "├── Choice [1-4]: " choice
        case $choice in
            1)
                echo "├── Attempting merge..."
                if git fetch $remote 2>&1; then
                    if git merge $remote/main --no-edit 2>&1; then
                        echo "├── Merge successful, pushing..."
                        if git push $remote main 2>&1; then
                            echo "├── ✓ Push successful after merge"
                            return 0
                        else
                            echo "├── ✗ Push failed after merge"
                            return 1
                        fi
                    else
                        echo "├── Merge conflicts detected"
                        echo "    Resolve conflicts manually and run: git commit && git push $remote main"
                        return 1
                    fi
                else
                    echo "├── ✗ Fetch failed"
                    return 1
                fi
                ;;
            2)
                echo "├── Force pushing..."
                if git push --force $remote main 2>&1; then
                    echo "├── ⚠️  Force push successful"
                    return 0
                else
                    echo "├── ✗ Force push failed"
                    return 1
                fi
                ;;
            3)
                echo "├── ⏭️  Skipping $user"
                return 1
                ;;
            4)
                echo "├── 🛑 Exiting..."
                exit 1
                ;;
            *)
                echo "    Invalid choice. Enter 1-4."
                ;;
        esac
    done
}

push_to_repository() {
    local remote=$1
    local user=$2
    
    echo "├── Pushing to $user..."
    
    local push_output=$(git push $remote main 2>&1 || git push -u $remote main 2>&1 || echo "FAILED")
    
    if echo "$push_output" | grep -q "FAILED"; then
        if echo "$push_output" | grep -q "rejected.*fetch first\|non-fast-forward"; then
            return $(handle_push_conflict "$remote" "$user"; echo $?)
        elif echo "$push_output" | grep -q "does not exist\|not found\|403"; then
            echo "├── Repository not found, creating..."
            create_github_repo "$user"
            sleep 2
            if git push -u $remote main 2>&1; then
                echo "├── ✓ Push successful after creation"
                return 0
            else
                echo "├── ✗ Push failed after creation"
                return 1
            fi
        elif echo "$push_output" | grep -q "Permission denied\|authentication failed"; then
            echo "├── ✗ Authentication failed for $user"
            echo "    Check SSH keys and tokens"
            return 1
        elif echo "$push_output" | grep -q "file is.*larger"; then
            echo "├── Large file detected. Options:"
            echo "    1) Remove large files and retry"
            echo "    2) Skip this repository"
            read -p "├── Choice [1-2]: " choice
            case $choice in
                1)
                    echo "├── Remove large files manually and re-run script"
                    return 1
                    ;;
                2)
                    echo "├── ⏭️  Skipping due to large files"
                    return 1
                    ;;
            esac
        else
            echo "├── ✗ Unknown push error:"
            echo "$push_output" | sed 's/^/    /'
            echo "├── Options:"
            echo "    1) Retry"
            echo "    2) Skip this repository"
            read -p "├── Choice [1-2]: " choice
            case $choice in
                1)
                    return $(push_to_repository "$remote" "$user"; echo $?)
                    ;;
                2)
                    echo "├── ⏭️  Skipping $user"
                    return 1
                    ;;
            esac
        fi
    else
        echo "$push_output"
        echo "├── ✓ Push successful"
        return 0
    fi
}

push_to_github() {
    echo "Synchronizing repositories..."
    
    local main_success=false
    local backup_success=false
    
    if push_to_repository "$MAIN_REMOTE" "$MAIN_USER"; then
        main_success=true
    fi
    
    if push_to_repository "$BACKUP_REMOTE" "$BACKUP_USER"; then
        backup_success=true
    fi
    
    if [ "$main_success" = true ] && [ "$backup_success" = true ]; then
        echo "└── 🎉 Both repositories synchronized"
    elif [ "$main_success" = true ] || [ "$backup_success" = true ]; then
        echo "└── ⚠️  Partial synchronization completed"
    else
        echo "└── ✗ Synchronization failed"
    fi
}

main() {
    echo "🚀 GitHub Dual Account Sync: $REPO_NAME"
    
    echo "├── Configuring SSH authentication..."
    ensure_ssh_setup
    
    echo "├── Retrieving GitHub tokens..."
    get_github_tokens
    
    if [ -d ".git" ]; then
        echo "├── Repository found, checking branch..."
        current_branch=$(git branch --show-current 2>/dev/null || echo "")
        if [ "$current_branch" != "main" ]; then
            echo "├── Switching to main branch..."
            git show-ref --verify --quiet refs/heads/main 2>/dev/null && git checkout main || git checkout -b main
        fi
        
        echo "├── Configuring remotes..."
        setup_remotes
        
        echo "├── Updating .gitignore..."
        gitignore_updated=false
        setup_gitignore && gitignore_updated=true
        
        if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
            echo "├── Committing changes..."
            git add .
            if [ "$gitignore_updated" = true ]; then
                git commit -m "Update .gitignore and files $(date '+%Y-%m-%d %H:%M:%S')" 2>/dev/null
            else
                git commit -m "Update $(date '+%Y-%m-%d %H:%M:%S')" 2>/dev/null
            fi
        else
            echo "├── No changes to commit"
        fi
        
        push_to_github
        
    else
        echo "├── Initializing repository..."
        git init && git branch -M main
        
        echo "├── Creating initial content..."
        [ $(find . -maxdepth 1 -type f ! -name ".*" | wc -l) -eq 0 ] && echo "# $REPO_NAME" > README.md
        
        echo "├── Setting up .gitignore..."
        setup_gitignore
        
        echo "├── Initial commit..."
        git add . && git commit -m "Initial commit" 2>/dev/null
        
        echo "├── Configuring remotes..."
        setup_remotes
        
        push_to_github
    fi
    
    echo ""
    echo "📊 Repository Links:"
    echo "├── Main: https://github.com/$MAIN_USER/$REPO_NAME"
    echo "└── Backup: https://github.com/$BACKUP_USER/$REPO_NAME"
}

main