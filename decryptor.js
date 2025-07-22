const express = require('express');
const crypto = require('crypto');
const bodyParser = require('body-parser');

const app = express();
const port = 3001; // Using a different port from the encryption service

// Middleware to parse JSON requests
app.use(bodyParser.json());

// AES decryption function
function aesDecrypt(key, iv, encryptedData, mode = 'cbc', padding = 'pkcs7') {
  // Convert key and IV from base64 to buffer
  const keyBuffer = Buffer.from(key, 'base64');
  const ivBuffer = Buffer.from(iv, 'base64');
  
  // Determine the cipher based on key length and mode
  const cipherAlgorithm = `aes-${keyBuffer.length * 8}-${mode.toLowerCase()}`;
  
  // Create decipher
  const decipher = crypto.createDecipheriv(cipherAlgorithm, keyBuffer, ivBuffer);
  
  // Set padding if applicable
  if (padding.toLowerCase() === 'pkcs7') {
    decipher.setAutoPadding(true);
  } else {
    decipher.setAutoPadding(false);
  }
  
  // Decrypt data
  let decrypted = decipher.update(encryptedData, 'base64', 'utf8');
  decrypted += decipher.final('utf8');
  
  return decrypted;
}

// Convert hex to base64
function hexToBase64(hexStr) {
  const buffer = Buffer.from(hexStr, 'hex');
  return buffer.toString('base64');
}

// Decryption endpoint
app.post('/decrypt', (req, res) => {
  try {
    const { encryptedHex } = req.body;
    
    if (!encryptedHex) {
      return res.status(400).json({ error: 'Encrypted hex string is required' });
    }
    
    // Key and IV values (same as in the encryption service)
    const key = 'YU5kUmZValhuMnI1dTh4L0E/RChHK0tiUGVTaFZrWXA=';
    const iv = 'AAAAAAAAAAAAAAAAAAAAAA==';
    
    // Convert hex to base64
    const encryptedBase64 = hexToBase64(encryptedHex);
    
    // Decrypt the data
    const decryptedJson = aesDecrypt(key, iv, encryptedBase64, 'CBC', 'PKCS7');
    
    // Parse the JSON
    const decryptedData = JSON.parse(decryptedJson);
    
    // Return the result
    res.json({ 
      success: true, 
      decryptedData: decryptedData 
    });
    
  } catch (error) {
    console.error('Decryption error:', error);
    res.status(500).json({ 
      error: 'An error occurred during decryption',
      details: error.message 
    });
  }
});

// Utility endpoint to decrypt and show data
app.get('/test-decrypt', (req, res) => {
  res.send(`
    <!DOCTYPE html>
    <html>
    <head>
      <title>Decryption Tester</title>
      <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
        textarea { width: 100%; height: 100px; margin-bottom: 10px; }
        button { padding: 10px; background: #4CAF50; color: white; border: none; cursor: pointer; }
        pre { background: #f4f4f4; padding: 10px; border-radius: 5px; }
      </style>
    </head>
    <body>
      <h1>Decrypt AES Encrypted Data</h1>
      <p>Enter the hex string from the encryption service:</p>
      <textarea id="encryptedHex" placeholder="Enter encrypted hex string..."></textarea>
      <button onclick="decryptData()">Decrypt</button>
      <h2>Results:</h2>
      <pre id="result">Results will appear here...</pre>
      
      <script>
        async function decryptData() {
          const encryptedHex = document.getElementById('encryptedHex').value.trim();
          
          if (!encryptedHex) {
            alert('Please enter an encrypted hex string');
            return;
          }
          
          try {
            const response = await fetch('/decrypt', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ encryptedHex })
            });
            
            const data = await response.json();
            
            if (data.success) {
              document.getElementById('result').textContent = 
                JSON.stringify(data.decryptedData, null, 2);
            } else {
              document.getElementById('result').textContent = 
                'Error: ' + data.error;
            }
          } catch (error) {
            document.getElementById('result').textContent = 
              'Error: ' + error.message;
          }
        }
      </script>
    </body>
    </html>
  `);
});

// Start the server
app.listen(port, () => {
  console.log(`Decryption server running at http://localhost:${port}`);
  console.log(`Test interface available at http://localhost:${port}/test-decrypt`);
});