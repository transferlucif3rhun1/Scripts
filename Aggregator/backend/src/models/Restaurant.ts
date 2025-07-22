import mongoose, { Schema, Document } from 'mongoose';

export interface IRestaurant extends Document {
  name: string;
  address: string;
  createdAt?: Date;
  updatedAt?: Date;
}

const RestaurantSchema = new Schema<IRestaurant>(
  {
    name: { type: String, required: true },
    address: { type: String, required: true },
  },
  { timestamps: true }
);

export const RestaurantModel = mongoose.model<IRestaurant>('Restaurant', RestaurantSchema);
