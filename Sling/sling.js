// server.js

// Required Modules
const express = require('express');
const bodyParser = require('body-parser');
const crypto = require('crypto');

// Initialize Express App
const app = express();

// Middleware to parse JSON bodies
app.use(bodyParser.json());

// Configuration Variables
const OAUTH_CONSUMER_KEY = 'Owa77FUtwJZcKau11Vttf4FoU7qnc6HGRCYAUkKH';
const HMAC_KEY = 'BALomd2e4p4A7xKXjQxq94qwnER5FEMEQlwHxHp3&';
const OAUTH_SIGNATURE_METHOD = 'HMAC-SHA1';
const OAUTH_VERSION = '1.0';
const BASE_URL = 'https://ums.p.sling.com/v3/xauth/access_token.json';

// Function to get current Unix time in seconds
function getCurrentUnixTime() {
    return Math.floor(Date.now() / 1000);
}

// Function to generate a random hexadecimal string of given length
function generateRandomHex(length) {
    return crypto.randomBytes(Math.ceil(length / 2))
                 .toString('hex') // Convert to hexadecimal format
                 .slice(0, length); // Return required number of characters
}

// Function to generate device GUID following the UUID v4 pattern
function generateDeviceGUID() {
    // UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
    // where y is one of [8, 9, A, B]
    let guid = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
        let r = crypto.randomBytes(1)[0] % 16;
        let v = (c === 'x') ? r : (r & 0x3 | 0x8);
        return v.toString(16);
    });
    return guid;
}

// Function to URL encode a string
function urlEncode(str) {
    return encodeURIComponent(str);
}

// Function to generate HMAC-SHA1 signature and return Base64 encoded string
function generateHMACSHA1Signature(key, message) {
    return crypto.createHmac('sha1', key)
                 .update(message)
                 .digest('base64');
}

// Route: POST /sling/login
app.post('/sling/login', (req, res) => {
    try {
        const { email, password } = req.body;

        // Validate Input
        if (!email || !password) {
            return res.status(400).json({ error: 'Email and password are required.' });
        }

        // Generate Variables

        // 1. Current Unix Time
        const oauth_timestamp = getCurrentUnixTime();

        // 2. OAuth Nonce (32 hex characters)
        const oauth_nonce = generateRandomHex(32);

        // 3. Device GUID
        const device_guid = generateDeviceGUID();

        // 4. URL Encode Email
        const encoded_email = urlEncode(email);

        // 5. URL Encode Password
        const encoded_password = urlEncode(password);

        // 6. Generate OAuth Signature

        // Construct the base string for signature
        const baseString = `PUT&${urlEncode(BASE_URL)}&` +
            `device_guid%3D${device_guid}%26` +
            `email%3D${encoded_email}%26` +
            `oauth_consumer_key%3D${OAUTH_CONSUMER_KEY}%26` +
            `oauth_nonce%3D${oauth_nonce}%26` +
            `oauth_signature_method%3D${OAUTH_SIGNATURE_METHOD}%26` +
            `oauth_timestamp%3D${oauth_timestamp}%26` +
            `oauth_version%3D${OAUTH_VERSION}%26` +
            `password%3D${encoded_password}`;

        // Generate HMAC-SHA1 Signature and encode it in Base64
        let oauth_signature = generateHMACSHA1Signature(HMAC_KEY, baseString);

        // 7. URL Encode the OAuth Signature
        oauth_signature = urlEncode(oauth_signature);

        // Prepare Response
        const response = {
            oauth_timestamp: oauth_timestamp,
            oauth_nonce: oauth_nonce,
            device_guid: device_guid,
            email: encoded_email,
            password: encoded_password,
            oauth_signature: oauth_signature
        };

        // Send Response
        return res.status(200).json(response);

    } catch (error) {
        console.error('Error processing /sling/login:', error);
        return res.status(500).json({ error: 'Internal Server Error' });
    }
});

// Root Route (Optional)
app.get('/', (req, res) => {
    res.send('Sling OAuth API is running.');
});

// Start the Server
const PORT = process.env.PORT || 8080;
app.listen(PORT, () => {
    console.log(`Sling OAuth API server is running on port ${PORT}`);
});
