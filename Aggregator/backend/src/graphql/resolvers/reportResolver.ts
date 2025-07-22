const Order = require("../../models/Order");

module.exports = {
  Query: {
    getSalesReport: async (_, { startDate, endDate }, { currentUser }) => {
      // Only admin or vendor can see sales
      if (!currentUser || (!currentUser.isAdmin && !currentUser.isVendor)) {
        throw new Error("Not authorized to view sales");
      }

      const start = new Date(startDate);
      const end = new Date(endDate);

      const orders = await Order.find({
        createdAt: { $gte: start, $lte: end },
      }).exec();

      const totalOrders = orders.length;
      const totalRevenue = orders.reduce((sum, order) => {
        const orderTotal = order.items.reduce((acc, item) => {
          return acc + item.price * item.quantity;
        }, 0);
        return sum + orderTotal;
      }, 0);

      return { totalOrders, totalRevenue };
    },
  },
};
