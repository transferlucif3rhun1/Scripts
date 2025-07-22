import { Prop, Schema, SchemaFactory } from '@nestjs/mongoose';
import { Document } from 'mongoose';

@Schema({ timestamps: true })
export class Order {
  @Prop({ required: true, ref: 'User' })
  userId: string;

  @Prop({ required: true, ref: 'Restaurant' })
  restaurantId: string;

  @Prop({ 
    type: String,
    enum: ['UPI', 'PayAtCounter', 'COD'],
    required: true 
  })
  paymentMethods: string;

  @Prop({
    type: String,
    enum: ['Pending', 'Completed', 'Failed', 'Confirmed'],
    default: 'Pending'
  })
  paymentStatus: string;

  @Prop({
    type: String,
    enum: ['Placed', 'Processing', 'Preparing', 'Delivered', 'Cancelled', 'Confirmed'],
    default: 'Placed'
  })
  status: string;
}

export const OrderSchema = SchemaFactory.createForClass(Order);
export type OrderDocument = Order & Document;
