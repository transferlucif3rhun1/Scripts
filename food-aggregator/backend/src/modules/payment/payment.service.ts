import { Injectable } from '@nestjs/common';
import { OrderService } from '../order/order.service';
import { NotificationGateway } from '../notification/notification.gateway';

@Injectable()
export class PaymentService {
  constructor(
    private orderService: OrderService,
    private notificationGateway: NotificationGateway,
  ) {}

  async handlePaymentConfirmation(orderId: string) {
    const order = await this.orderService.confirmOrder(orderId);
    
    this.notificationGateway.server.emit('order_update', {
      orderId: order._id,
      status: order.status,
      paymentStatus: order.paymentStatus
    });

    return order;
  }
}