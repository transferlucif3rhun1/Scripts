const pkg = require('obfuscator-io-deobfuscator');
const nativeDeobfuscate = pkg.default?.deobfuscate || pkg.deobfuscate || pkg.default || pkg;
if (typeof nativeDeobfuscate !== 'function') {
  console.error('Failed to load deobfuscate function from obfuscator-io-deobfuscator');
  process.exit(1);
}
function deobfuscate(code, options) {
  try {
    return nativeDeobfuscate(code);
  } catch {
    return nativeDeobfuscate(code, options || {});
  }
}
module.exports = deobfuscate;