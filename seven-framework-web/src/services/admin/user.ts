import { request } from '@/api/request';
import type { PageResult, UserRecord } from '@/types/admin';

interface ApiPayload<T> {
  code: number;
  data: T;
  message?: string;
}

export interface UserListParams {
  current?: number;
  pageSize?: number;
  sortField?: string;
  sortOrder?: 'ascend' | 'descend';
  [key: string]: string | number | boolean | readonly string[] | readonly number[] | undefined;
}

export interface UserAddPayload {
  nickName: string;
  userName?: string;
  userEmail?: string;
  userRole?: string;
  userGender?: number;
  userPhone?: string;
}

export interface UserUpdatePayload {
  id: number;
  userName?: string;
  userRole?: string;
  userGender?: number;
  userPhone?: string;
  status?: number;
}

export async function fetchUsers(params: UserListParams) {
  const response = await request<ApiPayload<PageResult<UserRecord>>>('/api/user/list/page', {
    method: 'POST',
    data: {
      current: params.current,
      size: params.pageSize,
      ...params,
    },
  });
  return response.data;
}

export async function createUser(payload: UserAddPayload) {
  const response = await request<ApiPayload<{ token?: string }>>('/api/user/add', {
    method: 'POST',
    data: payload,
  });
  return response.data;
}

export async function updateUser(payload: UserUpdatePayload) {
  const response = await request<ApiPayload<boolean>>('/api/user/update', {
    method: 'POST',
    data: payload,
  });
  return response.data;
}

export async function removeUser(userId: number) {
  const response = await request<ApiPayload<boolean>>('/api/user/delete', {
    method: 'POST',
    data: { id: userId },
  });
  return response.data;
}

export async function resetUserPassword(userId: number) {
  const response = await request<ApiPayload<string>>('/api/admin/user/reset-password', {
    method: 'POST',
    params: { userId },
  });
  return response.data;
}
