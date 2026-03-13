// User management types -- matches server REST responses

export interface User {
  user_id: string;
  email: string;
  display_name: string;
  role: string;
  created_at: string;
  updated_at: string;
}

export interface CreateUserParams {
  email: string;
  password: string;
  display_name: string;
  role: string;
}

export interface UpdateUserParams {
  display_name?: string;
  role?: string;
}
