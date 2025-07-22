import { Args, Mutation, Resolver } from '@nestjs/graphql';
import { AuthService } from './auth.service';
import { LoginInput, RegisterInput } from './dto/auth.input';

@Resolver()
export class AuthResolver {
  constructor(private authService: AuthService) {}

  @Mutation()
  async login(@Args('input') input: LoginInput) {
    return this.authService.login(input);
  }

  @Mutation()
  async register(@Args('input') input: RegisterInput) {
    return this.authService.register(input);
  }
}