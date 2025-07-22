import { userResolvers } from './userResolver';
import { restaurantResolvers } from './restaurantResolver';
import { orderResolvers } from './orderResolver';
import { reviewResolvers } from './reviewResolver';
import { errorReportResolvers } from './errorReportResolver';

export const resolvers = {
  Query: {
    ...userResolvers.Query,
    ...restaurantResolvers.Query,
    ...orderResolvers.Query,
    ...reviewResolvers.Query,
    ...errorReportResolvers.Query,
  },
  Mutation: {
    ...userResolvers.Mutation,
    ...restaurantResolvers.Mutation,
    ...orderResolvers.Mutation,
    ...reviewResolvers.Mutation,
    ...errorReportResolvers.Mutation,
  },
};
