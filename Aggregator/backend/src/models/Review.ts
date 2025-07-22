import mongoose, { Schema, Document } from 'mongoose';

export interface IReview extends Document {
  userId: string;
  orderId: string;
  restaurantId: string;
  rating: number;
  comment?: string;
  createdAt?: Date;
  updatedAt?: Date;
}

const ReviewSchema = new Schema<IReview>(
  {
    userId: { type: String, required: true },
    orderId: { type: String, required: true },
    restaurantId: { type: String, required: true },
    rating: { type: Number, required: true },
    comment: { type: String },
  },
  { timestamps: true }
);

export const ReviewModel = mongoose.model<IReview>('Review', ReviewSchema);
