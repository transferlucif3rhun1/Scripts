import dotenv from 'dotenv';

dotenv.config();

export const encryptionKey = process.env.ENCRYPTION_SECRET || 'fallback_encryption_key';
