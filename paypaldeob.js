const express = require('express');
const deobfuscator = require('./deobfuscator.js');
const app = express();
app.use(express.json({ limit: '1mb' }));
app.post('/deobfuscate', (req, res) => {
  try {
    const { script } = req.body;
    if (!script) return res.status(400).json({ error: 'Missing script in request body' });
    let code;
    try {
      code = Buffer.from(script, 'base64').toString('utf8');
    } catch {
      return res.status(400).json({ error: 'Invalid base64 script' });
    }
    const deobfuscated = deobfuscator.deobfuscate(code);
    const match = /csrf(?:Token)?\s*[:=]\s*["']([^"']+)["']/i.exec(deobfuscated);
    if (!match) return res.status(404).json({ error: 'CSRF token not found' });
    return res.json({ csrf: match[1] });
  } catch (err) {
    console.error(err);
    return res.status(500).json({ error: 'Internal server error' });
  }
});
const port = process.env.PORT || 3000;
app.listen(port, () => {
  console.log(`Server listening on port ${port}`);
});
