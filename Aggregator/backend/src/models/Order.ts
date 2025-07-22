import mongoose, { Schema, Document } from 'mongoose';

export interface IOrderItem {
  productId: string;
  quantity: number;
}

export interface IOrder extends Document {
  userId: string;
  restaurantId: string;
  items: IOrderItem[];
  paymentMethod: 'COD' | 'ONLINE';
  status: 'PLACED' | 'PREPARING' | 'DELIVERED' | 'CANCELLED';
  createdAt?: Date;
  updatedAt?: Date;
}

const OrderItemSchema = new Schema<IOrderItem>(
  {
    productId: { type: String, required: true },
    quantity: { type: Number, required: true },
  },
  { _id: false }
);

const OrderSchema = new Schema<IOrder>(
  {
    userId: { type: String, required: true },
    restaurantId: { type: String, required: true },
    items: { type: [OrderItemSchema], required: true },
    paymentMethod: { type: String, enum: ['COD', 'ONLINE'], default: 'COD' },
    status: { type: String, enum: ['PLACED', 'PREPARING', 'DELIVERED', 'CANCELLED'], default: 'PLACED' },
  },
  { timestamps: true }
);

export const OrderModel = mongoose.model<IOrder>('Order', OrderSchema);
