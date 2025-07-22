import { typeDefs } from './schema';
import { userResolver } from './resolvers/userResolver';
import { restaurantResolver } from './resolvers/restaurantResolver';
import { orderResolver } from './resolvers/orderResolver';
import { reviewResolver } from './resolvers/reviewResolver';
import { errorReportResolver } from './resolvers/errorReportResolver';

export const resolvers = {
  Query: {
    ...userResolver.Query,
    ...restaurantResolver.Query,
    ...orderResolver.Query,
    ...reviewResolver.Query,
    ...errorReportResolver.Query,
  },
  Mutation: {
    ...userResolver.Mutation,
    ...restaurantResolver.Mutation,
    ...orderResolver.Mutation,
    ...reviewResolver.Mutation,
    ...errorReportResolver.Mutation,
  },
};

export { typeDefs };
