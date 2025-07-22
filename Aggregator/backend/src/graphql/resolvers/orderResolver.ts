import { OrderModel } from '../../models/Order';
import { AuthenticationError } from 'apollo-server-express';
import { CurrentUser } from '../../middleware/authMiddleware';

interface CreateOrderArgs {
  restaurantId: string;
  items: { productId: string; quantity: number }[];
  paymentMethod: 'COD' | 'ONLINE';
}

interface UpdateOrderStatusArgs {
  orderId: string;
  status: 'PLACED' | 'PREPARING' | 'DELIVERED' | 'CANCELLED';
}

export const orderResolver = {
  Query: {
    getOrder: async (_parent: any, { id }: { id: string }, context: { currentUser?: CurrentUser }) => {
      if (!context.currentUser) {
        throw new AuthenticationError('Not authenticated');
      }
      return await OrderModel.findById(id);
    },
    getOrders: async (_parent: any, _args: any, context: { currentUser?: CurrentUser }) => {
      if (!context.currentUser) {
        throw new AuthenticationError('Not authenticated');
      }
      // Admin sees all orders, normal users see only their own
      if (context.currentUser.isAdmin) {
        return await OrderModel.find({});
      }
      return await OrderModel.find({ userId: context.currentUser.id });
    },
  },
  Mutation: {
    createOrder: async (_parent: any, args: CreateOrderArgs, context: { currentUser?: CurrentUser; io?: any }) => {
      if (!context.currentUser) {
        throw new AuthenticationError('Not authenticated');
      }
      const newOrder = new OrderModel({
        userId: context.currentUser.id,
        restaurantId: args.restaurantId,
        items: args.items,
        paymentMethod: args.paymentMethod,
        status: 'PLACED'
      });
      const savedOrder = await newOrder.save();

      // Example real-time event
      if (context.io) {
        context.io.emit('NEW_ORDER', { orderId: savedOrder._id });
      }

      return savedOrder;
    },

    updateOrderStatus: async (_parent: any, args: UpdateOrderStatusArgs, context: { currentUser?: CurrentUser; io?: any }) => {
      if (!context.currentUser) {
        throw new AuthenticationError('Not authenticated');
      }
      // Vendors or Admins can update status
      if (!context.currentUser.isVendor && !context.currentUser.isAdmin) {
        throw new AuthenticationError('Not authorized to update order status');
      }
      const order = await OrderModel.findById(args.orderId);
      if (!order) {
        throw new Error('Order not found');
      }
      order.status = args.status;
      await order.save();

      if (context.io) {
        context.io.emit('ORDER_STATUS_UPDATED', { orderId: order._id, status: order.status });
      }

      return order;
    },
  },
};
