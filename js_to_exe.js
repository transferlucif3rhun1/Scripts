#!/usr/bin/env node

const fs = require('fs');
const path = require('path');
const { execSync, spawn } = require('child_process');
const readline = require('readline');
const os = require('os');

const rl = readline.createInterface({
  input: process.stdin,
  output: process.stdout
});

const colors = {
  reset: "\x1b[0m",
  bright: "\x1b[1m",
  red: "\x1b[31m",
  green: "\x1b[32m",
  yellow: "\x1b[33m",
  blue: "\x1b[34m",
  cyan: "\x1b[36m"
};

function log(message, type = 'info') {
  switch (type) {
    case 'success': console.log(`${colors.green}✓ ${message}${colors.reset}`); break;
    case 'error': console.error(`${colors.red}✗ ${message}${colors.reset}`); break;
    case 'warning': console.warn(`${colors.yellow}⚠ ${message}${colors.reset}`); break;
    case 'info': console.log(`${colors.blue}ℹ ${message}${colors.reset}`); break;
    case 'title': console.log(`\n${colors.bright}${colors.cyan}=== ${message} ===${colors.reset}\n`); break;
    default: console.log(message);
  }
}

function executeCommand(command, options = {}) {
  return new Promise((resolve, reject) => {
    try {
      log(`Running: ${command}`, 'info');
      
      const proc = spawn(command, [], {
        ...options,
        shell: true,
        stdio: 'inherit'
      });

      proc.on('close', (code) => {
        if (code === 0) {
          resolve();
        } else {
          reject(new Error(`Command failed with exit code ${code}`));
        }
      });

      proc.on('error', (err) => {
        reject(err);
      });
    } catch (error) {
      reject(error);
    }
  });
}

function runCommandSync(command, options = {}) {
  try {
    log(`Running: ${command}`, 'info');
    return execSync(command, { 
      ...options, 
      encoding: 'utf8',
      maxBuffer: 10 * 1024 * 1024
    });
  } catch (error) {
    log(`Error executing command: ${command}`, 'error');
    log(error.message, 'error');
    throw error;
  }
}

function createTempDir() {
  const tmpDirBase = path.join(os.tmpdir(), 'js2exe-');
  const tempDir = fs.mkdtempSync(tmpDirBase);
  log(`Created temp directory: ${tempDir}`, 'success');
  return tempDir;
}

function findEntryPoint(inputPath) {
  if (fs.statSync(inputPath).isFile()) {
    return {
      entryFile: inputPath,
      projectDir: path.dirname(inputPath)
    };
  }
  
  const projectDir = inputPath;
  let entryFile = null;
  
  const packageJsonPath = path.join(projectDir, 'package.json');
  if (fs.existsSync(packageJsonPath)) {
    try {
      const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, 'utf8'));
      
      if (packageJson.main) {
        const mainPath = path.join(projectDir, packageJson.main);
        if (fs.existsSync(mainPath) && fs.statSync(mainPath).isFile()) {
          entryFile = mainPath;
        } else {
          const mainWithJs = `${mainPath}.js`;
          if (fs.existsSync(mainWithJs) && fs.statSync(mainWithJs).isFile()) {
            entryFile = mainWithJs;
          }
        }
      }
    } catch (error) {
      log(`Warning: Error parsing package.json: ${error.message}`, 'warning');
    }
  }
  
  if (!entryFile) {
    const commonEntryFiles = [
      'index.js', 'app.js', 'server.js', 'main.js',
      'src/index.js', 'src/app.js', 'src/server.js', 'src/main.js'
    ];
    
    for (const file of commonEntryFiles) {
      const filePath = path.join(projectDir, file);
      if (fs.existsSync(filePath) && fs.statSync(filePath).isFile()) {
        entryFile = filePath;
        break;
      }
    }
  }
  
  if (!entryFile) {
    throw new Error('Could not find entry point JS file');
  }
  
  return { entryFile, projectDir };
}

function extractDependenciesFromFile(filePath) {
  try {
    const content = fs.readFileSync(filePath, 'utf8');
    const requireRegex = /require\(['"]([^./][^'"\n]+)['"]\)/g;
    const deps = new Set();
    
    let match;
    while ((match = requireRegex.exec(content)) !== null) {
      const dep = match[1].split('/')[0];
      if (!isBuiltInModule(dep)) {
        deps.add(dep);
      }
    }
    
    return Array.from(deps);
  } catch (error) {
    log(`Error extracting dependencies: ${error.message}`, 'warning');
    return [];
  }
}

function isBuiltInModule(name) {
  const builtinModules = [
    'fs', 'path', 'http', 'https', 'net', 'os', 'child_process',
    'crypto', 'events', 'stream', 'util', 'buffer', 'url', 'querystring'
  ];
  return builtinModules.includes(name);
}

function copyProject(sourcePath, entryFile, tempDir) {
  if (fs.statSync(sourcePath).isFile()) {
    const fileName = path.basename(sourcePath);
    const sourceDir = path.dirname(sourcePath);
    
    const packageJsonPath = path.join(sourceDir, 'package.json');
    if (fs.existsSync(packageJsonPath)) {
      fs.copyFileSync(packageJsonPath, path.join(tempDir, 'package.json'));
    } else {
      const deps = extractDependenciesFromFile(sourcePath);
      
      const dependencies = {};
      deps.forEach(dep => {
        dependencies[dep] = "*";
      });

      const minimalPackageJson = {
        name: path.basename(entryFile, path.extname(entryFile)),
        version: "1.0.0",
        main: fileName,
        dependencies: dependencies
      };
      
      fs.writeFileSync(
        path.join(tempDir, 'package.json'),
        JSON.stringify(minimalPackageJson, null, 2)
      );
    }
    
    fs.copyFileSync(entryFile, path.join(tempDir, fileName));
    
    return {
      tempEntryFile: path.join(tempDir, fileName),
      tempPackageJsonPath: path.join(tempDir, 'package.json')
    };
  }
  
  const copyDir = (src, dest) => {
    if (!fs.existsSync(dest)) {
      fs.mkdirSync(dest, { recursive: true });
    }
    
    const entries = fs.readdirSync(src, { withFileTypes: true });
    
    for (const entry of entries) {
      const srcPath = path.join(src, entry.name);
      const destPath = path.join(dest, entry.name);
      
      if (entry.name === 'node_modules' || entry.name === '.git') {
        continue;
      }
      
      if (entry.isDirectory()) {
        copyDir(srcPath, destPath);
      } else {
        fs.copyFileSync(srcPath, destPath);
      }
    }
  };
  
  copyDir(sourcePath, tempDir);
  
  const relativePath = path.relative(sourcePath, entryFile);
  const tempEntryFile = path.join(tempDir, relativePath);
  
  return {
    tempEntryFile,
    tempPackageJsonPath: path.join(tempDir, 'package.json')
  };
}

function preparePackageJson(tempPackageJsonPath, tempEntryFile) {
  let packageJson;
  
  if (fs.existsSync(tempPackageJsonPath)) {
    packageJson = JSON.parse(fs.readFileSync(tempPackageJsonPath, 'utf8'));
  } else {
    const fileName = path.basename(tempEntryFile);
    const deps = extractDependenciesFromFile(tempEntryFile);
    
    const dependencies = {};
    deps.forEach(dep => {
      dependencies[dep] = "*";
    });
    
    packageJson = {
      name: path.basename(tempEntryFile, path.extname(tempEntryFile)),
      version: "1.0.0",
      main: fileName,
      dependencies: dependencies
    };
  }
  
  packageJson.bin = packageJson.bin || packageJson.main;
  packageJson.pkg = packageJson.pkg || {
    assets: [
      "node_modules/**/*"
    ]
  };
  
  fs.writeFileSync(tempPackageJsonPath, JSON.stringify(packageJson, null, 2));
  
  return packageJson;
}

async function installDependencies(tempDir, tempEntryFile) {
  try {
    const packageJsonPath = path.join(tempDir, 'package.json');
    const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, 'utf8'));
    
    // Check if dependencies need to be updated from the file
    const fileDeps = extractDependenciesFromFile(tempEntryFile);
    let updated = false;
    
    if (!packageJson.dependencies) {
      packageJson.dependencies = {};
    }
    
    fileDeps.forEach(dep => {
      if (!packageJson.dependencies[dep]) {
        packageJson.dependencies[dep] = "*";
        updated = true;
      }
    });
    
    if (updated) {
      fs.writeFileSync(packageJsonPath, JSON.stringify(packageJson, null, 2));
    }
    
    // Install all dependencies individually to avoid issues
    for (const dep in packageJson.dependencies) {
      try {
        log(`Installing dependency: ${dep}`, 'info');
        await executeCommand(`npm install ${dep} --no-audit --no-fund --loglevel=error`, { cwd: tempDir });
      } catch (err) {
        log(`Warning: Failed to install ${dep}: ${err.message}`, 'warning');
      }
    }
    
    log('Dependencies installed', 'success');
  } catch (error) {
    log(`Warning: Error installing dependencies: ${error.message}`, 'warning');
  }
}

async function buildExecutable(tempDir, tempEntryFile, projectPath) {
  const packageJsonPath = path.join(tempDir, 'package.json');
  const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, 'utf8'));
  const appName = packageJson.name;
  
  const relativeEntryPath = path.relative(tempDir, tempEntryFile);
  
  const outputPath = path.join(
    fs.statSync(projectPath).isFile() ? path.dirname(projectPath) : projectPath,
    `${appName}.exe`
  );
  
  try {
    // Try using pkg directly from global installation
    try {
      await executeCommand('pkg -v');
    } catch (error) {
      log('pkg not found globally, installing...', 'info');
      await executeCommand('npm install -g pkg');
    }
    
    // Create a simple wrapper for the main file
    const wrapperPath = path.join(tempDir, 'wrapper.js');
    const wrapperContent = `
process.on('uncaughtException', (err) => {
  console.error('Uncaught exception:', err);
  console.log('Press any key to exit...');
  process.stdin.setRawMode(true);
  process.stdin.resume();
  process.stdin.on('data', () => process.exit(1));
});

try {
  require('./${relativeEntryPath}');
} catch (err) {
  console.error('Error loading application:', err);
  console.log('Press any key to exit...');
  process.stdin.setRawMode(true);
  process.stdin.resume();
  process.stdin.on('data', () => process.exit(1));
}
`;
    fs.writeFileSync(wrapperPath, wrapperContent);
    
    // Try building with pkg and wrapper
    try {
      await executeCommand(`pkg "${wrapperPath}" --targets node16-win-x64 --output "${outputPath}"`, { cwd: tempDir });
    } catch (error) {
      log('First packaging attempt failed, trying alternative method...', 'warning');
      
      // Try updating package.json and using pkg with just the directory
      packageJson.bin = 'wrapper.js';
      packageJson.main = 'wrapper.js';
      fs.writeFileSync(packageJsonPath, JSON.stringify(packageJson, null, 2));
      
      await executeCommand(`pkg . --targets node16-win-x64 --output "${outputPath}"`, { cwd: tempDir });
    }
    
    if (fs.existsSync(outputPath)) {
      log(`Executable created at: ${outputPath}`, 'success');
      return outputPath;
    } else {
      throw new Error('Failed to create executable');
    }
  } catch (error) {
    log(`Error during packaging: ${error.message}`, 'error');
    throw error;
  }
}

async function main() {
  let tempDir = null;
  
  try {
    rl.question('Enter the path to your JS file or project directory: ', async (inputPath) => {
      try {
        inputPath = path.resolve(inputPath);
        
        if (!fs.existsSync(inputPath)) {
          throw new Error(`Path does not exist: ${inputPath}`);
        }
        
        log(`Converting ${path.basename(inputPath)} to EXE`, 'title');
        
        const { entryFile, projectDir } = findEntryPoint(inputPath);
        log(`Entry file: ${entryFile}`, 'info');
        log(`Project directory: ${projectDir}`, 'info');
        
        tempDir = createTempDir();
        
        const { tempEntryFile, tempPackageJsonPath } = copyProject(
          fs.statSync(inputPath).isFile() ? path.dirname(inputPath) : inputPath,
          entryFile,
          tempDir
        );
        
        preparePackageJson(tempPackageJsonPath, tempEntryFile);
        
        await installDependencies(tempDir, tempEntryFile);
        
        const outputPath = await buildExecutable(tempDir, tempEntryFile, inputPath);
        
        log('Conversion completed successfully!', 'title');
        log(`Executable is at: ${outputPath}`, 'success');
        
        rl.close();
      } catch (error) {
        log(`Error: ${error.message}`, 'error');
        if (tempDir) {
          log(`Temporary directory: ${tempDir}`, 'info');
        }
        rl.close();
      }
    });
  } catch (error) {
    log(`Fatal error: ${error.message}`, 'error');
    rl.close();
  }
}

main();