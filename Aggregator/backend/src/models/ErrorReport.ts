import mongoose, { Schema, Document } from 'mongoose';

export interface IErrorReport extends Document {
  userId: string;
  category: string;
  priority: string;
  description: string;
  screenshotURL?: string;
  status: 'Open' | 'Resolved';
  createdAt?: Date;
  updatedAt?: Date;
}

const ErrorReportSchema = new Schema<IErrorReport>(
  {
    userId: { type: String, required: true },
    category: { type: String, default: 'General' },
    priority: { type: String, default: 'Low' },
    description: { type: String, required: true },
    screenshotURL: { type: String },
    status: { type: String, enum: ['Open', 'Resolved'], default: 'Open' },
  },
  { timestamps: true }
);

export const ErrorReportModel = mongoose.model<IErrorReport>('ErrorReport', ErrorReportSchema);
