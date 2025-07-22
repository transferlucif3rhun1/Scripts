const CryptoJS = require('crypto-js');

// 3DES Decryption function
function decrypt3DES(encryptedData) {
    const key = CryptoJS.enc.Utf8.parse("E7wQ#@%wfXfdAnQMT%@77vMu");
    const iv = CryptoJS.enc.Utf8.parse("3C@L4Xx!");
    
    try {
        const decrypted = CryptoJS.TripleDES.decrypt(encryptedData, key, {
            mode: CryptoJS.mode.CBC,
            padding: CryptoJS.pad.Pkcs7,
            iv: iv
        });
        
        return decrypted.toString(CryptoJS.enc.Utf8);
    } catch (error) {
        throw new Error(`Decryption failed: ${error.message}`);
    }
}

// MD5 function for signature verification
function createMD5Hash(input) {
    return CryptoJS.MD5(input).toString();
}

// Verify the signature
function verifySignature(payload) {
    const { sign, timeStamp, reqData } = payload;
    
    // Recreate the signature string
    const signatureString = "1000001" + timeStamp + JSON.stringify(reqData);
    const expectedSignature = createMD5Hash(signatureString);
    
    return sign === expectedSignature;
}

// Main decryption function
function decryptPayload(encryptedData) {
    try {
        // Decrypt the 3DES encrypted data
        const decryptedJson = decrypt3DES(encryptedData);
        
        // Parse the JSON
        const payload = JSON.parse(decryptedJson);
        
        // Verify signature
        const isValidSignature = verifySignature(payload);
        
        return {
            success: true,
            payload: payload,
            isValidSignature: isValidSignature,
            originalData: payload.reqData
        };
    } catch (error) {
        return {
            success: false,
            error: error.message,
            payload: null,
            isValidSignature: false,
            originalData: null
        };
    }
}

// Extract login credentials from decrypted payload
function extractLoginData(encryptedData) {
    const result = decryptPayload(encryptedData);
    
    if (!result.success) {
        return {
            success: false,
            error: result.error
        };
    }
    
    const { payload, isValidSignature, originalData } = result;
    
    // Extract relevant login information
    const loginInfo = {
        success: true,
        isValidSignature: isValidSignature,
        timestamp: payload.timeStamp,
        deviceInfo: {
            device: payload.device,
            ip: payload.ip,
            appId: payload.appId,
            platform: payload.platform,
            version: payload.Version
        },
        credentials: originalData
    };
    
    return loginInfo;
}

// Utility function to check if timestamp is within acceptable range
function isTimestampValid(timestamp, maxAgeSeconds = 300) {
    const currentTimestamp = Math.floor(Date.now() / 1000);
    const age = currentTimestamp - timestamp;
    
    return age >= 0 && age <= maxAgeSeconds;
}

// Complete validation function
function validateAndDecrypt(encryptedData, options = {}) {
    const { maxAgeSeconds = 300, requireValidSignature = true } = options;
    
    const result = decryptPayload(encryptedData);
    
    if (!result.success) {
        return {
            valid: false,
            error: result.error,
            data: null
        };
    }
    
    const { payload, isValidSignature } = result;
    
    // Check signature if required
    if (requireValidSignature && !isValidSignature) {
        return {
            valid: false,
            error: "Invalid signature",
            data: null
        };
    }
    
    // Check timestamp validity
    if (!isTimestampValid(payload.timeStamp, maxAgeSeconds)) {
        return {
            valid: false,
            error: "Timestamp expired or invalid",
            data: null
        };
    }
    
    return {
        valid: true,
        error: null,
        data: {
            credentials: payload.reqData,
            metadata: {
                timestamp: payload.timeStamp,
                device: payload.device,
                ip: payload.ip,
                platform: payload.platform,
                signature: payload.sign,
                isValidSignature: isValidSignature
            }
        }
    };
}
const data=decrypt3DES("hpaB47VL6nbhaX5P9tXkihrM2GcptFbQ9BRKExPzxJH9uheAKbKpwye6mT9fBBFM3CXUvKFvTpVZ5Aq4JEMTBA==");
console.log(data);
module.exports = {
    decrypt3DES,
    decryptPayload,
    extractLoginData,
    validateAndDecrypt,
    verifySignature,
    isTimestampValid,
    createMD5Hash
};

// Example usage:
/*
// Decrypt an encrypted payload
const encryptedData = "your_encrypted_data_here";

// Basic decryption
const decrypted = decryptPayload(encryptedData);
console.log("Decrypted:", decrypted);

// Extract login data
const loginData = extractLoginData(encryptedData);
console.log("Login Data:", loginData);

// Complete validation
const validation = validateAndDecrypt(encryptedData, {
    maxAgeSeconds: 300,  // 5 minutes
    requireValidSignature: true
});

if (validation.valid) {
    console.log("Valid login attempt:");
    console.log("Username:", validation.data.credentials.username);
    console.log("Password:", validation.data.credentials.password);
    console.log("Device:", validation.data.metadata.device);
} else {
    console.log("Invalid login attempt:", validation.error);
}
*/