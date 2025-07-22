// express-to-exe.js - One-click solution for Express apps
const fs = require('fs');
const path = require('path');
const { spawn } = require('child_process');
const readline = require('readline');

const rl = readline.createInterface({
  input: process.stdin,
  output: process.stdout
});

// Execute command with live output
function executeWithLiveOutput(command, args, options = {}) {
  return new Promise((resolve, reject) => {
    const cmd = spawn(command, args, {
      ...options,
      shell: true,
      stdio: 'inherit' // This shows live output
    });

    cmd.on('close', (code) => {
      if (code === 0) {
        resolve();
      } else {
        reject(new Error(`Command failed with exit code ${code}`));
      }
    });

    cmd.on('error', (err) => {
      reject(err);
    });
  });
}

// Simple one-step conversion for Express apps
async function convertExpressToExe(jsFilePath) {
  try {
    // Get path info
    const fullPath = path.resolve(jsFilePath);
    const dirPath = path.dirname(fullPath);
    const filename = path.basename(fullPath, path.extname(fullPath));
    const outputPath = path.join(dirPath, filename);
    
    console.log(`\n=== Converting ${filename}.js to executable ===`);
    console.log('This may take a few minutes...');
    
    // Create or update package.json with Express-specific settings
    updatePackageJson(jsFilePath);
    
    // Execute pkg with verbose output
    await executeWithLiveOutput('npx', [
      'pkg',
      '.',
      '--target', 'node16-win-x64',
      '--output', outputPath,
      '--debug'
    ], { cwd: dirPath });
    
    console.log(`\n✅ Success! Executable created at: ${outputPath}.exe`);
    return true;
  } catch (error) {
    console.error(`\n❌ Conversion failed: ${error.message}`);
    return false;
  }
}

// Create or update package.json
function updatePackageJson(jsFilePath) {
  const dirPath = path.dirname(path.resolve(jsFilePath));
  const packageJsonPath = path.join(dirPath, 'package.json');
  const mainFile = path.basename(jsFilePath);
  
  let pkg = {};
  
  // Read existing package.json if it exists
  if (fs.existsSync(packageJsonPath)) {
    pkg = JSON.parse(fs.readFileSync(packageJsonPath, 'utf8'));
    console.log('Updating existing package.json...');
  } else {
    console.log('Creating new package.json...');
  }
  
  // Ensure essential fields exist
  pkg.name = pkg.name || path.basename(jsFilePath, path.extname(jsFilePath));
  pkg.version = pkg.version || '1.0.0';
  pkg.main = pkg.main || mainFile;
  
  // Add bin field if not present
  if (!pkg.bin) {
    pkg.bin = mainFile;
  }
  
  // Add Express-specific pkg configuration
  pkg.pkg = {
    assets: [
      "node_modules/**/*",
      "public/**/*",
      "views/**/*"
    ],
    targets: [
      "node16-win-x64"
    ],
    outputPath: "dist"
  };
  
  // Write updated package.json
  fs.writeFileSync(packageJsonPath, JSON.stringify(pkg, null, 2));
  console.log('✅ package.json configured for Express application');
}

// Main function
async function main() {
  rl.question('Enter the path to your Express JS file: ', async (jsFilePath) => {
    // Validate file
    if (!fs.existsSync(jsFilePath)) {
      console.error('Error: File does not exist!');
      rl.close();
      return;
    }
    
    if (path.extname(jsFilePath).toLowerCase() !== '.js') {
      console.error('Error: File must have a .js extension!');
      rl.close();
      return;
    }
    
    // Run conversion
    await convertExpressToExe(jsFilePath);
    rl.close();
  });
}

// Run main function
main();