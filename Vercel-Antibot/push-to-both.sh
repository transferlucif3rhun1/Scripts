#!/bin/bash

set -e

REPO_NAME=$(basename "$(pwd)")
MAIN_USER="lucif3rhun1"
BACKUP_USER="transferlucif3rhun1"
MAIN_REMOTE="origin"
BACKUP_REMOTE="backup"

cleanup_ssh_setup() {
    echo "Cleaning up SSH setup..."
    rm -f ~/.ssh/id_rsa_main ~/.ssh/id_rsa_main.pub 2>/dev/null || true
    rm -f ~/.ssh/id_rsa_backup ~/.ssh/id_rsa_backup.pub 2>/dev/null || true
    
    if [ -f ~/.ssh/config ]; then
        sed -i.bak '/Host github-main/,/^$/d' ~/.ssh/config 2>/dev/null || true
        sed -i.bak '/Host github-backup/,/^$/d' ~/.ssh/config 2>/dev/null || true
        rm -f ~/.ssh/config.bak 2>/dev/null || true
    fi
    
    ssh-add -d ~/.ssh/id_rsa_main 2>/dev/null || true
    ssh-add -d ~/.ssh/id_rsa_backup 2>/dev/null || true
}

generate_ssh_keys() {
    echo "Generating SSH keys..."
    
    mkdir -p ~/.ssh
    chmod 700 ~/.ssh
    
    ssh-keygen -t rsa -b 4096 -f ~/.ssh/id_rsa_main -N "" -C "$MAIN_USER@github.com" -q
    ssh-keygen -t rsa -b 4096 -f ~/.ssh/id_rsa_backup -N "" -C "$BACKUP_USER@github.com" -q
    
    chmod 600 ~/.ssh/id_rsa_main ~/.ssh/id_rsa_backup
    chmod 644 ~/.ssh/id_rsa_main.pub ~/.ssh/id_rsa_backup.pub
    
    echo "SSH keys generated successfully"
}

setup_ssh_config() {
    mkdir -p ~/.ssh
    chmod 700 ~/.ssh
    
    if [ -f ~/.ssh/config ]; then
        if grep -q "Host github-main" ~/.ssh/config && grep -q "Host github-backup" ~/.ssh/config; then
            return 0
        fi
        sed -i.bak '/Host github-main/,/^$/d' ~/.ssh/config 2>/dev/null || true
        sed -i.bak '/Host github-backup/,/^$/d' ~/.ssh/config 2>/dev/null || true
        rm -f ~/.ssh/config.bak 2>/dev/null || true
    else
        touch ~/.ssh/config
    fi
    
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
    
    ssh-add ~/.ssh/id_rsa_main 2>/dev/null || true
    ssh-add ~/.ssh/id_rsa_backup 2>/dev/null || true
}

test_ssh_keys() {
    setup_ssh_agent
    
    local main_result=$(ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 -T git@github-main 2>&1 || true)
    local backup_result=$(ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 -T git@github-backup 2>&1 || true)
    
    local main_ok=false
    local backup_ok=false
    
    if echo "$main_result" | grep -q "successfully authenticated"; then
        main_ok=true
    fi
    
    if echo "$backup_result" | grep -q "successfully authenticated"; then
        backup_ok=true
    fi
    
    if [ "$main_ok" = true ] && [ "$backup_ok" = true ]; then
        echo "SSH authentication successful for both accounts"
        return 0
    else
        if [ "$main_ok" = false ]; then
            echo "Main account SSH failed: $main_result"
        fi
        if [ "$backup_ok" = false ]; then
            echo "Backup account SSH failed: $backup_result"
        fi
        return 1
    fi
}

display_keys_and_get_confirmation() {
    echo ""
    echo "=== Add these SSH keys to your GitHub accounts ==="
    echo ""
    echo "1. Main Account ($MAIN_USER): https://github.com/settings/ssh/new"
    cat ~/.ssh/id_rsa_main.pub
    echo ""
    echo "2. Backup Account ($BACKUP_USER): https://github.com/settings/ssh/new"
    cat ~/.ssh/id_rsa_backup.pub
    echo ""
    
    while true; do
        read -p "Have you added both keys to GitHub? (y/n): " confirm
        case $confirm in
            [Yy]* ) break;;
            [Nn]* ) echo "Please add the keys first, then run the script again."; exit 1;;
            * ) echo "Please answer y or n.";;
        esac
    done
}

ensure_ssh_setup() {
    local need_new_keys=false
    
    if [ ! -f ~/.ssh/id_rsa_main ] || [ ! -f ~/.ssh/id_rsa_backup ]; then
        echo "SSH keys missing, generating new ones..."
        need_new_keys=true
        cleanup_ssh_setup
        generate_ssh_keys
    else
        echo "SSH keys found, testing authentication..."
        setup_ssh_config
        setup_ssh_agent
        
        if ! test_ssh_keys; then
            echo "SSH authentication failed, generating new keys..."
            need_new_keys=true
            cleanup_ssh_setup
            generate_ssh_keys
        else
            echo "SSH keys working perfectly!"
            return 0
        fi
    fi
    
    setup_ssh_config
    setup_ssh_agent
    
    if [ "$need_new_keys" = true ]; then
        display_keys_and_get_confirmation
        
        echo "Testing new SSH keys..."
        if ! test_ssh_keys; then
            echo "SSH setup failed. Please check that you added the correct keys."
            exit 1
        fi
    fi
}

create_github_repo() {
    local user=$1
    local remote_name=$2
    
    echo "Creating repository $user/$REPO_NAME on GitHub..."
    echo "Please create the repository manually:"
    echo "1. Go to: https://github.com/new"
    echo "2. Repository name: $REPO_NAME"
    echo "3. Make it public"
    echo "4. Do NOT initialize with README, .gitignore, or license"
    
    read -p "Press Enter after creating the repository for $user..."
}

setup_remotes() {
    local main_url="git@github-main:$MAIN_USER/$REPO_NAME.git"
    local backup_url="git@github-backup:$BACKUP_USER/$REPO_NAME.git"
    
    if git remote get-url $MAIN_REMOTE >/dev/null 2>&1; then
        git remote set-url $MAIN_REMOTE $main_url
    else
        git remote add $MAIN_REMOTE $main_url
    fi
    
    if git remote get-url $BACKUP_REMOTE >/dev/null 2>&1; then
        git remote set-url $BACKUP_REMOTE $backup_url
    else
        git remote add $BACKUP_REMOTE $backup_url
    fi
}

push_to_github() {
    echo "Pushing to GitHub repositories..."
    
    local main_success=false
    local backup_success=false
    
    if git push $MAIN_REMOTE main 2>/dev/null || git push -u $MAIN_REMOTE main 2>/dev/null; then
        echo "✓ Successfully pushed to main account"
        main_success=true
    else
        echo "✗ Push to main account failed - repository likely doesn't exist"
        create_github_repo $MAIN_USER $MAIN_REMOTE
        if git push -u $MAIN_REMOTE main 2>/dev/null; then
            echo "✓ Successfully pushed to main account after creation"
            main_success=true
        else
            echo "✗ Still failed to push to main account"
        fi
    fi
    
    if git push $BACKUP_REMOTE main 2>/dev/null || git push -u $BACKUP_REMOTE main 2>/dev/null; then
        echo "✓ Successfully pushed to backup account"
        backup_success=true
    else
        echo "✗ Push to backup account failed - repository likely doesn't exist"
        create_github_repo $BACKUP_USER $BACKUP_REMOTE
        if git push -u $BACKUP_REMOTE main 2>/dev/null; then
            echo "✓ Successfully pushed to backup account after creation"
            backup_success=true
        else
            echo "✗ Still failed to push to backup account"
        fi
    fi
    
    if [ "$main_success" = true ] && [ "$backup_success" = true ]; then
        echo ""
        echo "🎉 Repository successfully synced to both accounts!"
    else
        echo ""
        echo "⚠️  Some pushes failed, but you can retry them manually"
    fi
}

main() {
    echo "🚀 Starting GitHub dual account setup for: $REPO_NAME"
    echo ""
    
    ensure_ssh_setup
    
    if [ -d ".git" ]; then
        echo ""
        echo "📁 Git repository exists"
        
        current_branch=$(git branch --show-current 2>/dev/null || echo "")
        if [ "$current_branch" != "main" ]; then
            if git show-ref --verify --quiet refs/heads/main 2>/dev/null; then
                git checkout main
            else
                git checkout -b main
            fi
        fi
        
        setup_remotes
        
        if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
            echo "📝 Changes detected, committing..."
            git add .
            git commit -m "Update $(date '+%Y-%m-%d %H:%M:%S')"
        else
            echo "✨ Working directory clean"
        fi
        
        push_to_github
        
    else
        echo ""
        echo "🆕 Initializing new git repository"
        
        git init
        git branch -M main
        
        files_count=$(find . -maxdepth 1 -type f ! -name ".*" | wc -l)
        if [ "$files_count" -eq 0 ]; then
            echo "# $REPO_NAME" > README.md
            echo "📄 Added README.md to empty repository"
        fi
        
        git add .
        git commit -m "Initial commit"
        
        setup_remotes
        push_to_github
    fi
    
    echo ""
    echo "📊 Repository URLs:"
    echo "   Main: https://github.com/$MAIN_USER/$REPO_NAME"
    echo "   Backup: https://github.com/$BACKUP_USER/$REPO_NAME"
    echo ""
    echo "🔗 Configured remotes:"
    git remote -v 2>/dev/null | sed 's/^/   /'
}

main