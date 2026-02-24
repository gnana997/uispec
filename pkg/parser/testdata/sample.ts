// Sample TypeScript file for parser testing
interface User {
  id: number;
  name: string;
  email: string;
}

function getUserById(id: number): User | null {
  // Implementation would go here
  return null;
}

class UserService {
  private users: Map<number, User>;

  constructor() {
    this.users = new Map();
  }

  addUser(user: User): void {
    this.users.set(user.id, user);
  }

  getUser(id: number): User | undefined {
    return this.users.get(id);
  }
}

export { User, getUserById, UserService };
