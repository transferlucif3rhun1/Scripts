import { UserModel, IUser } from '../../models/User';
import argon2 from 'argon2';
import { AuthenticationError } from 'apollo-server-express';
import { signToken } from '../../utils/tokenHelpers';
import { CurrentUser } from '../../middleware/authMiddleware';

interface RegisterArgs {
  email: string;
  password: string;
  isAdmin?: boolean;
  isVendor?: boolean;
  phone?: string;
}

interface LoginArgs {
  email: string;
  password: string;
}

interface UpdateUserArgs {
  phone?: string;
}

export const userResolver = {
  Query: {
    me: async (_parent: any, _args: any, context: { currentUser?: CurrentUser }) => {
      if (!context.currentUser) {
        throw new AuthenticationError('Not authenticated');
      }
      const user = await UserModel.findById(context.currentUser.id);
      return user;
    },
  },
  Mutation: {
    register: async (_parent: any, args: RegisterArgs) => {
      const { email, password, isAdmin, isVendor, phone } = args;
      const existingUser = await UserModel.findOne({ email });
      if (existingUser) {
        throw new Error('User already exists');
      }

      const hashedPassword = await argon2.hash(password);
      const user = new UserModel({
        email,
        password: hashedPassword,
        isAdmin: !!isAdmin,
        isVendor: !!isVendor,
        phone
      });
      await user.save();

      const token = signToken(user);
      return { token, user };
    },

    login: async (_parent: any, args: LoginArgs) => {
      const { email, password } = args;
      const user = await UserModel.findOne({ email });
      if (!user) {
        throw new AuthenticationError('Invalid credentials');
      }

      const valid = await argon2.verify(user.password, password);
      if (!valid) {
        throw new AuthenticationError('Invalid credentials');
      }

      const token = signToken(user);
      return { token, user };
    },

    updateUser: async (_parent: any, args: UpdateUserArgs, context: { currentUser?: CurrentUser }) => {
      if (!context.currentUser) {
        throw new AuthenticationError('Not authenticated');
      }
      const user = await UserModel.findById(context.currentUser.id);
      if (!user) throw new Error('User not found');

      if (args.phone) {
        user.phone = args.phone;
      }
      await user.save();

      return user;
    },
  },
};
