import { ReviewModel } from '../../models/Review';
import { OrderModel } from '../../models/Order';
import { AuthenticationError } from 'apollo-server-express';
import { CurrentUser } from '../../middleware/authMiddleware';

interface SubmitReviewArgs {
  orderId: string;
  rating: number;
  comment?: string;
}

export const reviewResolver = {
  Query: {
    getReviewsForRestaurant: async (_parent: any, { restaurantId }: { restaurantId: string }) => {
      return await ReviewModel.find({ restaurantId });
    },
  },
  Mutation: {
    submitReview: async (_parent: any, args: SubmitReviewArgs, context: { currentUser?: CurrentUser }) => {
      if (!context.currentUser) {
        throw new AuthenticationError('Not authenticated');
      }
      const order = await OrderModel.findById(args.orderId);
      if (!order) {
        throw new Error('Order not found');
      }
      // Only the user who owns the order can review
      if (order.userId !== context.currentUser.id) {
        throw new AuthenticationError('Cannot review an order you did not place');
      }
      // Must be delivered
      if (order.status !== 'DELIVERED') {
        throw new Error('Cannot review an order that is not delivered');
      }

      const review = new ReviewModel({
        userId: context.currentUser.id,
        orderId: args.orderId,
        restaurantId: order.restaurantId,
        rating: args.rating,
        comment: args.comment,
      });
      return await review.save();
    },
  },
};
