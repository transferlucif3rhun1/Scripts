const express = require('express');
const crypto = require('crypto');
const bodyParser = require('body-parser');

const app = express();
const port = 3000;

// Middleware to parse JSON requests
app.use(bodyParser.json());

// AES encryption function
function aesEncrypt(key, iv, data, mode = 'cbc', padding = 'pkcs7') {
  // Convert key and IV from base64 to buffer
  const keyBuffer = Buffer.from(key, 'base64');
  const ivBuffer = Buffer.from(iv, 'base64');
  
  // Determine the cipher based on key length and mode
  const cipherAlgorithm = `aes-${keyBuffer.length * 8}-${mode.toLowerCase()}`;
  
  // Create cipher
  const cipher = crypto.createCipheriv(cipherAlgorithm, keyBuffer, ivBuffer);
  
  // Set padding if applicable
  if (padding.toLowerCase() === 'pkcs7') {
    cipher.setAutoPadding(true);
  } else {
    cipher.setAutoPadding(false);
  }
  
  // Encrypt data
  let encrypted = cipher.update(data, 'utf8', 'base64');
  encrypted += cipher.final('base64');
  
  return encrypted;
}

// Convert base64 to hex and lowercase
function base64ToHexLowercase(base64Str) {
  const buffer = Buffer.from(base64Str, 'base64');
  return buffer.toString('hex').toLowerCase();
}

// Encryption endpoint
app.post('/encrypt', (req, res) => {
  try {
    const { username, password } = req.body;
    
    if (!username || !password) {
      return res.status(400).json({ error: 'Username and password are required' });
    }
    
    // Key and IV values from the example
    const key = 'YU5kUmZValhuMnI1dTh4L0E/RChHK0tiUGVTaFZrWXA=';
    const iv = 'AAAAAAAAAAAAAAAAAAAAAA==';
    
    // Create the JSON payload with the provided username and password
    const data = JSON.stringify({
        phoneNumber: username,
      password: password,
      brandId: 31
    });
    
    // Encrypt the data
    const encryptedBase64 = aesEncrypt(key, iv, data, 'CBC', 'PKCS7');
    
    // Convert to hex and lowercase
    const encryptedHex = base64ToHexLowercase(encryptedBase64);
    
    // Return the result
    res.json({ 
      success: true, 
      loginInfo: encryptedHex 
    });
    
  } catch (error) {
    console.error('Encryption error:', error);
    res.status(500).json({ 
      error: 'An error occurred during encryption',
      details: error.message 
    });
  }
});

// Start the server
app.listen(port, () => {
  console.log(`Encryption server running at http://localhost:${port}`);
});