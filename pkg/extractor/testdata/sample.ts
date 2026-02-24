// TypeScript sample file for testing extraction
import { User, Profile } from './models';
import * as utils from './utils';

export interface Config {
  apiKey: string;
  debug: boolean;
}

export class UserService {
  private apiKey: string;

  constructor(config: Config) {
    this.apiKey = config.apiKey;
  }

  public async getUser(id: number): Promise<User> {
    const user = await this.fetchUser(id);
    return this.transformUser(user);
  }

  private async fetchUser(id: number): Promise<any> {
    return utils.apiCall(`/users/${id}`);
  }

  private transformUser(data: any): User {
    return {
      id: data.id,
      name: data.name,
    };
  }
}

export function createUserService(config: Config): UserService {
  return new UserService(config);
}

export default UserService;
