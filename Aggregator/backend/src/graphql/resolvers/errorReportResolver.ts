import { ErrorReportModel } from '../../models/ErrorReport';
import { AuthenticationError } from 'apollo-server-express';
import { CurrentUser } from '../../middleware/authMiddleware';

interface SubmitErrorReportArgs {
  category?: string;
  priority?: string;
  description: string;
  screenshotURL?: string;
}

interface UpdateErrorReportArgs {
  id: string;
  status?: 'Open' | 'Resolved';
}

export const errorReportResolver = {
  Query: {
    getErrorReports: async (_parent: any, _args: any, context: { currentUser?: CurrentUser }) => {
      if (!context.currentUser || !context.currentUser.isAdmin) {
        throw new AuthenticationError('Admin privileges required to view error reports');
      }
      return await ErrorReportModel.find({});
    },
  },
  Mutation: {
    submitErrorReport: async (_parent: any, args: SubmitErrorReportArgs, context: { currentUser?: CurrentUser; io?: any }) => {
      if (!context.currentUser) {
        throw new AuthenticationError('Not authenticated');
      }
      const newReport = new ErrorReportModel({
        userId: context.currentUser.id,
        category: args.category || 'General',
        priority: args.priority || 'Low',
        description: args.description,
        screenshotURL: args.screenshotURL,
        status: 'Open',
      });
      await newReport.save();

      if (context.io) {
        context.io.emit('NEW_ERROR_REPORT', { reportId: newReport._id });
      }

      return newReport;
    },

    updateErrorReport: async (_parent: any, args: UpdateErrorReportArgs, context: { currentUser?: CurrentUser; io?: any }) => {
      if (!context.currentUser || !context.currentUser.isAdmin) {
        throw new AuthenticationError('Admin privileges required to update error reports');
      }
      const report = await ErrorReportModel.findById(args.id);
      if (!report) {
        throw new Error('Error report not found');
      }
      if (args.status) {
        report.status = args.status;
      }
      await report.save();

      if (context.io) {
        context.io.emit('ERROR_REPORT_UPDATED', { reportId: report._id, status: report.status });
      }

      return report;
    },
  },
};
