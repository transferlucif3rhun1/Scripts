import jwt from 'jsonwebtoken';
import dotenv from 'dotenv';
import { IUser } from '../models/User';

dotenv.config();

const JWT_SECRET = process.env.JWT_SECRET || 'your_jwt_secret_key';

export const signToken = (user: IUser): string => {
  const payload = {
    id: user._id,
    isAdmin: user.isAdmin,
    isVendor: user.isVendor
  };
  return jwt.sign(payload, JWT_SECRET, { expiresIn: '1d' });
};
