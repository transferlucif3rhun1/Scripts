import { WebSocketGateway, WebSocketServer, OnGatewayConnection } from '@nestjs/websockets';
import { Server, Socket } from 'socket.io';
import { JwtService } from '@nestjs/jwt';
import { UserService } from '../user/user.service';

@WebSocketGateway({ cors: true })
export class NotificationGateway implements OnGatewayConnection {
  @WebSocketServer() server: Server;

  constructor(
    private jwtService: JwtService,
    private userService: UserService,
  ) {}

  async handleConnection(socket: Socket) {
    try {
      const token = socket.handshake.auth.token;
      const payload = this.jwtService.verify(token);
      const user = await this.userService.findById(payload.sub);
      
      if (user) {
        // Join user-specific and role-specific rooms
        socket.join(user._id.toString());
        socket.join(user.role);
        
        // Vendor-specific room for payment updates
        if (user.role === 'vendor') {
          const vendor = await this.userService.getVendorDetails(user._id);
          socket.join(`vendor_${vendor._id}`);
        }
      }
    } catch (e) {
      socket.disconnect();
    }
  }
}