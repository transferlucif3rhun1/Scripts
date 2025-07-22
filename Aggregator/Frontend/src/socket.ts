import { io, Socket } from 'socket.io-client';

const socket: Socket = io('http://localhost:4000', {
  transports: ['websocket'],
  reconnectionAttempts: 5,
});

export default socket;
