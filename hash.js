const cluster = require('cluster');
const os = require('os');
const express = require('express');
const crypto = require('crypto');
const { body, validationResult } = require('express-validator');

if (cluster.isMaster) {
  const numCPUs = os.cpus().length;
  console.log(`Master ${process.pid} is running`);
  console.log(`Forking ${numCPUs} workers`);

  // Fork workers equal to the number of CPU cores
  for (let i = 0; i < numCPUs; i++) {
    cluster.fork();
  }

  // Restart a worker if it dies
  cluster.on('exit', (worker, code, signal) => {
    console.log(`Worker ${worker.process.pid} died with code ${code}, signal ${signal}. Forking a new worker.`);
    cluster.fork();
  });
} else {
  const app = express();

  // Middleware to parse JSON bodies (for /count endpoint)
  app.use(express.json());
  // Middleware to parse URL-encoded bodies (for /hash endpoint)
  app.use(express.urlencoded({ extended: true }));

  // Utility function to generate a GUID
  function makeGuid() {
    return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
      const r = Math.random() * 16 | 0;
      const v = c === 'x' ? r : (r & 0x3 | 0x8);
      return v.toString(16);
    });
  }

  // Utility function to generate a hash using PBKDF2
  function generateHash(email, password, iterations) {
    const pass = Buffer.from(password);
    const salt = Buffer.from(email);

    const inkey = crypto.pbkdf2Sync(pass, salt, iterations, 32, 'sha256');
    const key = crypto.pbkdf2Sync(inkey, pass, 1, 32, 'sha256');

    return key.toString('base64');
  }

  // GET /guid endpoint: Returns a newly generated GUID
  app.get('/guid', (req, res) => {
    try {
      const guid = makeGuid();
      res.json({ guid });
    } catch (error) {
      console.error(`Error in /guid: ${error.message}`);
      res.status(500).json({ error: 'Internal server error' });
    }
  });

  // POST /hash endpoint: Generates a hash from email, password, and iterations
  app.post('/hash', [
    body('email').isEmail().withMessage('Invalid email format'),
    body('password').isString().isLength({ min: 1 }).withMessage('Password is required'),
    body('kdfIterations').isInt({ min: 1 }).withMessage('kdfIterations must be a positive integer')
  ], (req, res) => {
    const errors = validationResult(req);
    if (!errors.isEmpty()) {
      return res.status(400).json({ errors: errors.array() });
    }

    const { email, password, kdfIterations } = req.body;
    const iterations = parseInt(kdfIterations);

    try {
      const hash = generateHash(email, password, iterations);
      res.json({ hash });
    } catch (error) {
      console.error(`Error in /hash: ${error.message}`);
      res.status(500).json({ error: 'Internal server error' });
    }
  });

  // POST /count endpoint: Counts non-null cipher fields and returns the counts
  app.post('/count', (req, res) => {
    try {
      // Extract ciphers from the request body
      const ciphers = req.body.ciphers;

      // Validate that ciphers is an array
      if (!Array.isArray(ciphers)) {
        return res.status(400).json({ error: 'Invalid input: ciphers must be an array' });
      }

      // Initialize counts for each field
      const counts = {
        login: 0,
        card: 0,
        identity: 0,
        secureNote: 0,
        sshKey: 0
      };

      // Iterate over each cipher and increment counts for non-null fields
      ciphers.forEach(cipher => {
        if (cipher.login !== null) counts.login++;
        if (cipher.card !== null) counts.card++;
        if (cipher.identity !== null) counts.identity++;
        if (cipher.secureNote !== null) counts.secureNote++;
        if (cipher.sshKey !== null) counts.sshKey++;
      });

      // Respond with the counts in JSON format
      res.json(counts);
    } catch (error) {
      console.error(`Error in /count: ${error.message}`);
      res.status(500).json({ error: 'Internal server error' });
    }
  });

  // Start the server
  const port = 3090;
  app.listen(port, () => {
    console.log(`Worker ${process.pid} is running on port ${port}`);
  });
}