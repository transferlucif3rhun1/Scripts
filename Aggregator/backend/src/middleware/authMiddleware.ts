import { Request, Response, NextFunction } from 'express';
import jwt, { JwtPayload } from 'jsonwebtoken';
import dotenv from 'dotenv';

dotenv.config();

const JWT_SECRET = process.env.JWT_SECRET || 'your_jwt_secret_key';

export interface CurrentUser {
  id: string;
  isAdmin: boolean;
  isVendor: boolean;
}

declare module 'express-serve-static-core' {
  interface Request {
    currentUser?: CurrentUser;
  }
}

export const authMiddleware = (req: Request, _res: Response, next: NextFunction): void => {
  const authHeader = req.headers.authorization;

  if (authHeader) {
    const token = authHeader.replace('Bearer ', '');
    try {
      const decoded = jwt.verify(token, JWT_SECRET) as JwtPayload & CurrentUser;
      req.currentUser = {
        id: decoded.id,
        isAdmin: !!decoded.isAdmin,
        isVendor: !!decoded.isVendor,
      };
    } catch (error) {
      // Token invalid or expired - proceed without setting currentUser
    }
  }
  next();
};
