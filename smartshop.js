const crypto = require('crypto');

// RSA public key as a string
const publicKey = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQCRn+iPY/ENsTQpLsyIDPK/HRzv
irt81Wc8Nl9Iv/Vt10hSsefW98j1vo0RaBOYUYpVeSaM13C/r0LqSFkF/gC6t5vr
U3bJ6vLfLg9IDx33h+G5aT78ZHyVdj1VBiJBIQxmd9tV+xphm1dQsptZEzJ2t/0Y
7U7BSRu35ERVxi+HzwIDAQAB
-----END PUBLIC KEY-----`;

// Function to generate current timestamp in the format YYYY-MM-DD HH:mm:ss
function getCurrentTimestamp() {
    const now = new Date();
    const year = now.getFullYear();
    const month = String(now.getMonth() + 1).padStart(2, '0'); // Months are 0-based
    const day = String(now.getDate()).padStart(2, '0');
    const hours = String(now.getHours()).padStart(2, '0');
    const minutes = String(now.getMinutes()).padStart(2, '0');
    const seconds = String(now.getSeconds()).padStart(2, '0');
    return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`;
}

// Function to generate a random GUID
function generateGUID() {
    return crypto.randomUUID(); // Generates a random UUID (GUID)
}

// Generate random string
const prefix = "ss_android_mobile_1k";
const timestamp = getCurrentTimestamp();
const guid = generateGUID();
const randomString = `${prefix}#${timestamp}#${guid}`;

console.log('Random String:', randomString);

try {
    // Encrypt the random string using the public key with RSA/ECB/PKCS1Padding
    const encryptedBuffer = crypto.publicEncrypt(
        {
            key: publicKey,
            padding: crypto.constants.RSA_PKCS1_PADDING
        },
        Buffer.from(randomString, 'utf-8')
    );

    // Convert the encrypted data to Base64
    const encryptedBase64 = encryptedBuffer.toString('base64');
    console.log('Encrypted (Base64):', encryptedBase64);

    // Combine prefix and encrypted text and Base64 encode the final result
    const finalText = `${prefix}:${encryptedBase64}`;
    const finalBase64 = Buffer.from(finalText).toString('base64');

    console.log('Final Base64:', finalBase64);
} catch (error) {
    console.error('Error encrypting:', error.message);
}

function generateSerial() {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
  let serial = '';

  // Generate a 12-character alphanumeric string
  for (let i = 0; i < 12; i++) {
    serial += chars.charAt(Math.floor(Math.random() * chars.length));
  }

  return serial;
}

console.log('X-Device-Id:', generateSerial());