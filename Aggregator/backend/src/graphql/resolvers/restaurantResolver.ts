import { RestaurantModel } from '../../models/Restaurant';
import { AuthenticationError } from 'apollo-server-express';
import { CurrentUser } from '../../middleware/authMiddleware';

interface CreateRestaurantArgs {
  name: string;
  address: string;
}

export const restaurantResolver = {
  Query: {
    getRestaurant: async (_parent: any, { id }: { id: string }) => {
      return await RestaurantModel.findById(id);
    },
    getRestaurants: async () => {
      return await RestaurantModel.find({});
    },
  },
  Mutation: {
    createRestaurant: async (_parent: any, args: CreateRestaurantArgs, context: { currentUser?: CurrentUser }) => {
      if (!context.currentUser || !context.currentUser.isAdmin) {
        throw new AuthenticationError('Admin privileges required to create restaurants');
      }
      const { name, address } = args;
      const restaurant = new RestaurantModel({ name, address });
      return await restaurant.save();
    },
  },
};
