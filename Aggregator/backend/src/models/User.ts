import mongoose, { Schema, Document } from 'mongoose';
import { fieldEncryption } from 'mongoose-field-encryption'; // <-- Use the named import
import { encryptionKey } from '../config/encryptionConfig';

export interface IUser extends Document {
  email: string;
  password: string;
  isAdmin?: boolean;
  isVendor?: boolean;
  phone?: string; // Field-level encrypted
}

const UserSchema = new Schema<IUser>(
  {
    email: { type: String, required: true, unique: true },
    password: { type: String, required: true },
    isAdmin: { type: Boolean, default: false },
    isVendor: { type: Boolean, default: false },
    phone: { type: String },
  },
  { timestamps: true }
);

// Correct usage of fieldEncryption
UserSchema.plugin(fieldEncryption, {
  fields: ['phone'],
  secret: encryptionKey,
  // Optional: define custom salt generator
  saltGenerator: (secret: string) => secret.slice(0, 16),
});

export const UserModel = mongoose.model<IUser>('User', UserSchema);
