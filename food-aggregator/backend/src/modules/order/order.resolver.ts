order.resolver.tsimport { Args, Mutation, Query, Resolver } from '@nestjs/graphql';
import { UseGuards } from '@nestjs/common';
import { GqlAuthGuard } from '../../common/guards/gql-auth.guard';
import { RolesGuard } from '../../common/guards/roles.guard';
import { Roles } from '../../common/decorators/roles.decorator';
import { UserRole } from '../../constants/enums';
import { OrderService } from './order.service';
import { CreateOrderInput } from './dto/order.input';

@Resolver('Order')
export class OrderResolver {
  constructor(private orderService: OrderService) {}

  @Mutation()
  @UseGuards(GqlAuthGuard, RolesGuard)
  @Roles(UserRole.CUSTOMER)
  async placeOrder(@Args('input') input: CreateOrderInput) {
    return this.orderService.createOrder(input);
  }

  @Query()
  @UseGuards(GqlAuthGuard, RolesGuard)
  @Roles(UserRole.ADMIN, UserRole.VENDOR)
  async orders() {
    return this.orderService.findAll();
  }
}