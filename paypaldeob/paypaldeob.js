const express = require('express');
const deobfuscate = require('./deobfuscator.js');
const app = express();

app.use(express.json({ limit: '1mb' }));

app.post('/deobfuscate', async (req, res) => {
  const { script } = req.body;
  if (!script) return res.status(400).json({ error: 'Missing script in request body' });

  let code;
  try {
    code = Buffer.from(script, 'base64').toString('utf8');
  } catch {
    return res.status(400).json({ error: 'Invalid base64 script' });
  }

  let deobfuscated;
  try {
    // support both sync and async deobfuscators
    deobfuscated = await Promise.resolve(deobfuscate(code));
  } catch (err) {
    console.error(err);
    if (err.name === 'SyntaxError') {
      return res.status(422).json({ error: 'Syntax error during deobfuscation' });
    }
    return res.status(500).json({ error: 'Failed to deobfuscate script' });
  }

  const csrfMatch = /encodeURIComponent\(\s*["']([^"']+)["']\s*\)\s*\+\s*["']&refTimestamp/.exec(deobfuscated);
  if (!csrfMatch) return res.status(404).json({ error: 'CSRF token not found' });

  return res.json({ csrf: csrfMatch[1] });
});

const port = process.env.PORT || 3000;
app.listen(port, () => console.log(`Server listening on port ${port}`));