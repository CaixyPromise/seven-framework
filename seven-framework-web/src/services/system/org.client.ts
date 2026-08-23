import { request } from '@/api/request';

interface ApiPayload<T> {
  code: number;
  data: T;
  message?: string;
}

export async function checkOrgCode(code: string, excludeId?: number) {
  try {
    const response = await request<ApiPayload<boolean>>('/api/org/checkCode', {
      method: 'GET',
      params: excludeId !== undefined ? { code, excludeId } : { code },
    });
    return Boolean(response.data);
  } catch {
    return false;
  }
}
