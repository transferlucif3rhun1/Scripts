#!/usr/bin/env python3

import os
import sys
import time
import json
import logging
import tempfile
import shutil
import subprocess
import re
from pathlib import Path
from dataclasses import dataclass, field
from typing import List, Dict, Optional, Tuple
import stat

import click
import requests
from github import Github, Auth, GithubException
import pygit2


logging.basicConfig(level=logging.INFO, format='%(levelname)s: %(message)s')
logger = logging.getLogger(__name__)


class AuthenticationError(Exception):
    pass


class LFSError(Exception):
    pass


class RepositoryError(Exception):
    pass


@dataclass
class GitHubAccount:
    username: str
    token: str
    remote_name: str
    validated: bool = False
    github_api: Optional[Github] = field(default=None, init=False)
    
    def __post_init__(self):
        if self.token:
            auth = Auth.Token(self.token)
            self.github_api = Github(auth=auth)
    
    def validate_token(self) -> bool:
        if not self.token or not self.token.startswith(('ghp_', 'github_pat_')):
            logger.error(f"Invalid token format for {self.username}")
            return False
        
        try:
            user = self.github_api.get_user()
            self.username = user.login
            self.validated = True
            logger.info(f"Token validated for user: {self.username}")
            return True
        except GithubException as e:
            logger.error(f"Token validation failed: {e}")
            return False
    
    def check_rate_limit(self) -> Dict:
        try:
            rate_limit = self.github_api.get_rate_limit()
            return {
                'remaining': rate_limit.core.remaining,
                'limit': rate_limit.core.limit,
                'reset': rate_limit.core.reset
            }
        except Exception:
            return {'remaining': 0, 'limit': 5000, 'reset': time.time() + 3600}
    
    def create_repository(self, repo_name: str, description: str = None) -> bool:
        try:
            rate_limit = self.check_rate_limit()
            if rate_limit['remaining'] < 10:
                sleep_time = rate_limit['reset'].timestamp() - time.time() + 5
                if sleep_time > 0:
                    logger.warning(f"Rate limit low, sleeping {sleep_time}s")
                    time.sleep(sleep_time)
            
            try:
                repo = self.github_api.get_user().get_repo(repo_name)
                logger.info(f"Repository {repo_name} already exists")
                return True
            except GithubException as e:
                if e.status != 404:
                    raise
            
            user = self.github_api.get_user()
            repo = user.create_repo(
                name=repo_name,
                description=description or f"Synchronized repository created by dual account script",
                private=False,
                auto_init=False,
                has_issues=True,
                has_projects=True,
                has_wiki=True
            )
            logger.info(f"Repository {repo_name} created successfully")
            
            for attempt in range(30):
                try:
                    test_url = f"https://{self.username}:{self.token}@github.com/{self.username}/{repo_name}.git"
                    subprocess.run(['git', 'ls-remote', test_url, 'HEAD'], 
                                 capture_output=True, check=True, timeout=10)
                    return True
                except (subprocess.CalledProcessError, subprocess.TimeoutExpired):
                    if attempt < 29:
                        time.sleep(2)
                        continue
                    break
            
            logger.warning(f"Repository {repo_name} created but not immediately accessible")
            return True
            
        except Exception as e:
            logger.error(f"Failed to create repository {repo_name}: {e}")
            return False


class CredentialManager:
    @staticmethod
    def setup_git_credentials(username: str, token: str, hostname: str = "github.com"):
        creds_file = Path.home() / ".git-credentials"
        credential_line = f"https://{username}:{token}@{hostname}"
        
        existing_lines = []
        if creds_file.exists():
            with open(creds_file) as f:
                existing_lines = [line.strip() for line in f if hostname not in line]
        
        existing_lines.append(credential_line)
        
        with tempfile.NamedTemporaryFile(mode='w', delete=False, dir=creds_file.parent) as temp:
            for line in existing_lines:
                temp.write(line + '\n')
            temp_path = temp.name
        
        os.chmod(temp_path, stat.S_IRUSR | stat.S_IWUSR)
        shutil.move(temp_path, creds_file)
        
        subprocess.run(['git', 'config', '--global', 'credential.helper', 'store'], check=True)
        logger.info(f"Git credentials configured for {username}@{hostname}")
    
    @staticmethod
    def setup_netrc(username: str, token: str, hostname: str = "github.com"):
        netrc_file = Path.home() / ".netrc"
        
        netrc_entry = f"""machine {hostname}
login {username}
password {token}
protocol https

"""
        
        existing_content = ""
        if netrc_file.exists():
            with open(netrc_file) as f:
                content = f.read()
            
            lines = content.split('\n')
            filtered_lines = []
            skip_next = 0
            
            for i, line in enumerate(lines):
                if skip_next > 0:
                    skip_next -= 1
                    continue
                
                if line.strip().startswith('machine') and hostname in line:
                    skip_next = 4
                    continue
                
                filtered_lines.append(line)
            
            existing_content = '\n'.join(filtered_lines)
        
        with tempfile.NamedTemporaryFile(mode='w', delete=False, dir=netrc_file.parent) as temp:
            temp.write(existing_content)
            if existing_content and not existing_content.endswith('\n'):
                temp.write('\n')
            temp.write(netrc_entry)
            temp_path = temp.name
        
        os.chmod(temp_path, stat.S_IRUSR | stat.S_IWUSR)
        shutil.move(temp_path, netrc_file)
        logger.info(f"Netrc configured for {username}@{hostname}")


class LFSManager:
    def __init__(self, repo_path: Path):
        self.repo_path = repo_path
        self.lfs_configured = False
    
    def is_lfs_available(self) -> bool:
        try:
            result = subprocess.run(['git', 'lfs', 'version'], capture_output=True, check=True)
            return 'git-lfs' in result.stdout.decode().lower()
        except (subprocess.CalledProcessError, FileNotFoundError):
            return False
    
    def install_lfs_if_needed(self) -> bool:
        if self.is_lfs_available():
            return True
        
        logger.warning("Git LFS not found, attempting to install...")
        
        if sys.platform == "darwin":
            try:
                subprocess.run(['brew', 'install', 'git-lfs'], check=True)
                return self.is_lfs_available()
            except (subprocess.CalledProcessError, FileNotFoundError):
                pass
        elif sys.platform.startswith("linux"):
            try:
                subprocess.run(['sudo', 'apt-get', 'update'], check=True)
                subprocess.run(['sudo', 'apt-get', 'install', '-y', 'git-lfs'], check=True)
                return self.is_lfs_available()
            except subprocess.CalledProcessError:
                pass
        
        logger.error("Could not install Git LFS automatically")
        return False
    
    def detect_large_files(self, threshold_mb: int = 50) -> List[Path]:
        threshold_bytes = threshold_mb * 1024 * 1024
        large_files = []
        
        try:
            if (self.repo_path / '.git').exists():
                result = subprocess.run(['git', 'ls-files'], cwd=self.repo_path, 
                                      capture_output=True, text=True, check=True)
                tracked_files = result.stdout.strip().split('\n') if result.stdout.strip() else []
                
                result = subprocess.run(['git', 'ls-files', '--others', '--exclude-standard'], 
                                      cwd=self.repo_path, capture_output=True, text=True, check=True)
                untracked_files = result.stdout.strip().split('\n') if result.stdout.strip() else []
                
                all_files = tracked_files + untracked_files
            else:
                all_files = [str(f.relative_to(self.repo_path)) for f in self.repo_path.rglob('*') if f.is_file()]
            
            for file_rel in all_files:
                if not file_rel:
                    continue
                file_path = self.repo_path / file_rel
                if file_path.exists() and file_path.is_file():
                    try:
                        if file_path.stat().st_size > threshold_bytes:
                            large_files.append(file_path)
                    except (OSError, IOError):
                        continue
        
        except subprocess.CalledProcessError:
            for file_path in self.repo_path.rglob('*'):
                if file_path.is_file():
                    try:
                        if file_path.stat().st_size > threshold_bytes:
                            large_files.append(file_path)
                    except (OSError, IOError):
                        continue
        
        return large_files
    
    def configure_lfs(self, accounts: List[GitHubAccount]) -> bool:
        if not self.install_lfs_if_needed():
            return False
        
        try:
            subprocess.run(['git', 'lfs', 'install', '--local'], cwd=self.repo_path, check=True)
            
            for account in accounts:
                self._configure_lfs_auth(account)
            
            self.lfs_configured = True
            logger.info("Git LFS configured successfully")
            return True
            
        except subprocess.CalledProcessError as e:
            logger.error(f"Failed to configure Git LFS: {e}")
            return False
    
    def _configure_lfs_auth(self, account: GitHubAccount):
        config_commands = [
            ['git', 'config', '--local', 'lfs.batch', 'true'],
            ['git', 'config', '--local', 'lfs.transfer.maxretries', '3'],
<<<<<<< Updated upstream
            ['git', 'config', '--local', f'lfs.https://github.com/{account.username}.git/info/lfs.access', 'basic'],
            ['git', 'config', '--local', f'credential.https://github.com.helper', 'store'],
            ['git', 'config', '--local', 'credential.useHttpPath', 'true']
        ]
        
        for cmd in config_commands:
            try:
                subprocess.run(cmd, cwd=self.repo_path, check=True)
            except subprocess.CalledProcessError:
                logger.warning(f"Failed to run LFS config command: {' '.join(cmd)}")
        
        lfs_url = f"https://{account.username}:{account.token}@github.com/{account.username}.git/info/lfs"
        try:
            subprocess.run(['git', 'config', '--local', 'lfs.url', lfs_url], 
                         cwd=self.repo_path, check=True)
        except subprocess.CalledProcessError:
            pass
    
    def track_large_files(self, files: List[Path]) -> bool:
        if not files:
            return True
        
        try:
            for file_path in files:
                relative_path = file_path.relative_to(self.repo_path)
                subprocess.run(['git', 'lfs', 'track', str(relative_path)], 
                             cwd=self.repo_path, check=True)
                logger.info(f"Tracking {relative_path} with Git LFS")
            
            return True
            
        except subprocess.CalledProcessError as e:
            logger.error(f"Failed to track files with LFS: {e}")
            return False
    
    def migrate_existing_files(self, files: List[Path]) -> bool:
        if not files:
            return True
        
        try:
            tracked_files = []
            for file_path in files:
                relative_path = file_path.relative_to(self.repo_path)
                result = subprocess.run(['git', 'ls-files', '--error-unmatch', str(relative_path)], 
                                      cwd=self.repo_path, capture_output=True)
                if result.returncode == 0:
                    tracked_files.append(str(relative_path))
            
            if not tracked_files:
                return True
            
            logger.info(f"Migrating {len(tracked_files)} existing files to LFS")
            
            is_clean = subprocess.run(['git', 'diff-index', '--quiet', 'HEAD', '--'], 
                                    cwd=self.repo_path).returncode == 0
            
            stashed = False
            if not is_clean:
                try:
                    subprocess.run(['git', 'add', '.gitattributes'], cwd=self.repo_path, capture_output=True)
                    subprocess.run(['git', 'stash', 'push', '-m', 'temp for LFS migration'], 
                                 cwd=self.repo_path, check=True)
                    stashed = True
                except subprocess.CalledProcessError:
                    subprocess.run(['git', 'reset', '--hard', 'HEAD'], cwd=self.repo_path, check=True)
            
            try:
                include_patterns = ','.join(tracked_files)
                subprocess.run(['git', 'lfs', 'migrate', 'import', '--yes', 
                              '--everything', '--include', include_patterns], 
                             cwd=self.repo_path, check=True)
                logger.info(f"Successfully migrated {len(tracked_files)} files to LFS")
            finally:
                if stashed:
                    result = subprocess.run(['git', 'stash', 'pop'], cwd=self.repo_path, 
                                          capture_output=True)
                    if result.returncode != 0:
                        subprocess.run(['git', 'stash', 'drop'], cwd=self.repo_path, capture_output=True)
            
            return True
=======
            ['git', 'config', '--local', f'lfs.https://github.com/{account.username}.git/info/lfs.access', 'basic']
        ]
        
        for cmd in config_commands:
            try:
                subprocess.run(cmd, cwd=self.repo_path, check=True)
            except subprocess.CalledProcessError:
                logger.warning(f"Failed to run LFS config command: {' '.join(cmd)}")
    
    def track_large_files(self, files: List[Path]) -> bool:
        if not files:
            return True
        
        try:
            for file_path in files:
                relative_path = file_path.relative_to(self.repo_path)
                subprocess.run(['git', 'lfs', 'track', str(relative_path)], 
                             cwd=self.repo_path, check=True)
                logger.info(f"Tracking {relative_path} with Git LFS")
            
            return True
            
        except subprocess.CalledProcessError as e:
            logger.error(f"Failed to track files with LFS: {e}")
            return False
    
    def migrate_existing_files(self, files: List[Path]) -> bool:
        if not files:
            return True
        
        try:
            for file_path in files:
                relative_path = file_path.relative_to(self.repo_path)
                
                result = subprocess.run(['git', 'ls-files', '--error-unmatch', str(relative_path)], 
                                      cwd=self.repo_path, capture_output=True)
                
                if result.returncode == 0:
                    logger.info(f"Migrating existing file {relative_path} to LFS")
                    
                    is_clean = subprocess.run(['git', 'diff-index', '--quiet', 'HEAD', '--'], 
                                            cwd=self.repo_path).returncode == 0
                    
                    if not is_clean:
                        subprocess.run(['git', 'stash', 'push', '-m', 'temp for LFS migration'], 
                                     cwd=self.repo_path, check=True)
                    
                    try:
                        subprocess.run(['git', 'lfs', 'migrate', 'import', '--yes', 
                                      '--everything', '--include', str(relative_path)], 
                                     cwd=self.repo_path, check=True)
                    finally:
                        if not is_clean:
                            subprocess.run(['git', 'stash', 'pop'], cwd=self.repo_path, 
                                         capture_output=True)
            
            return True
>>>>>>> Stashed changes
            
        except subprocess.CalledProcessError as e:
            logger.error(f"Failed to migrate files to LFS: {e}")
            return False


class GitRepository:
    def __init__(self, repo_path: Path):
        self.repo_path = repo_path
        self.repo = None
        self.lfs_manager = LFSManager(repo_path)
    
    def initialize_repository(self) -> bool:
        try:
            if (self.repo_path / '.git').exists():
                self.repo = pygit2.Repository(str(self.repo_path))
                logger.info("Repository found")
            else:
                self.repo = pygit2.init_repository(str(self.repo_path), False)
                logger.info("Repository initialized")
            
            subprocess.run(['git', 'config', '--local', 'init.defaultBranch', 'main'], 
                         cwd=self.repo_path, check=True)
            
            return True
            
        except Exception as e:
            logger.error(f"Failed to initialize repository: {e}")
            return False
    
    def setup_remotes(self, accounts: List[GitHubAccount], repo_name: str):
        if not self.repo:
            return False
        
        try:
            for account in accounts:
                remote_url = f"https://{account.username}:{account.token}@github.com/{account.username}/{repo_name}.git"
                
                try:
                    remote = self.repo.remotes[account.remote_name]
                    self.repo.remotes.delete(account.remote_name)
                    self.repo.remotes.create(account.remote_name, remote_url)
                    logger.info(f"Updated remote {account.remote_name}")
                except KeyError:
                    self.repo.remotes.create(account.remote_name, remote_url)
                    logger.info(f"Added remote {account.remote_name}")
            
            subprocess.run(['git', 'config', '--local', 'push.default', 'current'], 
                         cwd=self.repo_path, check=True)
            
            return True
            
        except Exception as e:
            logger.error(f"Failed to setup remotes: {e}")
            return False
    
    def ensure_main_branch(self) -> bool:
        try:
            if not self.repo.head_is_unborn:
                current_branch = self.repo.head.shorthand
                if current_branch != 'main':
                    try:
                        main_branch = self.repo.lookup_branch('main')
                        if main_branch:
                            self.repo.checkout(main_branch)
                        else:
                            subprocess.run(['git', 'checkout', '-b', 'main'], 
                                         cwd=self.repo_path, check=True)
                        logger.info("Switched to main branch")
                    except:
                        subprocess.run(['git', 'checkout', '-b', 'main'], 
                                     cwd=self.repo_path, check=True)
                        logger.info("Created main branch")
            
            return True
            
        except Exception as e:
            logger.error(f"Failed to ensure main branch: {e}")
            return False
    
    def create_gitignore(self):
        gitignore_items = [
            "*.log", "*.tmp", "*.cache", "*.swp", "*.swo", "*~",
            ".DS_Store", ".DS_Store?", "._*", ".Spotlight-V100", ".Trashes",
            "ehthumbs.db", "Thumbs.db", "Desktop.ini",
            "node_modules/", "npm-debug.log*", "yarn-debug.log*", "yarn-error.log*",
            "package-lock.json", "yarn.lock",
            "__pycache__/", "*.pyc", "*.pyo", "*.pyd", ".Python",
            "env/", "venv/", ".env", ".env.local", ".env.development", ".env.test", ".env.production",
            "config.json", "config.local.json", "secrets.json", "credentials.json",
            ".vscode/", ".idea/", "*.sublime-project", "*.sublime-workspace", ".vs/",
            "dist/", "build/", "out/", "*.egg-info/",
            ".coverage", ".pytest_cache/", ".mypy_cache/", ".tox/",
            "target/", "*.class", "*.jar", "*.war", "*.ear",
            ".gradle/", "*.iml", "*.ipr", "*.iws",
            ".next/", ".nuxt/", ".output/", ".temp/", ".sass-cache/", ".parcel-cache/"
        ]
        
        gitignore_path = self.repo_path / '.gitignore'
        
        if gitignore_path.exists():
            with open(gitignore_path) as f:
                existing_content = f.read()
            
            new_items = [item for item in gitignore_items if item not in existing_content]
            
            if new_items:
                with open(gitignore_path, 'a') as f:
                    f.write('\n# Auto-added by sync script\n')
                    for item in new_items:
                        f.write(f'{item}\n')
                logger.info(f"Added {len(new_items)} items to .gitignore")
        else:
            with open(gitignore_path, 'w') as f:
                f.write('# Generated by GitHub Dual Account Sync script\n')
                f.write(f'# {time.strftime("%Y-%m-%d %H:%M:%S")}\n\n')
                for item in gitignore_items:
                    f.write(f'{item}\n')
            logger.info("Created .gitignore")
    
    def create_readme_if_needed(self, repo_name: str, main_user: str, backup_user: str):
        readme_path = self.repo_path / 'README.md'
        
        if not readme_path.exists():
            readme_content = f"""# {repo_name}

This repository is synchronized to multiple GitHub accounts.

## Accounts
- Main: https://github.com/{main_user}/{repo_name}
- Backup: https://github.com/{backup_user}/{repo_name}

Created on {time.strftime("%Y-%m-%d %H:%M:%S")}
"""
            with open(readme_path, 'w') as f:
                f.write(readme_content)
            logger.info("Created README.md")
    
    def commit_changes(self, message: str) -> bool:
        try:
            subprocess.run(['git', 'add', '-A'], cwd=self.repo_path, check=True)
            
            result = subprocess.run(['git', 'diff', '--cached', '--quiet'], 
                                  cwd=self.repo_path, capture_output=True)
            
            if result.returncode == 0:
                logger.info("No changes to commit")
                return True
            
            subprocess.run(['git', 'commit', '-m', message], cwd=self.repo_path, check=True)
            logger.info("Changes committed")
            return True
            
        except subprocess.CalledProcessError as e:
            logger.error(f"Failed to commit changes: {e}")
            return False
    
    def push_to_remote(self, account: GitHubAccount, repo_name: str, max_retries: int = 3) -> bool:
        for attempt in range(max_retries):
            try:
                logger.info(f"Pushing to {account.username} (attempt {attempt + 1}/{max_retries})")
                
                result = subprocess.run(['git', 'push', account.remote_name, 'main'], 
                                      cwd=self.repo_path, capture_output=True, text=True, timeout=300)
                
                if result.returncode == 0:
                    if 'Everything up-to-date' in result.stderr:
                        logger.info(f"✓ {account.username}: Already up-to-date")
                    else:
                        logger.info(f"✓ {account.username}: Push successful")
                    return True
                
                error_output = result.stderr.lower()
                
                if any(phrase in error_output for phrase in ['rejected', 'non-fast-forward', 'updates were rejected']):
                    if self._handle_push_conflict(account, repo_name):
                        continue
                    return False
                
                elif 'repository not found' in error_output or '404' in error_output:
                    logger.info(f"Repository not found for {account.username}, creating...")
                    if account.create_repository(repo_name):
                        time.sleep(5)
                        continue
                    return False
                
                elif any(phrase in error_output for phrase in ['authentication failed', '401', '403', 'bad credentials']):
                    logger.error(f"Authentication failed for {account.username}")
                    return False
                
                elif 'rate limit' in error_output:
                    rate_limit = account.check_rate_limit()
                    sleep_time = rate_limit['reset'].timestamp() - time.time() + 60
                    if sleep_time > 0 and sleep_time < 3600:
                        logger.warning(f"Rate limited, waiting {sleep_time}s")
                        time.sleep(sleep_time)
                        continue
                    return False
                
                elif any(phrase in error_output for phrase in ['larger', 'file size limit', 'exceeds']):
                    if self._handle_large_file_error(account):
                        continue
                    return False
                
                elif any(phrase in error_output for phrase in ['network', 'timeout', 'connection', 'could not resolve']):
                    if attempt < max_retries - 1:
                        logger.warning(f"Network error, retrying in 5 seconds...")
                        time.sleep(5)
                        continue
                    return False
                
                logger.error(f"Push failed for {account.username}: {result.stderr}")
                return False
                
            except subprocess.TimeoutExpired:
                logger.error(f"Push timeout for {account.username}")
                if attempt < max_retries - 1:
                    time.sleep(5)
                    continue
                return False
            
            except Exception as e:
                logger.error(f"Push error for {account.username}: {e}")
                if attempt < max_retries - 1:
                    time.sleep(5)
                    continue
                return False
        
        return False
    
    def _handle_push_conflict(self, account: GitHubAccount, repo_name: str) -> bool:
        try:
            logger.info("Repository diverged, attempting merge...")
            
            subprocess.run(['git', 'fetch', account.remote_name], cwd=self.repo_path, check=True)
            
            result = subprocess.run(['git', 'merge', f'{account.remote_name}/main', '--no-edit'], 
                                  cwd=self.repo_path, capture_output=True, text=True)
            
            if result.returncode == 0:
                logger.info("Merge successful, retrying push...")
                return True
            
            if 'CONFLICT' in result.stdout:
                logger.error("Merge conflicts detected. Manual resolution required:")
                conflict_files = []
                for line in result.stdout.split('\n'):
                    if 'CONFLICT' in line:
                        logger.error(f"  {line}")
                
                subprocess.run(['git', 'merge', '--abort'], cwd=self.repo_path, capture_output=True)
                return False
            
            logger.error(f"Merge failed: {result.stderr}")
            return False
            
        except Exception as e:
            logger.error(f"Failed to handle push conflict: {e}")
            return False
    
    def _handle_large_file_error(self, account: GitHubAccount) -> bool:
        logger.warning("Large file detected, configuring Git LFS...")
        
        if not self.lfs_manager.lfs_configured:
            large_files = self.lfs_manager.detect_large_files()
            if large_files:
                logger.info(f"Found {len(large_files)} large files")
                
                if self.lfs_manager.configure_lfs([account]):
                    if self.lfs_manager.track_large_files(large_files):
                        if self.lfs_manager.migrate_existing_files(large_files):
                            if self.commit_changes("Configure Git LFS for large files"):
                                return True
        
        logger.error("Failed to configure Git LFS for large files")
        return False


class DualAccountSync:
    def __init__(self, main_account: GitHubAccount, backup_account: GitHubAccount, 
                 repo_path: Path, dry_run: bool = False):
        self.main_account = main_account
        self.backup_account = backup_account
        self.repo_path = repo_path
        self.dry_run = dry_run
        self.git_repo = GitRepository(repo_path)
    
    def setup_authentication(self) -> bool:
        logger.info("Setting up authentication...")
        
        try:
            for account in [self.main_account, self.backup_account]:
                if not account.validate_token():
                    logger.error(f"Token validation failed for {account.username}")
                    return False
                
                if not self.dry_run:
                    CredentialManager.setup_git_credentials(account.username, account.token)
                    CredentialManager.setup_netrc(account.username, account.token)
            
            logger.info("Authentication setup complete")
            return True
            
        except Exception as e:
            logger.error(f"Authentication setup failed: {e}")
            return False
    
    def sync_repository(self, repo_name: str, commit_message: str = None) -> bool:
        logger.info(f"🚀 GitHub Dual Account Sync: {repo_name}")
        
        if self.dry_run:
            logger.info("🔍 DRY RUN MODE - No changes will be made")
        
        try:
            if not self.git_repo.initialize_repository():
                return False
            
            repo_size = sum(f.stat().st_size for f in self.repo_path.rglob('*') if f.is_file()) / (1024**3)
            if repo_size > 1:
                logger.warning(f"Large repository detected: {repo_size:.1f}GB")
            
            if not self.git_repo.setup_remotes([self.main_account, self.backup_account], repo_name):
                return False
            
            if not self.git_repo.ensure_main_branch():
                return False
            
            large_files = self.git_repo.lfs_manager.detect_large_files()
            if large_files:
                logger.info(f"Large files detected (>50MB):")
                for file_path in large_files:
                    size_mb = file_path.stat().st_size / (1024**2)
                    logger.info(f"  - {file_path.relative_to(self.repo_path)} ({size_mb:.1f}MB)")
                
                if not self.dry_run:
                    if not self.git_repo.lfs_manager.configure_lfs([self.main_account, self.backup_account]):
                        logger.error("Failed to configure Git LFS")
                        return False
                    
                    if not self.git_repo.lfs_manager.track_large_files(large_files):
                        return False
                    
                    if not self.git_repo.lfs_manager.migrate_existing_files(large_files):
                        return False
            
            if not self.dry_run:
                self.git_repo.create_gitignore()
                self.git_repo.create_readme_if_needed(repo_name, self.main_account.username, 
                                                    self.backup_account.username)
                
                if not commit_message:
                    if large_files:
                        commit_message = f"Update with Git LFS configuration {time.strftime('%Y-%m-%d %H:%M:%S')}"
                    else:
                        commit_message = f"Update {time.strftime('%Y-%m-%d %H:%M:%S')}"
                
                if not self.git_repo.commit_changes(commit_message):
                    return False
            
            return self.push_to_accounts(repo_name)
            
        except Exception as e:
            logger.error(f"Repository sync failed: {e}")
            return False
    
    def push_to_accounts(self, repo_name: str) -> bool:
        logger.info("Synchronizing repositories...")
        
        main_success = False
        backup_success = False
        
        if self.dry_run:
            logger.info("✓ DRY RUN: Would push to both accounts")
            return True
        
        logger.info(f"├── Phase 1: Processing main account ({self.main_account.username})...")
        if self.git_repo.push_to_remote(self.main_account, repo_name):
            main_success = True
            logger.info("├── ✓ Main account synchronized successfully")
        else:
            logger.error("├── ✗ Main account synchronization failed")
        
        logger.info(f"├── Phase 2: Processing backup account ({self.backup_account.username})...")
        if self.git_repo.push_to_remote(self.backup_account, repo_name):
            backup_success = True
            logger.info("├── ✓ Backup account synchronized successfully")
        else:
            logger.error("├── ✗ Backup account synchronization failed")
        
        logger.info("└── Summary:")
        if main_success and backup_success:
            logger.info("    🎉 Both repositories synchronized successfully")
            logger.info("📊 Repository Links:")
            logger.info(f"├── Main: https://github.com/{self.main_account.username}/{repo_name}")
            logger.info(f"└── Backup: https://github.com/{self.backup_account.username}/{repo_name}")
            return True
        elif main_success or backup_success:
            logger.warning("    ⚠️  Partial synchronization completed")
            if main_success:
                logger.info("    ✓ Main repository updated")
            if backup_success:
                logger.info("    ✓ Backup repository updated")
            return False
        else:
            logger.error("    ✗ Synchronization failed for both repositories")
            return False


def get_github_tokens() -> Tuple[str, str]:
    main_token = os.getenv('GITHUB_MAIN_TOKEN')
    backup_token = os.getenv('GITHUB_BACKUP_TOKEN')
    
    token_file = Path.home() / '.github_dual_tokens'
    if token_file.exists():
        try:
            with open(token_file) as f:
                for line in f:
                    line = line.strip()
                    if not line or line.startswith('#'):
                        continue
                    
                    if '=' in line:
                        key, value = line.split('=', 1)
                        key = key.strip()
                        value = value.strip().strip('"\'')
                        
                        if key == 'MAIN_TOKEN' and not main_token:
                            main_token = value
                        elif key == 'BACKUP_TOKEN' and not backup_token:
                            backup_token = value
        except Exception:
            pass
    
    if not main_token:
        main_token = click.prompt("Main GitHub token", hide_input=True)
    
    if not backup_token:
        backup_token = click.prompt("Backup GitHub token", hide_input=True)
    
    if not token_file.exists() or not main_token or not backup_token:
        try:
            token_file.parent.mkdir(parents=True, exist_ok=True)
            with tempfile.NamedTemporaryFile(mode='w', delete=False, dir=token_file.parent) as temp:
                temp.write(f'MAIN_TOKEN="{main_token}"\n')
                temp.write(f'BACKUP_TOKEN="{backup_token}"\n')
                temp_path = temp.name
            
            os.chmod(temp_path, stat.S_IRUSR | stat.S_IWUSR)
            shutil.move(temp_path, token_file)
            logger.info("✓ Tokens saved securely")
        except Exception:
            logger.warning("Failed to save tokens")
    
    return main_token, backup_token


@click.command()
@click.option('-m', '--message', help='Commit message')
@click.option('-v', '--verbose', is_flag=True, help='Enable verbose output')
@click.option('-d', '--dry-run', is_flag=True, help='Dry run mode (no changes)')
@click.option('--main-user', default='lucif3rhun1', help='Main GitHub username')
@click.option('--backup-user', default='transferlucif3rhun1', help='Backup GitHub username')
@click.option('--main-remote', default='origin', help='Main remote name')
@click.option('--backup-remote', default='backup', help='Backup remote name')
def main(message, verbose, dry_run, main_user, backup_user, main_remote, backup_remote):
    if verbose:
        logging.getLogger().setLevel(logging.DEBUG)
    
    repo_path = Path.cwd()
    repo_name = repo_path.name
    
    if not re.match(r'^[a-zA-Z0-9][a-zA-Z0-9._-]*[a-zA-Z0-9]$|^[a-zA-Z0-9]$', repo_name):
        logger.error(f"Invalid repository name: {repo_name}")
        sys.exit(1)
    
    if len(repo_name) > 100:
        logger.error(f"Repository name too long: {repo_name}")
        sys.exit(1)
    
    try:
        main_token, backup_token = get_github_tokens()
        
        main_account = GitHubAccount(main_user, main_token, main_remote)
        backup_account = GitHubAccount(backup_user, backup_token, backup_remote)
        
        sync = DualAccountSync(main_account, backup_account, repo_path, dry_run)
        
        if not sync.setup_authentication():
            sys.exit(1)
        
        if not sync.sync_repository(repo_name, message):
            sys.exit(1)
        
        logger.info("🎯 Synchronization completed successfully!")
        
    except KeyboardInterrupt:
        logger.info("Operation cancelled by user")
        sys.exit(1)
    except Exception as e:
        logger.error(f"Unexpected error: {e}")
        sys.exit(1)


if __name__ == '__main__':
    main()